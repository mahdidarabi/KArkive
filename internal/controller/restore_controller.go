package controller

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/config"
	"github.com/mahdidarabi/Karkive/internal/resources"
)

var requiredRestoreSecretKeys = []string{
	"s3_access_key",
	"s3_secret_key",
	"gpg_passphrase",
}

// RestoreReconciler reconciles a Restore object into ConfigMap + optional PVC + CronJob.
type RestoreReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   config.Config
}

// +kubebuilder:rbac:groups=karkive.io,resources=restores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=karkive.io,resources=restores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=karkive.io,resources=restores/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *RestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	restore := &karkivev1alpha1.Restore{}
	if err := r.Get(ctx, req.NamespacedName, restore); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !restore.DeletionTimestamp.IsZero() {
		logger.Info("Restore is deleting; not recreating owned resources")
		return ctrl.Result{}, nil
	}

	if !resources.EngineImplemented(restore.Spec.Engine) {
		engine := resources.EffectiveEngine(restore.Spec.Engine)
		msg := fmt.Sprintf("engine %q is not implemented", engine)
		logger.Info(msg)
		r.Recorder.Event(restore, corev1.EventTypeWarning, "UnsupportedEngine", msg)
		return ctrl.Result{}, r.setStatus(ctx, restore, karkivev1alpha1.RestorePhaseUnsupported, metav1.ConditionFalse, "UnsupportedEngine", msg)
	}

	if err := karkivev1alpha1.ValidateRestoreSpec(restore.Spec); err != nil {
		r.Recorder.Event(restore, corev1.EventTypeWarning, "InvalidSpec", err.Error())
		return ctrl.Result{}, r.setStatus(ctx, restore, karkivev1alpha1.RestorePhaseError, metav1.ConditionFalse, "InvalidSpec", err.Error())
	}

	if err := r.ensureJobSecret(ctx, restore); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("secret %q not found", restore.Spec.SecretRef.Name)
			r.Recorder.Event(restore, corev1.EventTypeWarning, "SecretNotFound", msg)
			if statusErr := r.setStatus(ctx, restore, karkivev1alpha1.RestorePhasePending, metav1.ConditionFalse, "SecretNotFound", msg); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: secretRequeue}, nil
		}
		r.Recorder.Event(restore, corev1.EventTypeWarning, "SecretInvalid", err.Error())
		return ctrl.Result{}, r.setStatus(ctx, restore, karkivev1alpha1.RestorePhaseError, metav1.ConditionFalse, "SecretInvalid", err.Error())
	}

	if err := r.ensureTargetSecret(ctx, restore); err != nil {
		if apierrors.IsNotFound(err) {
			name, _, _ := resources.RestoreTargetSecret(restore)
			msg := fmt.Sprintf("target secret %q not found", name)
			r.Recorder.Event(restore, corev1.EventTypeWarning, "TargetSecretNotFound", msg)
			if statusErr := r.setStatus(ctx, restore, karkivev1alpha1.RestorePhasePending, metav1.ConditionFalse, "TargetSecretNotFound", msg); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: secretRequeue}, nil
		}
		r.Recorder.Event(restore, corev1.EventTypeWarning, "TargetSecretInvalid", err.Error())
		return ctrl.Result{}, r.setStatus(ctx, restore, karkivev1alpha1.RestorePhaseError, metav1.ConditionFalse, "TargetSecretInvalid", err.Error())
	}

	if err := r.ensureConfigMap(ctx, restore); err != nil {
		return ctrl.Result{}, r.fail(ctx, restore, "ConfigMapError", err)
	}
	if err := r.ensurePVC(ctx, restore); err != nil {
		return ctrl.Result{}, r.fail(ctx, restore, "PVCError", err)
	}
	cron, err := r.ensureCronJob(ctx, restore)
	if err != nil {
		return ctrl.Result{}, r.fail(ctx, restore, "CronJobError", err)
	}

	restore.Status.CronJobName = cron.Name
	restore.Status.LastScheduleTime = cron.Status.LastScheduleTime
	restore.Status.LastSuccessfulTime = cron.Status.LastSuccessfulTime
	msg := fmt.Sprintf("CronJob %s is synced", cron.Name)
	r.Recorder.Event(restore, corev1.EventTypeNormal, "Synced", msg)
	return ctrl.Result{}, r.setStatus(ctx, restore, karkivev1alpha1.RestorePhaseReady, metav1.ConditionTrue, "Synced", msg)
}

func (r *RestoreReconciler) fail(ctx context.Context, restore *karkivev1alpha1.Restore, reason string, err error) error {
	r.Recorder.Event(restore, corev1.EventTypeWarning, reason, err.Error())
	if statusErr := r.setStatus(ctx, restore, karkivev1alpha1.RestorePhaseError, metav1.ConditionFalse, reason, err.Error()); statusErr != nil {
		return statusErr
	}
	return err
}

func (r *RestoreReconciler) ensureJobSecret(ctx context.Context, restore *karkivev1alpha1.Restore) error {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: restore.Namespace, Name: restore.Spec.SecretRef.Name}
	if err := r.Get(ctx, key, secret); err != nil {
		return err
	}
	for _, k := range requiredRestoreSecretKeys {
		if _, ok := secret.Data[k]; !ok {
			return fmt.Errorf("secret %q is missing key %q", secret.Name, k)
		}
	}
	return nil
}

func (r *RestoreReconciler) ensureTargetSecret(ctx context.Context, restore *karkivev1alpha1.Restore) error {
	name, userKey, passKey := resources.RestoreTargetSecret(restore)
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: restore.Namespace, Name: name}
	if err := r.Get(ctx, key, secret); err != nil {
		return err
	}
	for _, k := range []string{userKey, passKey} {
		if _, ok := secret.Data[k]; !ok {
			return fmt.Errorf("target secret %q is missing key %q", secret.Name, k)
		}
	}
	return nil
}

func (r *RestoreReconciler) ensureConfigMap(ctx context.Context, restore *karkivev1alpha1.Restore) error {
	owned := resources.RestoreOwnedName(restore)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: owned, Namespace: restore.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := resources.MutateRestoreConfigMap(cm, restore, r.Config); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(restore, cm, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
	})
	return err
}

func (r *RestoreReconciler) ensurePVC(ctx context.Context, restore *karkivev1alpha1.Restore) error {
	if restore.Spec.Persistence != nil && restore.Spec.Persistence.Enabled != nil && !*restore.Spec.Persistence.Enabled {
		return nil
	}
	owned := resources.RestoreOwnedName(restore)
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: owned, Namespace: restore.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		if !pvc.CreationTimestamp.IsZero() {
			pvc.Labels = resources.RestoreLabels(restore)
			return controllerutil.SetControllerReference(restore, pvc, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
		}
		resources.MutateRestorePVC(pvc, restore)
		return controllerutil.SetControllerReference(restore, pvc, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
	})
	return err
}

func (r *RestoreReconciler) ensureCronJob(ctx context.Context, restore *karkivev1alpha1.Restore) (*batchv1.CronJob, error) {
	owned := resources.RestoreOwnedName(restore)
	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: owned, Namespace: restore.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cj, func() error {
		resources.MutateRestoreCronJob(cj, restore, r.Config)
		return controllerutil.SetControllerReference(restore, cj, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
	})
	return cj, err
}

func (r *RestoreReconciler) setStatus(
	ctx context.Context,
	restore *karkivev1alpha1.Restore,
	phase string,
	ready metav1.ConditionStatus,
	reason, message string,
) error {
	restore.Status.LastJob = lastJobStatus(ctx, r.Client, restore.Namespace, resources.LabelRestoreName, restore.Name, resources.KindRestore)
	restore.Status.Phase = phase
	restore.Status.ObservedGeneration = restore.Generation
	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             ready,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: restore.Generation,
	})
	return r.Status().Update(ctx, restore)
}

func (r *RestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karkivev1alpha1.Restore{}).
		Owns(&batchv1.CronJob{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(&batchv1.Job{}, handler.EnqueueRequestsFromMapFunc(mapJobToOwner(resources.KindRestore, resources.LabelRestoreName))).
		Complete(r)
}

package controller

import (
	"context"
	"fmt"
	"time"

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

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
	"github.com/mahdidarabi/KArkive/internal/config"
	"github.com/mahdidarabi/KArkive/internal/resources"
)

const (
	conditionReady = "Ready"
	secretRequeue  = 30 * time.Second
)

var requiredBackupSecretKeys = []string{
	"username",
	"password",
	"s3_access_key",
	"s3_secret_key",
	"gpg_passphrase",
}

// BackupReconciler reconciles a Backup object into ConfigMap + PVC + CronJob.
type BackupReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Config   config.Config
}

// +kubebuilder:rbac:groups=karkive.io,resources=backups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=karkive.io,resources=backups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=karkive.io,resources=backups/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	backup := &karkivev1alpha1.Backup{}
	if err := r.Get(ctx, req.NamespacedName, backup); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !backup.DeletionTimestamp.IsZero() {
		logger.Info("Backup is deleting; not recreating owned resources")
		return ctrl.Result{}, nil
	}

	if !resources.EngineImplemented(backup.Spec.Engine) {
		engine := resources.EffectiveEngine(backup.Spec.Engine)
		msg := fmt.Sprintf("engine %q is not implemented", engine)
		logger.Info(msg)
		r.Recorder.Event(backup, corev1.EventTypeWarning, "UnsupportedEngine", msg)
		return ctrl.Result{}, r.setStatus(ctx, backup, karkivev1alpha1.BackupPhaseUnsupported, metav1.ConditionFalse, "UnsupportedEngine", msg)
	}

	if err := karkivev1alpha1.ValidateBackupSpec(backup.Spec); err != nil {
		r.Recorder.Event(backup, corev1.EventTypeWarning, "InvalidSpec", err.Error())
		return ctrl.Result{}, r.setStatus(ctx, backup, karkivev1alpha1.BackupPhaseError, metav1.ConditionFalse, "InvalidSpec", err.Error())
	}

	if err := r.ensureSecret(ctx, backup); err != nil {
		if apierrors.IsNotFound(err) {
			msg := fmt.Sprintf("secret %q not found", backup.Spec.SecretRef.Name)
			r.Recorder.Event(backup, corev1.EventTypeWarning, "SecretNotFound", msg)
			if statusErr := r.setStatus(ctx, backup, karkivev1alpha1.BackupPhasePending, metav1.ConditionFalse, "SecretNotFound", msg); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: secretRequeue}, nil
		}
		r.Recorder.Event(backup, corev1.EventTypeWarning, "SecretInvalid", err.Error())
		return ctrl.Result{}, r.setStatus(ctx, backup, karkivev1alpha1.BackupPhaseError, metav1.ConditionFalse, "SecretInvalid", err.Error())
	}

	if err := r.ensureConfigMap(ctx, backup); err != nil {
		return ctrl.Result{}, r.fail(ctx, backup, "ConfigMapError", err)
	}
	if err := r.ensurePVC(ctx, backup); err != nil {
		return ctrl.Result{}, r.fail(ctx, backup, "PVCError", err)
	}
	cron, err := r.ensureCronJob(ctx, backup)
	if err != nil {
		return ctrl.Result{}, r.fail(ctx, backup, "CronJobError", err)
	}

	backup.Status.CronJobName = cron.Name
	backup.Status.LastScheduleTime = cron.Status.LastScheduleTime
	backup.Status.LastSuccessfulTime = cron.Status.LastSuccessfulTime
	msg := fmt.Sprintf("CronJob %s is synced", cron.Name)
	r.Recorder.Event(backup, corev1.EventTypeNormal, "Synced", msg)
	return ctrl.Result{}, r.setStatus(ctx, backup, karkivev1alpha1.BackupPhaseReady, metav1.ConditionTrue, "Synced", msg)
}

func (r *BackupReconciler) fail(ctx context.Context, backup *karkivev1alpha1.Backup, reason string, err error) error {
	r.Recorder.Event(backup, corev1.EventTypeWarning, reason, err.Error())
	if statusErr := r.setStatus(ctx, backup, karkivev1alpha1.BackupPhaseError, metav1.ConditionFalse, reason, err.Error()); statusErr != nil {
		return statusErr
	}
	return err
}

func (r *BackupReconciler) ensureSecret(ctx context.Context, backup *karkivev1alpha1.Backup) error {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: backup.Namespace, Name: backup.Spec.SecretRef.Name}
	if err := r.Get(ctx, key, secret); err != nil {
		return err
	}
	for _, k := range requiredBackupSecretKeys {
		if _, ok := secret.Data[k]; !ok {
			return fmt.Errorf("secret %q is missing key %q", secret.Name, k)
		}
	}
	return nil
}

func (r *BackupReconciler) ensureConfigMap(ctx context.Context, backup *karkivev1alpha1.Backup) error {
	owned := resources.BackupOwnedName(backup)
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: owned, Namespace: backup.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if err := resources.MutateBackupConfigMap(cm, backup, r.Config); err != nil {
			return err
		}
		return controllerutil.SetControllerReference(backup, cm, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
	})
	return err
}

func (r *BackupReconciler) ensurePVC(ctx context.Context, backup *karkivev1alpha1.Backup) error {
	if backup.Spec.Persistence != nil && backup.Spec.Persistence.Enabled != nil && !*backup.Spec.Persistence.Enabled {
		return nil
	}
	owned := resources.BackupOwnedName(backup)
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: owned, Namespace: backup.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, func() error {
		if !pvc.CreationTimestamp.IsZero() {
			pvc.Labels = resources.BackupLabels(backup)
			return controllerutil.SetControllerReference(backup, pvc, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
		}
		resources.MutateBackupPVC(pvc, backup)
		return controllerutil.SetControllerReference(backup, pvc, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
	})
	return err
}

func (r *BackupReconciler) ensureCronJob(ctx context.Context, backup *karkivev1alpha1.Backup) (*batchv1.CronJob, error) {
	owned := resources.BackupOwnedName(backup)
	cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: owned, Namespace: backup.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, cj, func() error {
		resources.MutateBackupCronJob(cj, backup, r.Config)
		return controllerutil.SetControllerReference(backup, cj, r.Scheme, controllerutil.WithBlockOwnerDeletion(false))
	})
	return cj, err
}

func (r *BackupReconciler) setStatus(
	ctx context.Context,
	backup *karkivev1alpha1.Backup,
	phase string,
	ready metav1.ConditionStatus,
	reason, message string,
) error {
	backup.Status.LastJob = lastJobStatus(ctx, r.Client, backup.Namespace, resources.LabelBackupName, backup.Name, resources.KindBackup)
	backup.Status.Phase = phase
	backup.Status.ObservedGeneration = backup.Generation
	meta.SetStatusCondition(&backup.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             ready,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: backup.Generation,
	})
	return r.Status().Update(ctx, backup)
}

func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karkivev1alpha1.Backup{}).
		Owns(&batchv1.CronJob{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(&batchv1.Job{}, handler.EnqueueRequestsFromMapFunc(mapJobToOwner(resources.KindBackup, resources.LabelBackupName))).
		Complete(r)
}

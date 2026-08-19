package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
)

// RestoreReconciler is a stub: Restore CRs are accepted so the API is stable,
// but pipeline resources are not created until the restore phase.
type RestoreReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=karkive.io,resources=restores,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=karkive.io,resources=restores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=karkive.io,resources=restores/finalizers,verbs=update

func (r *RestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	restore := &karkivev1alpha1.Restore{}
	if err := r.Get(ctx, req.NamespacedName, restore); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	msg := "restore reconciliation is not implemented yet; postgres backup ships first"
	logger.Info(msg, "restore", req.NamespacedName)
	r.Recorder.Event(restore, corev1.EventTypeNormal, "NotImplemented", msg)

	restore.Status.Phase = karkivev1alpha1.RestorePhaseNotImplemented
	restore.Status.ObservedGeneration = restore.Generation
	meta.SetStatusCondition(&restore.Status.Conditions, metav1.Condition{
		Type:               conditionReady,
		Status:             metav1.ConditionFalse,
		Reason:             "NotImplemented",
		Message:            msg,
		ObservedGeneration: restore.Generation,
	})
	return ctrl.Result{}, r.Status().Update(ctx, restore)
}

func (r *RestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karkivev1alpha1.Restore{}).
		Complete(r)
}

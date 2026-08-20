package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-karkive-io-v1alpha1-restore,mutating=false,failurePolicy=fail,sideEffects=None,groups=karkive.io,resources=restores,verbs=create;update,versions=v1alpha1,name=vrestore.karkive.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &RestoreCustomValidator{}

// SetupWebhookWithManager registers the Restore validating webhook.
func (r *Restore) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(&RestoreCustomValidator{}).
		Complete()
}

// RestoreCustomValidator validates Restore specs at admission time.
type RestoreCustomValidator struct{}

func (v *RestoreCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	restore, err := asRestore(obj)
	if err != nil {
		return nil, err
	}
	return nil, ValidateRestoreSpec(restore.Spec)
}

func (v *RestoreCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	restore, err := asRestore(newObj)
	if err != nil {
		return nil, err
	}
	return nil, ValidateRestoreSpec(restore.Spec)
}

func (v *RestoreCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func asRestore(obj runtime.Object) (*Restore, error) {
	restore, ok := obj.(*Restore)
	if !ok {
		return nil, fmt.Errorf("expected Restore, got %T", obj)
	}
	return restore, nil
}

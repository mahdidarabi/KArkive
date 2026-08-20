package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-karkive-io-v1alpha1-backup,mutating=false,failurePolicy=fail,sideEffects=None,groups=karkive.io,resources=backups,verbs=create;update,versions=v1alpha1,name=vbackup.karkive.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &BackupCustomValidator{}

// SetupWebhookWithManager registers the Backup validating webhook.
func (r *Backup) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		WithValidator(&BackupCustomValidator{}).
		Complete()
}

// BackupCustomValidator validates Backup specs at admission time.
type BackupCustomValidator struct{}

func (v *BackupCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	backup, err := asBackup(obj)
	if err != nil {
		return nil, err
	}
	return nil, ValidateBackupSpec(backup.Spec)
}

func (v *BackupCustomValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	backup, err := asBackup(newObj)
	if err != nil {
		return nil, err
	}
	return nil, ValidateBackupSpec(backup.Spec)
}

func (v *BackupCustomValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func asBackup(obj runtime.Object) (*Backup, error) {
	backup, ok := obj.(*Backup)
	if !ok {
		return nil, fmt.Errorf("expected Backup, got %T", obj)
	}
	return backup, nil
}

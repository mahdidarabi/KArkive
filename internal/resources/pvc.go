package resources

import (
	corev1 "k8s.io/api/core/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
)

// MutateBackupPVC writes the PVC spec. Call only when creating; spec is mostly immutable.
func MutateBackupPVC(pvc *corev1.PersistentVolumeClaim, backup *karkivev1alpha1.Backup) {
	p := backup.Spec.Persistence
	accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	if p != nil && len(p.AccessModes) > 0 {
		accessModes = p.AccessModes
	}

	pvc.Labels = BackupLabels(backup)
	if p != nil && len(p.Annotations) > 0 {
		pvc.Annotations = p.Annotations
	}

	spec := corev1.PersistentVolumeClaimSpec{
		AccessModes: accessModes,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: persistenceSize(p),
			},
		},
	}
	if p != nil && p.StorageClassName != "" {
		sc := p.StorageClassName
		spec.StorageClassName = &sc
	}
	pvc.Spec = spec
}

// MutateRestorePVC writes the PVC spec. Call only when creating; spec is mostly immutable.
func MutateRestorePVC(pvc *corev1.PersistentVolumeClaim, restore *karkivev1alpha1.Restore) {
	p := restore.Spec.Persistence
	accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	if p != nil && len(p.AccessModes) > 0 {
		accessModes = p.AccessModes
	}

	pvc.Labels = RestoreLabels(restore)
	if p != nil && len(p.Annotations) > 0 {
		pvc.Annotations = p.Annotations
	}

	spec := corev1.PersistentVolumeClaimSpec{
		AccessModes: accessModes,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: persistenceSize(p),
			},
		},
	}
	if p != nil && p.StorageClassName != "" {
		sc := p.StorageClassName
		spec.StorageClassName = &sc
	}
	pvc.Spec = spec
}

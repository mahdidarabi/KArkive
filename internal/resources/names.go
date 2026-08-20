package resources

import (
	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
)

// ResourceNamePrefix is prepended to every Kubernetes object the operator creates.
const ResourceNamePrefix = "karkive-"

// BackupOwnedName is the ConfigMap / PVC / CronJob name for a Backup.
func BackupOwnedName(backup *karkivev1alpha1.Backup) string {
	return ResourceNamePrefix + backup.Name
}

// RestoreOwnedName is the ConfigMap / PVC / CronJob name for a Restore.
func RestoreOwnedName(restore *karkivev1alpha1.Restore) string {
	return ResourceNamePrefix + restore.Name
}

package resources

import (
	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
)

const (
	// BackupNamePrefix is prepended to ConfigMap / PVC / CronJob names for a Backup.
	BackupNamePrefix = "karkive-backup-"
	// RestoreNamePrefix is prepended to ConfigMap / PVC / CronJob names for a Restore.
	RestoreNamePrefix = "karkive-restore-"
	// LegacyNamePrefix is the pre-0.0.4 owned-resource prefix (`karkive-<cr-name>`).
	// Backup and Restore CRs with the same name shared that name, so CronJobs collided.
	LegacyNamePrefix = "karkive-"
)

// BackupOwnedName is the ConfigMap / PVC / CronJob name for a Backup.
func BackupOwnedName(backup *karkivev1alpha1.Backup) string {
	return BackupNamePrefix + backup.Name
}

// RestoreOwnedName is the ConfigMap / PVC / CronJob name for a Restore.
func RestoreOwnedName(restore *karkivev1alpha1.Restore) string {
	return RestoreNamePrefix + restore.Name
}

// LegacyOwnedName is the pre-0.0.4 ConfigMap / PVC / CronJob name for a CR.
func LegacyOwnedName(crName string) string {
	return LegacyNamePrefix + crName
}

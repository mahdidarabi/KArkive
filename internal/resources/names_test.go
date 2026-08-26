package resources

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
)

func TestBackupOwnedName(t *testing.T) {
	got := BackupOwnedName(&karkivev1alpha1.Backup{ObjectMeta: metav1.ObjectMeta{Name: "app-postgres"}})
	if got != "karkive-backup-app-postgres" {
		t.Errorf("BackupOwnedName=%q", got)
	}
}

func TestRestoreOwnedName(t *testing.T) {
	got := RestoreOwnedName(&karkivev1alpha1.Restore{ObjectMeta: metav1.ObjectMeta{Name: "app-postgres"}})
	if got != "karkive-restore-app-postgres" {
		t.Errorf("RestoreOwnedName=%q", got)
	}
}

func TestBackupAndRestoreOwnedNamesDiffer(t *testing.T) {
	backup := BackupOwnedName(&karkivev1alpha1.Backup{ObjectMeta: metav1.ObjectMeta{Name: "app-postgres"}})
	restore := RestoreOwnedName(&karkivev1alpha1.Restore{ObjectMeta: metav1.ObjectMeta{Name: "app-postgres"}})
	if backup == restore {
		t.Fatalf("Backup and Restore with the same CR name must not share owned names, got %q", backup)
	}
	if LegacyOwnedName("app-postgres") == backup {
		t.Fatal("legacy name must differ from Backup owned name")
	}
}

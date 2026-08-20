package v1alpha1

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestBackupWebhookRejectsBadSchedule(t *testing.T) {
	v := &BackupCustomValidator{}
	backup := &Backup{
		Spec: BackupSpec{
			Schedule:  "never",
			Database:  DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3:        S3Spec{Path: "app/pgdump"},
			SecretRef: corev1.LocalObjectReference{Name: "creds"},
		},
	}
	if _, err := v.ValidateCreate(context.Background(), backup); err == nil {
		t.Fatal("expected reject")
	}
	backup.Spec.Schedule = "0 2 * * *"
	if _, err := v.ValidateCreate(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreWebhookRejectsWrongType(t *testing.T) {
	v := &RestoreCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), &Backup{}); err == nil {
		t.Fatal("expected type error")
	}
}

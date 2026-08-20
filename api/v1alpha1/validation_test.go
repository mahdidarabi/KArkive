package v1alpha1

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestValidateBackupSpec(t *testing.T) {
	ok := BackupSpec{
		Schedule:  "0 2 * * *",
		Database:  DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
		S3:        S3Spec{Path: "app/pgdump"},
		SecretRef: corev1.LocalObjectReference{Name: "backup-creds"},
	}
	if err := ValidateBackupSpec(ok); err != nil {
		t.Fatal(err)
	}
	bad := ok
	bad.Schedule = "not a cron"
	if err := ValidateBackupSpec(bad); err == nil {
		t.Fatal("expected invalid schedule")
	}
	missing := ok
	missing.SecretRef.Name = ""
	if err := ValidateBackupSpec(missing); err == nil {
		t.Fatal("expected missing secretRef")
	}
}

func TestValidateRestoreSpec(t *testing.T) {
	ok := RestoreSpec{
		Engine:         EnginePostgres,
		Schedule:       "30 2 * * *",
		Database:       DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
		S3:             S3Spec{Path: "app/pgdump"},
		SecretRef:      corev1.LocalObjectReference{Name: "restore-creds"},
		PostgresSecret: &SecretKeySelector{Name: "postgres"},
	}
	if err := ValidateRestoreSpec(ok); err != nil {
		t.Fatal(err)
	}
	redis := ok
	redis.Engine = EngineRedis
	redis.PostgresSecret = nil
	if err := ValidateRestoreSpec(redis); err == nil {
		t.Fatal("expected redisSecret")
	}
	redis.RedisSecret = &SecretKeySelector{Name: "redis"}
	if err := ValidateRestoreSpec(redis); err != nil {
		t.Fatal(err)
	}
}

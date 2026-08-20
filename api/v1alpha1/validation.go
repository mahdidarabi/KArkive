package v1alpha1

import (
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
)

// ValidateBackupSpec checks Backup fields the operator can evaluate without the cluster.
func ValidateBackupSpec(spec BackupSpec) error {
	if err := validateSchedule(spec.Schedule); err != nil {
		return err
	}
	if spec.Database.Host == "" {
		return fmt.Errorf("spec.database.host is required")
	}
	if spec.Database.Name == "" {
		return fmt.Errorf("spec.database.name is required")
	}
	if spec.S3.Path == "" {
		return fmt.Errorf("spec.s3.path is required")
	}
	if spec.SecretRef.Name == "" {
		return fmt.Errorf("spec.secretRef.name is required")
	}
	if err := validateJobPolicy(spec.Job); err != nil {
		return err
	}
	return nil
}

// ValidateRestoreSpec checks Restore fields the operator can evaluate without the cluster.
func ValidateRestoreSpec(spec RestoreSpec) error {
	if err := validateSchedule(spec.Schedule); err != nil {
		return err
	}
	if spec.Database.Host == "" {
		return fmt.Errorf("spec.database.host is required")
	}
	if spec.Database.Name == "" {
		return fmt.Errorf("spec.database.name is required")
	}
	if spec.S3.Path == "" {
		return fmt.Errorf("spec.s3.path is required")
	}
	if spec.SecretRef.Name == "" {
		return fmt.Errorf("spec.secretRef.name is required")
	}
	switch engine := spec.Engine; engine {
	case EngineMariaDB:
		if spec.MariaDBSecret == nil || spec.MariaDBSecret.Name == "" {
			return fmt.Errorf("spec.mariadbSecret.name is required")
		}
	case EngineRedis:
		if spec.RedisSecret == nil || spec.RedisSecret.Name == "" {
			return fmt.Errorf("spec.redisSecret.name is required")
		}
	default:
		if spec.PostgresSecret == nil || spec.PostgresSecret.Name == "" {
			return fmt.Errorf("spec.postgresSecret.name is required")
		}
	}
	if err := validateJobPolicy(spec.Job); err != nil {
		return err
	}
	return nil
}

func validateSchedule(schedule string) error {
	if strings.TrimSpace(schedule) == "" {
		return fmt.Errorf("spec.schedule is required")
	}
	if _, err := cron.ParseStandard(schedule); err != nil {
		return fmt.Errorf("spec.schedule is not a valid cron expression: %w", err)
	}
	return nil
}

func validateJobPolicy(job *JobPolicy) error {
	if job == nil {
		return nil
	}
	switch job.ConcurrencyPolicy {
	case "", "Allow", "Forbid", "Replace":
	default:
		return fmt.Errorf("spec.job.concurrencyPolicy must be Allow, Forbid, or Replace")
	}
	switch job.RestartPolicy {
	case "", corev1.RestartPolicyAlways, corev1.RestartPolicyOnFailure, corev1.RestartPolicyNever:
	default:
		return fmt.Errorf("spec.job.restartPolicy must be Always, OnFailure, or Never")
	}
	return nil
}

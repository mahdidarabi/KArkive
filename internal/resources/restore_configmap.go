package resources

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
	"github.com/mahdidarabi/KArkive/internal/config"
)

// MutateRestoreConfigMap writes the restore pipeline environment into cm.
func MutateRestoreConfigMap(cm *corev1.ConfigMap, restore *karkivev1alpha1.Restore, cfg config.Config) error {
	engine := EffectiveEngine(restore.Spec.Engine)
	port := DatabasePort(restore.Spec.Database, engine)

	cm.Labels = RestoreLabels(restore)
	cm.Data = map[string]string{
		"ENGINE":                        string(engine),
		"DUMP_PREFIX":                   DumpPrefix(engine),
		"WORKDIR":                       restoreWorkdir(restore),
		"MC_CONFIG_DIR":                 restoreMcConfigDir(restore),
		"S3_ENDPOINT":                   restoreS3Endpoint(restore, cfg),
		"S3_BUCKET":                     restoreS3Bucket(restore, cfg),
		"S3_PATH":                       restore.Spec.S3.Path,
		"BACKUP_FILE":                   restore.Spec.BackupFile,
		"USE_LATEST_BACKUP_AS_FALLBACK": boolEnv(restore.Spec.UseLatestBackupAsFallback, true),
		"DROP_DATABASE_IF_EXISTS":       boolEnv(restore.Spec.DropDatabaseIfExists, true),
	}

	switch engine {
	case karkivev1alpha1.EngineMariaDB:
		cm.Data["MYSQL_HOST"] = restore.Spec.Database.Host
		cm.Data["MYSQL_PORT"] = strconv.Itoa(int(port))
		cm.Data["MYSQL_DATABASE"] = restore.Spec.Database.Name
	case karkivev1alpha1.EngineRedis:
		cm.Data["REDIS_HOST"] = restore.Spec.Database.Host
		cm.Data["REDIS_PORT"] = strconv.Itoa(int(port))
		cm.Data["REDIS_NAME"] = restore.Spec.Database.Name
	default:
		cm.Data["PGHOST"] = restore.Spec.Database.Host
		cm.Data["PGPORT"] = strconv.Itoa(int(port))
		cm.Data["PGDATABASE"] = restore.Spec.Database.Name
		cm.Data["PG_OWNER_ROLE"] = restoreOwnerRole(restore)
		cm.Data["STRIP_PGAUDIT_EXTENSION"] = boolEnv(restore.Spec.StripPgAuditExtension, true)
	}

	if cm.Data["S3_ENDPOINT"] == "" {
		return fmt.Errorf("s3.endpoint is empty (set spec.s3.endpoint or --default-s3-endpoint)")
	}
	if cm.Data["S3_BUCKET"] == "" {
		return fmt.Errorf("s3.bucket is empty (set spec.s3.bucket or --default-s3-bucket)")
	}
	return nil
}

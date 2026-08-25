package resources

import (
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
	"github.com/mahdidarabi/KArkive/internal/config"
	"github.com/mahdidarabi/KArkive/internal/pipeline"
	"github.com/mahdidarabi/KArkive/internal/ptr"
)

func testBackup() *karkivev1alpha1.Backup {
	return &karkivev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup"},
		Spec: karkivev1alpha1.BackupSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "0 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{
				Host: "postgres.example.svc.cluster.local",
				Name: "app",
			},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef: corev1.LocalObjectReference{Name: "backup-app-postgres"},
		},
	}
}

func TestMutateBackupConfigMap_Postgres(t *testing.T) {
	cm := &corev1.ConfigMap{}
	if err := MutateBackupConfigMap(cm, testBackup(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"ENGINE":      "postgres",
		"DUMP_PREFIX": "pg_dump",
		"PGHOST":      "postgres.example.svc.cluster.local",
		"PGPORT":      "5432",
		"PGDATABASE":  "app",
		"S3_PATH":     "app/pgdump",
		"S3_ENDPOINT": "https://s3.example.com",
		"S3_BUCKET":   "backups",
		"S3_ENABLED":  "true",
	}
	for k, v := range want {
		if cm.Data[k] != v {
			t.Errorf("data[%s]=%q, want %q", k, cm.Data[k], v)
		}
	}
}

func TestMutateBackupCronJob_PostgresStages(t *testing.T) {
	cj := &batchv1.CronJob{}
	MutateBackupCronJob(cj, testBackup(), config.Config{})

	got := containerNames(cj)
	want := []string{"cleanup", "pgdump", "compress", "encrypt", "s3-sync"}
	if len(got) != len(want) {
		t.Fatalf("containers=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("containers=%v, want %v", got, want)
		}
	}

	pgdump := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1]
	if !hasEnv(pgdump, "PGUSER") || !hasEnv(pgdump, "PGPASSWORD") {
		t.Errorf("pgdump missing PGUSER/PGPASSWORD from secret")
	}
	if cj.Spec.Schedule != "0 2 * * *" {
		t.Errorf("schedule=%q", cj.Spec.Schedule)
	}
	if cj.Spec.ConcurrencyPolicy != batchv1.ForbidConcurrent {
		t.Errorf("concurrency=%q", cj.Spec.ConcurrencyPolicy)
	}

	images := map[string]string{}
	for _, c := range cj.Spec.JobTemplate.Spec.Template.Spec.Containers {
		images[c.Name] = c.Image
	}
	if images["cleanup"] != config.DefaultBusyBoxImage || images["compress"] != config.DefaultBusyBoxImage {
		t.Errorf("cleanup/compress images=%v", images)
	}
	if images["pgdump"] != config.DefaultPostgresImage {
		t.Errorf("pgdump image=%s", images["pgdump"])
	}
	if images["encrypt"] != config.DefaultGnuPGImage {
		t.Errorf("encrypt image=%s", images["encrypt"])
	}
	if images["s3-sync"] != config.DefaultMcImage {
		t.Errorf("s3-sync image=%s", images["s3-sync"])
	}

	cleanup := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	if len(cleanup.Command) != 3 || cleanup.Command[0] != "/bin/sh" {
		t.Fatalf("cleanup command=%v", cleanup.Command)
	}
	script := cleanup.Command[2]
	if !strings.HasPrefix(script, pipeline.CommonScript()) {
		t.Error("cleanup script missing prepended common.sh")
	}
	if !strings.Contains(script, "STAGE=cleanup") {
		t.Error("cleanup script missing STAGE=cleanup")
	}
}

func TestMutateBackupCronJob_S3Disabled(t *testing.T) {
	backup := testBackup()
	backup.Spec.S3 = karkivev1alpha1.S3Spec{Enabled: ptr.To(false)}
	cm := &corev1.ConfigMap{}
	if err := MutateBackupConfigMap(cm, backup, config.Config{}); err != nil {
		t.Fatal(err)
	}
	if cm.Data["S3_ENABLED"] != "false" {
		t.Errorf("S3_ENABLED=%q", cm.Data["S3_ENABLED"])
	}
	if _, ok := cm.Data["S3_ENDPOINT"]; ok {
		t.Error("S3_ENDPOINT should be omitted when S3 is disabled")
	}

	cj := &batchv1.CronJob{}
	MutateBackupCronJob(cj, backup, config.Config{})
	got := containerNames(cj)
	want := []string{"cleanup", "pgdump", "compress", "encrypt"}
	if len(got) != len(want) {
		t.Fatalf("containers=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("containers=%v, want %v", got, want)
		}
	}
	encrypt := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[3]
	if len(encrypt.Command) < 3 || !strings.Contains(encrypt.Command[2], "S3_ENABLED") {
		t.Error("encrypt script should honor S3_ENABLED")
	}
}

func TestPersistenceDisabledUsesEmptyDir(t *testing.T) {
	backup := testBackup()
	backup.Spec.Persistence = &karkivev1alpha1.PersistenceSpec{Enabled: ptr.To(false)}
	cj := &batchv1.CronJob{}
	MutateBackupCronJob(cj, backup, config.Config{})

	var datadir *corev1.Volume
	for i := range cj.Spec.JobTemplate.Spec.Template.Spec.Volumes {
		v := &cj.Spec.JobTemplate.Spec.Template.Spec.Volumes[i]
		if v.Name == volumeDataDir {
			datadir = v
			break
		}
	}
	if datadir == nil || datadir.EmptyDir == nil {
		t.Fatalf("expected emptyDir datadir, got %#v", datadir)
	}
}

func testRestore() *karkivev1alpha1.Restore {
	return &karkivev1alpha1.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup"},
		Spec: karkivev1alpha1.RestoreSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "30 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{
				Host:      "postgres.example.svc.cluster.local",
				Name:      "app",
				OwnerRole: "app",
			},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef: corev1.LocalObjectReference{Name: "restore-app-postgres"},
			PostgresSecret: &karkivev1alpha1.SecretKeySelector{
				Name: "postgres",
			},
			Persistence: &karkivev1alpha1.PersistenceSpec{Enabled: ptr.To(false)},
		},
	}
}

func TestMutateRestoreConfigMap_Postgres(t *testing.T) {
	cm := &corev1.ConfigMap{}
	if err := MutateRestoreConfigMap(cm, testRestore(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"ENGINE":                        "postgres",
		"DUMP_PREFIX":                   "pg_dump",
		"WORKDIR":                       "/workdir",
		"PGHOST":                        "postgres.example.svc.cluster.local",
		"PGPORT":                        "5432",
		"PGDATABASE":                    "app",
		"PG_OWNER_ROLE":                 "app",
		"S3_PATH":                       "app/pgdump",
		"USE_LATEST_BACKUP_AS_FALLBACK": "true",
		"DROP_DATABASE_IF_EXISTS":       "true",
		"STRIP_PGAUDIT_EXTENSION":       "true",
	}
	for k, v := range want {
		if cm.Data[k] != v {
			t.Errorf("data[%s]=%q, want %q", k, cm.Data[k], v)
		}
	}
}

func TestMutateRestoreCronJob_PostgresStages(t *testing.T) {
	cj := &batchv1.CronJob{}
	MutateRestoreCronJob(cj, testRestore(), config.Config{})

	got := containerNames(cj)
	want := []string{"cleanup", "fetch", "decrypt", "extract", "pgrestore"}
	if len(got) != len(want) {
		t.Fatalf("containers=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("containers=%v, want %v", got, want)
		}
	}

	fetch := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1]
	if !hasEnv(fetch, "S3_ACCESS_KEY") || !hasEnv(fetch, "S3_SECRET_KEY") {
		t.Errorf("fetch missing S3 keys from secret")
	}
	pgrestore := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[4]
	if !hasEnv(pgrestore, "PGUSER") || !hasEnv(pgrestore, "PGPASSWORD") {
		t.Errorf("pgrestore missing PGUSER/PGPASSWORD from postgresSecret")
	}
	if cj.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy=%q", cj.Spec.JobTemplate.Spec.Template.Spec.RestartPolicy)
	}
	if cj.Spec.JobTemplate.Spec.BackoffLimit != nil && *cj.Spec.JobTemplate.Spec.BackoffLimit != 1 {
		t.Errorf("backoffLimit=%v", cj.Spec.JobTemplate.Spec.BackoffLimit)
	}

	cleanup := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[0]
	if len(cleanup.Command) != 3 {
		t.Fatalf("cleanup command=%v", cleanup.Command)
	}
	if !strings.HasPrefix(cleanup.Command[2], pipeline.CommonScript()) {
		t.Error("restore cleanup script missing prepended common.sh")
	}
	if !strings.Contains(cleanup.Command[2], "STAGE=cleanup") {
		t.Error("restore cleanup script missing STAGE=cleanup")
	}

	var workdir *corev1.Volume
	for i := range cj.Spec.JobTemplate.Spec.Template.Spec.Volumes {
		v := &cj.Spec.JobTemplate.Spec.Template.Spec.Volumes[i]
		if v.Name == volumeWorkdir {
			workdir = v
			break
		}
	}
	if workdir == nil || workdir.EmptyDir == nil {
		t.Fatalf("expected emptyDir workdir, got %#v", workdir)
	}
}

func TestDumpPrefixAndPort(t *testing.T) {
	if DumpPrefix(karkivev1alpha1.EnginePostgres) != "pg_dump" {
		t.Fatal(DumpPrefix(karkivev1alpha1.EnginePostgres))
	}
	if DumpPrefix(karkivev1alpha1.EngineMariaDB) != "mysqldump" {
		t.Fatal(DumpPrefix(karkivev1alpha1.EngineMariaDB))
	}
	if DumpPrefix(karkivev1alpha1.EngineRedis) != "redisdump" {
		t.Fatal(DumpPrefix(karkivev1alpha1.EngineRedis))
	}
	if DefaultPort("") != 5432 {
		t.Fatal(DefaultPort(""))
	}
	if DefaultPort(karkivev1alpha1.EngineMariaDB) != 3306 {
		t.Fatal(DefaultPort(karkivev1alpha1.EngineMariaDB))
	}
	if DefaultPort(karkivev1alpha1.EngineRedis) != 6379 {
		t.Fatal(DefaultPort(karkivev1alpha1.EngineRedis))
	}
}

func testMariaBackup() *karkivev1alpha1.Backup {
	b := testBackup()
	b.Name = "app-mariadb"
	b.Spec.Engine = karkivev1alpha1.EngineMariaDB
	b.Spec.Database.Host = "mariadb.example.svc.cluster.local"
	b.Spec.S3.Path = "app/mysqldump"
	b.Spec.SecretRef.Name = "backup-app-mariadb"
	return b
}

func TestMutateBackupCronJob_MariaDBStages(t *testing.T) {
	cj := &batchv1.CronJob{}
	MutateBackupCronJob(cj, testMariaBackup(), config.Config{})
	got := containerNames(cj)
	want := []string{"cleanup", "mysqldump", "compress", "encrypt", "s3-sync"}
	if len(got) != len(want) {
		t.Fatalf("containers=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("containers=%v, want %v", got, want)
		}
	}
	dump := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1]
	if dump.Image != config.DefaultMariaDBImage {
		t.Errorf("mysqldump image=%s", dump.Image)
	}
	if !hasEnv(dump, "MYSQL_USER") || !hasEnv(dump, "MYSQL_PASSWORD") {
		t.Errorf("mysqldump missing MYSQL_USER/MYSQL_PASSWORD")
	}
	if len(dump.Command) < 3 || !strings.Contains(dump.Command[2], "--hex-blob") {
		t.Errorf("mysqldump script missing --hex-blob / utf8mb4 flags")
	}
}

func TestMutateBackupConfigMap_MariaDB(t *testing.T) {
	cm := &corev1.ConfigMap{}
	if err := MutateBackupConfigMap(cm, testMariaBackup(), config.Config{}); err != nil {
		t.Fatal(err)
	}
	if cm.Data["ENGINE"] != "mariadb" || cm.Data["DUMP_PREFIX"] != "mysqldump" {
		t.Errorf("engine/prefix=%q/%q", cm.Data["ENGINE"], cm.Data["DUMP_PREFIX"])
	}
	if cm.Data["MYSQL_HOST"] != "mariadb.example.svc.cluster.local" || cm.Data["MYSQL_PORT"] != "3306" {
		t.Errorf("mysql host/port=%q/%q", cm.Data["MYSQL_HOST"], cm.Data["MYSQL_PORT"])
	}
}

func testRedisBackup() *karkivev1alpha1.Backup {
	b := testBackup()
	b.Name = "cache-redis"
	b.Spec.Engine = karkivev1alpha1.EngineRedis
	b.Spec.Database.Host = "redis.example.svc.cluster.local"
	b.Spec.Database.Name = "cache"
	b.Spec.S3.Path = "cache/redisdump"
	b.Spec.SecretRef.Name = "backup-cache-redis"
	return b
}

func TestMutateBackupCronJob_RedisStages(t *testing.T) {
	cj := &batchv1.CronJob{}
	MutateBackupCronJob(cj, testRedisBackup(), config.Config{})
	got := containerNames(cj)
	want := []string{"cleanup", "redisdump", "compress", "encrypt", "s3-sync"}
	if len(got) != len(want) {
		t.Fatalf("containers=%v, want %v", got, want)
	}
	dump := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1]
	if dump.Image != config.DefaultRedisImage {
		t.Errorf("redisdump image=%s", dump.Image)
	}
	if !hasEnv(dump, "REDIS_USERNAME") || !hasEnv(dump, "REDIS_PASSWORD") {
		t.Errorf("redisdump missing REDIS_USERNAME/REDIS_PASSWORD")
	}
}

func testMariaRestore() *karkivev1alpha1.Restore {
	r := testRestore()
	r.Name = "app-mariadb"
	r.Spec.Engine = karkivev1alpha1.EngineMariaDB
	r.Spec.Database.Host = "mariadb.example.svc.cluster.local"
	r.Spec.Database.OwnerRole = ""
	r.Spec.S3.Path = "app/mysqldump"
	r.Spec.PostgresSecret = nil
	r.Spec.MariaDBSecret = &karkivev1alpha1.SecretKeySelector{Name: "mariadb"}
	return r
}

func TestMutateRestoreCronJob_MariaDBStages(t *testing.T) {
	cj := &batchv1.CronJob{}
	MutateRestoreCronJob(cj, testMariaRestore(), config.Config{})
	got := containerNames(cj)
	want := []string{"cleanup", "fetch", "decrypt", "extract", "mysqlrestore"}
	if len(got) != len(want) {
		t.Fatalf("containers=%v, want %v", got, want)
	}
	restorec := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[4]
	if !hasEnv(restorec, "MYSQL_USER") || !hasEnv(restorec, "MYSQL_PASSWORD") {
		t.Errorf("mysqlrestore missing MYSQL_USER/MYSQL_PASSWORD")
	}
	if len(restorec.Command) < 3 || !strings.Contains(restorec.Command[2], "GTID_PURGED") {
		t.Errorf("mysqlrestore script missing GTID/DEFINER filter")
	}
}

func testRedisRestore() *karkivev1alpha1.Restore {
	r := testRestore()
	r.Name = "cache-redis"
	r.Spec.Engine = karkivev1alpha1.EngineRedis
	r.Spec.Database.Host = "redis.example.svc.cluster.local"
	r.Spec.Database.Name = "cache"
	r.Spec.S3.Path = "cache/redisdump"
	r.Spec.PostgresSecret = nil
	r.Spec.RedisSecret = &karkivev1alpha1.SecretKeySelector{Name: "redis"}
	return r
}

func TestMutateRestoreCronJob_RedisStages(t *testing.T) {
	cj := &batchv1.CronJob{}
	MutateRestoreCronJob(cj, testRedisRestore(), config.Config{})
	got := containerNames(cj)
	want := []string{"cleanup", "fetch", "decrypt", "extract", "redisrestore"}
	if len(got) != len(want) {
		t.Fatalf("containers=%v, want %v", got, want)
	}
	restorec := cj.Spec.JobTemplate.Spec.Template.Spec.Containers[4]
	if len(restorec.Command) < 3 || !strings.Contains(restorec.Command[2], "REPLICAOF") {
		t.Errorf("redisrestore script missing REPLICAOF")
	}
	if strings.Contains(restorec.Command[2], "MIGRATE") {
		t.Errorf("redisrestore script still uses SCAN+MIGRATE")
	}
}

func containerNames(cj *batchv1.CronJob) []string {
	cs := cj.Spec.JobTemplate.Spec.Template.Spec.Containers
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		names = append(names, c.Name)
	}
	return names
}

func hasEnv(c corev1.Container, name string) bool {
	for _, e := range c.Env {
		if e.Name == name {
			return true
		}
	}
	return false
}

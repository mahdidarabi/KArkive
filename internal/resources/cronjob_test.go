package resources

import (
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/config"
	"github.com/mahdidarabi/Karkive/internal/ptr"
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
	if *cj.Spec.JobTemplate.Spec.BackoffLimit != 1 {
		t.Errorf("backoffLimit=%v", cj.Spec.JobTemplate.Spec.BackoffLimit)
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
	if DefaultPort("") != 5432 {
		t.Fatal(DefaultPort(""))
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

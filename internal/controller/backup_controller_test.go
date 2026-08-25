package controller

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
	"github.com/mahdidarabi/KArkive/internal/config"
	"github.com/mahdidarabi/KArkive/internal/resources"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := karkivev1alpha1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBackupReconcile_CreatesOwnedResources(t *testing.T) {
	scheme := testScheme(t)
	backup := &karkivev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup"},
		Spec: karkivev1alpha1.BackupSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "0 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef: corev1.LocalObjectReference{Name: "backup-creds"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"username":       []byte("app"),
			"password":       []byte("secret"),
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup, secret).
		WithStatusSubresource(&karkivev1alpha1.Backup{}).
		Build()

	r := &BackupReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
		Config:   config.Config{},
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}

	cm := &corev1.ConfigMap{}
	owned := resources.BackupOwnedName(backup)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: backup.Namespace, Name: owned}, cm); err != nil {
		t.Fatalf("configmap: %v", err)
	}
	if cm.Data["PGDATABASE"] != "app" {
		t.Errorf("PGDATABASE=%q", cm.Data["PGDATABASE"])
	}
	if cm.Name != "karkive-app-postgres" {
		t.Errorf("configmap name=%q", cm.Name)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: backup.Namespace, Name: owned}, pvc); err != nil {
		t.Fatalf("pvc: %v", err)
	}

	cj := &batchv1.CronJob{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: backup.Namespace, Name: owned}, cj); err != nil {
		t.Fatalf("cronjob: %v", err)
	}
	if len(cj.Spec.JobTemplate.Spec.Template.Spec.Containers) != 5 {
		t.Fatalf("expected 5 containers, got %d", len(cj.Spec.JobTemplate.Spec.Template.Spec.Containers))
	}

	updated := &karkivev1alpha1.Backup{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(backup), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != karkivev1alpha1.BackupPhaseReady {
		t.Errorf("phase=%q", updated.Status.Phase)
	}
	if updated.Status.CronJobName != owned {
		t.Errorf("status.cronJobName=%q", updated.Status.CronJobName)
	}
	cond := meta.FindStatusCondition(updated.Status.Conditions, conditionReady)
	if cond == nil || cond.Status != metav1.ConditionTrue {
		t.Errorf("ready condition=%v", cond)
	}
}

func TestBackupReconcile_S3DisabledSkipsSyncAndS3Keys(t *testing.T) {
	scheme := testScheme(t)
	enabled := false
	backup := &karkivev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup"},
		Spec: karkivev1alpha1.BackupSpec{
			Engine:    karkivev1alpha1.EnginePostgres,
			Schedule:  "0 2 * * *",
			Database:  karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3:        karkivev1alpha1.S3Spec{Enabled: &enabled},
			SecretRef: corev1.LocalObjectReference{Name: "backup-creds"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"username":       []byte("app"),
			"password":       []byte("secret"),
			"gpg_passphrase": []byte("pgp"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup, secret).
		WithStatusSubresource(&karkivev1alpha1.Backup{}).
		Build()
	r := &BackupReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
		Config:   config.Config{},
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace},
	}); err != nil {
		t.Fatal(err)
	}

	owned := resources.BackupOwnedName(backup)
	cj := &batchv1.CronJob{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: backup.Namespace, Name: owned}, cj); err != nil {
		t.Fatalf("cronjob: %v", err)
	}
	if n := len(cj.Spec.JobTemplate.Spec.Template.Spec.Containers); n != 4 {
		t.Fatalf("expected 4 containers without s3-sync, got %d", n)
	}

	cm := &corev1.ConfigMap{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: backup.Namespace, Name: owned}, cm); err != nil {
		t.Fatalf("configmap: %v", err)
	}
	if cm.Data["S3_ENABLED"] != "false" {
		t.Errorf("S3_ENABLED=%q", cm.Data["S3_ENABLED"])
	}

	updated := &karkivev1alpha1.Backup{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(backup), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != karkivev1alpha1.BackupPhaseReady {
		t.Errorf("phase=%q", updated.Status.Phase)
	}
}

func TestBackupReconcile_RedisCreatesOwnedResources(t *testing.T) {
	scheme := testScheme(t)
	backup := &karkivev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "cache-redis", Namespace: "backup"},
		Spec: karkivev1alpha1.BackupSpec{
			Engine:   karkivev1alpha1.EngineRedis,
			Schedule: "0 */12 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "redis.example.svc.cluster.local", Name: "cache"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "cache/redisdump",
			},
			SecretRef: corev1.LocalObjectReference{Name: "backup-creds"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"username":       []byte("default"),
			"password":       []byte("secret"),
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup, secret).
		WithStatusSubresource(&karkivev1alpha1.Backup{}).
		Build()
	r := &BackupReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := &karkivev1alpha1.Backup{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(backup), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != karkivev1alpha1.BackupPhaseReady {
		t.Errorf("phase=%q", updated.Status.Phase)
	}
	cj := &batchv1.CronJob{}
	owned := resources.BackupOwnedName(backup)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: backup.Namespace, Name: owned}, cj); err != nil {
		t.Fatal(err)
	}
	if cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1].Name != "redisdump" {
		t.Errorf("dump container=%q", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[1].Name)
	}
}

func TestBackupReconcile_CopiesLastJobFailure(t *testing.T) {
	scheme := testScheme(t)
	backup := &karkivev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup"},
		Spec: karkivev1alpha1.BackupSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "0 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef: corev1.LocalObjectReference{Name: "backup-creds"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"username":       []byte("app"),
			"password":       []byte("secret"),
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}
	failedAt := metav1.Now()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "karkive-app-postgres-1",
			Namespace: "backup",
			Labels: map[string]string{
				resources.LabelAppManagedBy: resources.ManagedBy,
				resources.LabelKind:         resources.KindBackup,
				resources.LabelBackupName:   "app-postgres",
			},
		},
		Status: batchv1.JobStatus{
			Failed: 1,
			Conditions: []batchv1.JobCondition{{
				Type:               batchv1.JobFailed,
				Status:             corev1.ConditionTrue,
				Reason:             "BackoffLimitExceeded",
				Message:            "Job has reached the specified backoff limit",
				LastTransitionTime: failedAt,
			}},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup, secret, job).
		WithStatusSubresource(&karkivev1alpha1.Backup{}).
		Build()
	r := &BackupReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}
	updated := &karkivev1alpha1.Backup{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(backup), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.LastJob == nil {
		t.Fatal("expected lastJob")
	}
	if updated.Status.LastJob.Outcome != karkivev1alpha1.LastJobOutcomeFailed {
		t.Errorf("outcome=%q", updated.Status.LastJob.Outcome)
	}
	if updated.Status.LastJob.Reason != "BackoffLimitExceeded" {
		t.Errorf("reason=%q", updated.Status.LastJob.Reason)
	}
}

func TestBackupReconcile_DoesNotRecreateWhileDeleting(t *testing.T) {
	scheme := testScheme(t)
	now := metav1.Now()
	backup := &karkivev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "app-postgres",
			Namespace:         "backup",
			DeletionTimestamp: &now,
			Finalizers:        []string{"foregroundDeletion"},
		},
		Spec: karkivev1alpha1.BackupSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "0 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef: corev1.LocalObjectReference{Name: "backup-creds"},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "backup-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"username":       []byte("app"),
			"password":       []byte("secret"),
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(backup, secret).
		WithStatusSubresource(&karkivev1alpha1.Backup{}).
		Build()
	r := &BackupReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace},
	}); err != nil {
		t.Fatal(err)
	}
	owned := resources.BackupOwnedName(backup)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: backup.Namespace, Name: owned}, &batchv1.CronJob{}); err == nil {
		t.Fatal("expected no CronJob while Backup is terminating")
	}
}

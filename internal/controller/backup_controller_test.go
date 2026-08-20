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

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/config"
	"github.com/mahdidarabi/Karkive/internal/resources"
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

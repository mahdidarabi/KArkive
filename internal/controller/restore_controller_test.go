package controller

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
	"github.com/mahdidarabi/KArkive/internal/config"
	"github.com/mahdidarabi/KArkive/internal/ptr"
	"github.com/mahdidarabi/KArkive/internal/resources"
)

func TestRestoreReconcile_CreatesOwnedResources(t *testing.T) {
	scheme := testScheme(t)
	restore := &karkivev1alpha1.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup"},
		Spec: karkivev1alpha1.RestoreSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "30 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef: corev1.LocalObjectReference{Name: "restore-creds"},
			PostgresSecret: &karkivev1alpha1.SecretKeySelector{
				Name: "postgres",
			},
			Persistence: &karkivev1alpha1.PersistenceSpec{Enabled: ptr.To(false)},
		},
	}
	jobSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "restore-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}
	pgSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres", Namespace: "backup"},
		Data: map[string][]byte{
			"username": []byte("postgres"),
			"password": []byte("secret"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, jobSecret, pgSecret).
		WithStatusSubresource(&karkivev1alpha1.Restore{}).
		Build()

	r := &RestoreReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
		Config:   config.Config{},
	}

	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}

	owned := resources.RestoreOwnedName(restore)
	cm := &corev1.ConfigMap{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: restore.Namespace, Name: owned}, cm); err != nil {
		t.Fatalf("configmap: %v", err)
	}
	if cm.Data["PGDATABASE"] != "app" {
		t.Errorf("PGDATABASE=%q", cm.Data["PGDATABASE"])
	}
	if cm.Data["WORKDIR"] != "/workdir" {
		t.Errorf("WORKDIR=%q", cm.Data["WORKDIR"])
	}
	if cm.Name != "karkive-restore-app-postgres" {
		t.Errorf("configmap name=%q", cm.Name)
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: restore.Namespace, Name: owned}, pvc); err == nil {
		t.Fatal("did not expect a PVC when persistence.enabled=false")
	}

	cj := &batchv1.CronJob{}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: restore.Namespace, Name: owned}, cj); err != nil {
		t.Fatalf("cronjob: %v", err)
	}
	if len(cj.Spec.JobTemplate.Spec.Template.Spec.Containers) != 5 {
		t.Fatalf("expected 5 containers, got %d", len(cj.Spec.JobTemplate.Spec.Template.Spec.Containers))
	}

	updated := &karkivev1alpha1.Restore{}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(restore), updated); err != nil {
		t.Fatal(err)
	}
	if updated.Status.Phase != karkivev1alpha1.RestorePhaseReady {
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

func TestRestoreReconcile_DoesNotRecreateWhileDeleting(t *testing.T) {
	scheme := testScheme(t)
	now := metav1.Now()
	restore := &karkivev1alpha1.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "app-postgres",
			Namespace:         "backup",
			DeletionTimestamp: &now,
			Finalizers:        []string{"foregroundDeletion"},
		},
		Spec: karkivev1alpha1.RestoreSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "30 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef:      corev1.LocalObjectReference{Name: "restore-creds"},
			PostgresSecret: &karkivev1alpha1.SecretKeySelector{Name: "postgres"},
			Persistence:    &karkivev1alpha1.PersistenceSpec{Enabled: ptr.To(false)},
		},
	}
	jobSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "restore-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres", Namespace: "backup"},
		Data: map[string][]byte{
			"username": []byte("app"),
			"password": []byte("secret"),
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, jobSecret, dbSecret).
		WithStatusSubresource(&karkivev1alpha1.Restore{}).
		Build()
	r := &RestoreReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace},
	}); err != nil {
		t.Fatal(err)
	}
	owned := resources.RestoreOwnedName(restore)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: restore.Namespace, Name: owned}, &batchv1.CronJob{}); err == nil {
		t.Fatal("expected no CronJob while Restore is terminating")
	}
}

func TestRestoreReconcile_MariaDBCreatesOwnedResources(t *testing.T) {
	scheme := testScheme(t)
	restore := &karkivev1alpha1.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "app-mariadb", Namespace: "backup"},
		Spec: karkivev1alpha1.RestoreSpec{
			Engine:   karkivev1alpha1.EngineMariaDB,
			Schedule: "30 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "mariadb.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/mysqldump",
			},
			SecretRef:     corev1.LocalObjectReference{Name: "restore-creds"},
			MariaDBSecret: &karkivev1alpha1.SecretKeySelector{Name: "mariadb"},
			Persistence:   &karkivev1alpha1.PersistenceSpec{Enabled: ptr.To(false)},
		},
	}
	jobSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "restore-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}
	dbSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "mariadb", Namespace: "backup"},
		Data: map[string][]byte{
			"username": []byte("root"),
			"password": []byte("secret"),
		},
	}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, jobSecret, dbSecret).
		WithStatusSubresource(&karkivev1alpha1.Restore{}).
		Build()
	r := &RestoreReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
		Config:   config.Config{},
	}
	_, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace},
	})
	if err != nil {
		t.Fatal(err)
	}
	cj := &batchv1.CronJob{}
	owned := resources.RestoreOwnedName(restore)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: restore.Namespace, Name: owned}, cj); err != nil {
		t.Fatal(err)
	}
	if cj.Spec.JobTemplate.Spec.Template.Spec.Containers[4].Name != "mysqlrestore" {
		t.Errorf("restore container=%q", cj.Spec.JobTemplate.Spec.Template.Spec.Containers[4].Name)
	}
}

func TestRestoreReconcile_DeletesLegacyOwnedNames(t *testing.T) {
	scheme := testScheme(t)
	restore := &karkivev1alpha1.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup", UID: types.UID("restore-uid"), Generation: 1},
		Spec: karkivev1alpha1.RestoreSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "30 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef:      corev1.LocalObjectReference{Name: "restore-creds"},
			PostgresSecret: &karkivev1alpha1.SecretKeySelector{Name: "postgres"},
			Persistence:    &karkivev1alpha1.PersistenceSpec{Enabled: ptr.To(false)},
		},
	}
	jobSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "restore-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}
	pgSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres", Namespace: "backup"},
		Data:       map[string][]byte{"username": []byte("postgres"), "password": []byte("secret")},
	}
	legacy := resources.LegacyOwnedName(restore.Name)
	owners := []metav1.OwnerReference{{
		APIVersion: karkivev1alpha1.GroupVersion.String(),
		Kind:       "Restore",
		Name:       restore.Name,
		UID:        restore.UID,
		Controller: ptr.To(true),
	}}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, jobSecret, pgSecret,
			&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: legacy, Namespace: restore.Namespace, OwnerReferences: owners}},
			&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: legacy, Namespace: restore.Namespace, OwnerReferences: owners}},
		).
		WithStatusSubresource(&karkivev1alpha1.Restore{}).
		Build()
	r := &RestoreReconciler{Client: c, Scheme: scheme, Recorder: record.NewFakeRecorder(8), Config: config.Config{}}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace},
	}); err != nil {
		t.Fatal(err)
	}
	owned := resources.RestoreOwnedName(restore)
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: restore.Namespace, Name: owned}, &batchv1.CronJob{}); err != nil {
		t.Fatalf("new CronJob: %v", err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: restore.Namespace, Name: legacy}, &batchv1.CronJob{}); err == nil {
		t.Fatal("expected legacy CronJob to be deleted")
	}
}

func TestRestoreReconcile_SkipsNoopStatusAndEvents(t *testing.T) {
	scheme := testScheme(t)
	restore := &karkivev1alpha1.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup", Generation: 1},
		Spec: karkivev1alpha1.RestoreSpec{
			Engine:   karkivev1alpha1.EnginePostgres,
			Schedule: "30 2 * * *",
			Database: karkivev1alpha1.DatabaseSpec{Host: "postgres.example.svc.cluster.local", Name: "app"},
			S3: karkivev1alpha1.S3Spec{
				Endpoint: "https://s3.example.com",
				Bucket:   "backups",
				Path:     "app/pgdump",
			},
			SecretRef:      corev1.LocalObjectReference{Name: "restore-creds"},
			PostgresSecret: &karkivev1alpha1.SecretKeySelector{Name: "postgres"},
			Persistence:    &karkivev1alpha1.PersistenceSpec{Enabled: ptr.To(false)},
		},
	}
	jobSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "restore-creds", Namespace: "backup"},
		Data: map[string][]byte{
			"s3_access_key":  []byte("ak"),
			"s3_secret_key":  []byte("sk"),
			"gpg_passphrase": []byte("pgp"),
		},
	}
	pgSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "postgres", Namespace: "backup"},
		Data:       map[string][]byte{"username": []byte("postgres"), "password": []byte("secret")},
	}
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(restore, jobSecret, pgSecret).
		WithStatusSubresource(&karkivev1alpha1.Restore{}).
		Build()
	c := &statusCountClient{Client: base}
	rec := record.NewFakeRecorder(16)
	r := &RestoreReconciler{Client: c, Scheme: scheme, Recorder: rec, Config: config.Config{}}
	req := reconcile.Request{NamespacedName: types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace}}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if c.updates != 1 {
		t.Fatalf("status updates after first reconcile=%d, want 1", c.updates)
	}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if c.updates != 1 {
		t.Fatalf("status updates after second reconcile=%d, want 1", c.updates)
	}
	if extra := drainEvents(rec); len(extra) != 1 {
		t.Fatalf("expected one Synced event total, got %v", extra)
	}
}

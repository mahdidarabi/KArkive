package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/ptr"
	"github.com/mahdidarabi/Karkive/internal/resources"
)

func TestCollector_BackupAndLastJob(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := karkivev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	success := metav1.NewTime(time.Unix(1_700_000_000, 0))
	start := metav1.NewTime(success.Add(-2 * time.Minute))
	backup := &karkivev1alpha1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "app-postgres", Namespace: "backup"},
		Spec: karkivev1alpha1.BackupSpec{
			Engine:  karkivev1alpha1.EnginePostgres,
			Suspend: ptr.To(true),
		},
		Status: karkivev1alpha1.BackupStatus{
			Phase:              karkivev1alpha1.BackupPhaseReady,
			LastSuccessfulTime: &success,
			LastScheduleTime:   &success,
		},
	}
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
			Succeeded:      1,
			StartTime:      &start,
			CompletionTime: &success,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(backup, job).Build()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(&Collector{Client: c})
	fams, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]float64{}
	for _, fam := range fams {
		for _, m := range fam.Metric {
			if match(m, "backup", "app-postgres") {
				got[fam.GetName()] = m.GetGauge().GetValue()
			}
		}
	}
	if got["karkive_backup_ready"] != 1 {
		t.Errorf("ready=%v", got["karkive_backup_ready"])
	}
	if got["karkive_backup_suspended"] != 1 {
		t.Errorf("suspended=%v", got["karkive_backup_suspended"])
	}
	if got["karkive_backup_last_successful_timestamp_seconds"] != float64(success.Unix()) {
		t.Errorf("last success=%v", got["karkive_backup_last_successful_timestamp_seconds"])
	}
	if got["karkive_backup_last_job_failed"] != 0 {
		t.Errorf("last job failed=%v", got["karkive_backup_last_job_failed"])
	}
	if got["karkive_backup_last_job_duration_seconds"] != 120 {
		t.Errorf("duration=%v", got["karkive_backup_last_job_duration_seconds"])
	}
}

func match(m *dto.Metric, namespace, name string) bool {
	var ns, n string
	for _, l := range m.Label {
		switch l.GetName() {
		case "namespace":
			ns = l.GetValue()
		case "name":
			n = l.GetValue()
		}
	}
	return ns == namespace && n == name
}

func TestCollector_NoSeriesWithoutCRs(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := karkivev1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	reg := prometheus.NewPedanticRegistry()
	reg.MustRegister(&Collector{Client: c})
	fams, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range fams {
		if strings.HasPrefix(fam.GetName(), "karkive_") && len(fam.Metric) > 0 {
			t.Fatalf("unexpected %s series", fam.GetName())
		}
	}
}

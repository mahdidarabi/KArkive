package jobstatus

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
	"github.com/mahdidarabi/KArkive/internal/resources"
)

func TestSummarizeFailedJob(t *testing.T) {
	start := metav1.NewTime(time.Unix(1_700_000_000, 0))
	failedAt := metav1.NewTime(start.Add(45 * time.Second))
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "karkive-app-1", Namespace: "ns"},
		Status: batchv1.JobStatus{
			Failed:    1,
			StartTime: &start,
			Conditions: []batchv1.JobCondition{{
				Type:               batchv1.JobFailed,
				Status:             corev1.ConditionTrue,
				Reason:             "BackoffLimitExceeded",
				Message:            "Job has reached the specified backoff limit",
				LastTransitionTime: failedAt,
			}},
		},
	}
	got := Summarize(job)
	if got.Name != "karkive-app-1" {
		t.Errorf("name=%q", got.Name)
	}
	if got.Outcome != karkivev1alpha1.LastJobOutcomeFailed {
		t.Errorf("outcome=%q", got.Outcome)
	}
	if got.Reason != "BackoffLimitExceeded" {
		t.Errorf("reason=%q", got.Reason)
	}
	if got.Message == "" {
		t.Error("expected message")
	}
	if got.CompletionTime == nil || !got.CompletionTime.Equal(&failedAt) {
		t.Errorf("completion=%v", got.CompletionTime)
	}
}

func TestLastFinishedPicksLatest(t *testing.T) {
	t1 := metav1.NewTime(time.Unix(1_700_000_000, 0))
	t2 := metav1.NewTime(t1.Add(time.Hour))
	jobs := []batchv1.Job{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "old",
				Namespace: "ns",
				Labels: map[string]string{
					resources.LabelKind:       resources.KindBackup,
					resources.LabelBackupName: "app",
				},
			},
			Status: batchv1.JobStatus{CompletionTime: &t1, Succeeded: 1},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "new",
				Namespace: "ns",
				Labels: map[string]string{
					resources.LabelKind:       resources.KindBackup,
					resources.LabelBackupName: "app",
				},
			},
			Status: batchv1.JobStatus{CompletionTime: &t2, Succeeded: 1},
		},
	}
	got := LastFinished(jobs, "ns", resources.LabelBackupName, "app", resources.KindBackup)
	if got == nil || got.Name != "new" {
		t.Fatalf("got %+v", got)
	}
}

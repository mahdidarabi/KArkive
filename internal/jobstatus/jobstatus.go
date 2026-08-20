package jobstatus

import (
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/Karkive/api/v1alpha1"
	"github.com/mahdidarabi/Karkive/internal/resources"
)

// LastFinished returns the Job that finished most recently among those matching
// namespace, kind, and name labels. Jobs that have not finished are ignored.
func LastFinished(jobs []batchv1.Job, namespace, nameLabel, name, kind string) *batchv1.Job {
	var best *batchv1.Job
	var bestAt time.Time
	for i := range jobs {
		job := &jobs[i]
		if job.Namespace != namespace {
			continue
		}
		if job.Labels[nameLabel] != name {
			continue
		}
		if kind != "" && job.Labels[resources.LabelKind] != kind {
			continue
		}
		end := FinishedAt(job)
		if end == nil {
			continue
		}
		if best == nil || end.After(bestAt) {
			best = job
			bestAt = end.Time
		}
	}
	return best
}

// Summarize copies success/failure into a CR status fragment.
func Summarize(job *batchv1.Job) *karkivev1alpha1.LastJobStatus {
	if job == nil {
		return nil
	}
	out := &karkivev1alpha1.LastJobStatus{Name: job.Name}
	if job.Status.StartTime != nil && !job.Status.StartTime.IsZero() {
		t := *job.Status.StartTime
		out.StartTime = &t
	}
	if end := FinishedAt(job); end != nil {
		t := *end
		out.CompletionTime = &t
	}
	if Failed(job) {
		out.Outcome = karkivev1alpha1.LastJobOutcomeFailed
		out.Reason, out.Message = condition(job, batchv1.JobFailed)
		return out
	}
	out.Outcome = karkivev1alpha1.LastJobOutcomeSucceeded
	out.Reason, out.Message = condition(job, batchv1.JobComplete)
	return out
}

func FinishedAt(job *batchv1.Job) *metav1.Time {
	if job.Status.CompletionTime != nil && !job.Status.CompletionTime.IsZero() {
		return job.Status.CompletionTime
	}
	for i := range job.Status.Conditions {
		c := job.Status.Conditions[i]
		if c.Status != corev1.ConditionTrue {
			continue
		}
		if c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed {
			if !c.LastTransitionTime.IsZero() {
				return &c.LastTransitionTime
			}
		}
	}
	return nil
}

func Failed(job *batchv1.Job) bool {
	if job.Status.Failed > 0 {
		return true
	}
	for i := range job.Status.Conditions {
		c := job.Status.Conditions[i]
		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func condition(job *batchv1.Job, typ batchv1.JobConditionType) (reason, message string) {
	for i := range job.Status.Conditions {
		c := job.Status.Conditions[i]
		if c.Type == typ && c.Status == corev1.ConditionTrue {
			return c.Reason, c.Message
		}
	}
	return "", ""
}

package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
)

func lastJobEqual(a, b *karkivev1alpha1.LastJobStatus) bool {
	return equality.Semantic.DeepEqual(a, b)
}

func metaTimeEqual(a, b *metav1.Time) bool {
	return equality.Semantic.DeepEqual(a, b)
}

// setJobSucceededCondition writes BackupSucceeded / RestoreSucceeded from lastJob.
// Unknown until a Job has finished. Does not change Ready or phase.
func setJobSucceededCondition(conditions *[]metav1.Condition, condType string, generation int64, lastJob *karkivev1alpha1.LastJobStatus) bool {
	cond := metav1.Condition{
		Type:               condType,
		ObservedGeneration: generation,
	}
	switch {
	case lastJob == nil:
		cond.Status = metav1.ConditionUnknown
		cond.Reason = "NoFinishedJob"
		cond.Message = "No finished Job yet"
	case lastJob.Outcome == karkivev1alpha1.LastJobOutcomeSucceeded:
		cond.Status = metav1.ConditionTrue
		cond.Reason = "JobSucceeded"
		if lastJob.Name != "" {
			cond.Message = fmt.Sprintf("Job %s succeeded", lastJob.Name)
		} else {
			cond.Message = "Last Job succeeded"
		}
	default:
		cond.Status = metav1.ConditionFalse
		cond.Reason = lastJob.Reason
		if cond.Reason == "" {
			cond.Reason = "JobFailed"
		}
		if lastJob.Message != "" {
			cond.Message = lastJob.Message
		} else if lastJob.Name != "" {
			cond.Message = fmt.Sprintf("Job %s failed", lastJob.Name)
		} else {
			cond.Message = "Last Job failed"
		}
	}
	return meta.SetStatusCondition(conditions, cond)
}

// generationChanged is true on create (status not yet observed) or when spec
// generation has moved. Generation 0 + empty phase covers fake clients that
// do not bump metadata.generation.
func generationChanged(generation, observed int64, currentPhase string) bool {
	if observed != generation {
		return true
	}
	return generation == 0 && observed == 0 && currentPhase == ""
}

// shouldRecordEvent implements "event only on create or spec change" for Normal
// events. Warnings also fire when phase or the Ready condition actually change
// (e.g. a Secret disappears) so operators still see the transition.
func shouldRecordEvent(eventType string, specChanged, phaseChanged, condChanged bool) bool {
	if eventType == corev1.EventTypeNormal {
		return specChanged
	}
	return specChanged || phaseChanged || condChanged
}

func statusNeedsPatch(specChanged, phaseChanged, condChanged, lastJobChanged, cronChanged bool) bool {
	return specChanged || phaseChanged || condChanged || lastJobChanged || cronChanged
}

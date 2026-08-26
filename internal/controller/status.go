package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
)

func lastJobEqual(a, b *karkivev1alpha1.LastJobStatus) bool {
	return equality.Semantic.DeepEqual(a, b)
}

func metaTimeEqual(a, b *metav1.Time) bool {
	return equality.Semantic.DeepEqual(a, b)
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

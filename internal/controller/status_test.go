package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	karkivev1alpha1 "github.com/mahdidarabi/KArkive/api/v1alpha1"
)

func TestGenerationChanged(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		gen, obs int64
		phase    string
		want     bool
	}{
		{name: "create", gen: 1, obs: 0, phase: "", want: true},
		{name: "fake client create", gen: 0, obs: 0, phase: "", want: true},
		{name: "steady", gen: 1, obs: 1, phase: karkivev1alpha1.BackupPhaseReady, want: false},
		{name: "spec bump", gen: 2, obs: 1, phase: karkivev1alpha1.BackupPhaseReady, want: true},
		{name: "fake client steady", gen: 0, obs: 0, phase: karkivev1alpha1.BackupPhaseReady, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := generationChanged(tc.gen, tc.obs, tc.phase); got != tc.want {
				t.Fatalf("generationChanged()=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestShouldRecordEvent(t *testing.T) {
	t.Parallel()
	if shouldRecordEvent(corev1.EventTypeNormal, false, true, true) {
		t.Fatal("Synced must not fire when spec is unchanged")
	}
	if !shouldRecordEvent(corev1.EventTypeNormal, true, false, false) {
		t.Fatal("Synced must fire on create or spec change")
	}
	if !shouldRecordEvent(corev1.EventTypeWarning, false, true, false) {
		t.Fatal("warnings must fire when phase changes")
	}
	if shouldRecordEvent(corev1.EventTypeWarning, false, false, false) {
		t.Fatal("warnings must not repeat when status is unchanged")
	}
}

func TestStatusNeedsPatch(t *testing.T) {
	t.Parallel()
	if statusNeedsPatch(false, false, false, false, false) {
		t.Fatal("expected no patch")
	}
	if !statusNeedsPatch(false, false, false, true, false) {
		t.Fatal("lastJob change must patch")
	}
	if !statusNeedsPatch(false, true, false, false, false) {
		t.Fatal("phase change must patch")
	}
}

func TestLastJobEqual(t *testing.T) {
	t.Parallel()
	a := &karkivev1alpha1.LastJobStatus{Name: "job-1", Outcome: karkivev1alpha1.LastJobOutcomeSucceeded}
	b := &karkivev1alpha1.LastJobStatus{Name: "job-1", Outcome: karkivev1alpha1.LastJobOutcomeSucceeded}
	if !lastJobEqual(a, b) {
		t.Fatal("expected equal")
	}
	b.Outcome = karkivev1alpha1.LastJobOutcomeFailed
	if lastJobEqual(a, b) {
		t.Fatal("expected not equal")
	}
	if !lastJobEqual(nil, nil) {
		t.Fatal("nil lastJob should be equal")
	}
}

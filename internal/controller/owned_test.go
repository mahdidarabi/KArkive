package controller

import (
	"errors"
	"testing"
)

func TestOwnedReason(t *testing.T) {
	t.Parallel()
	if got := ownedReason(&ownedApplyError{Reason: "PVCError", Err: errors.New("quota")}); got != "PVCError" {
		t.Fatalf("ownedReason=%q", got)
	}
	if got := ownedReason(errors.New("other")); got != "OwnedResourceError" {
		t.Fatalf("ownedReason=%q", got)
	}
}

package main

import (
	"testing"
	"time"
)

func TestPendingStoreRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	pa := &PendingAction{ElevatorID: 10054, InspectionDate: "2026-06-30", InspectionType: "Periodic"}

	t.Cleanup(func() { clearPending(4242) })

	// Nothing stored yet.
	if got := loadPending(4242, now); got != nil {
		t.Fatalf("expected nil before store, got %+v", got)
	}
	// Store then load within TTL.
	storePending(4242, pa, now)
	got := loadPending(4242, now.Add(time.Minute))
	if got == nil || got.ElevatorID != 10054 {
		t.Fatalf("expected stored pending for 10054, got %+v", got)
	}
	// Expires after the TTL window (and is evicted).
	if got := loadPending(4242, now.Add(pendingActionTTL+time.Second)); got != nil {
		t.Fatalf("expected nil after TTL, got %+v", got)
	}
	// Re-store then explicit clear.
	storePending(4242, pa, now)
	clearPending(4242)
	if got := loadPending(4242, now); got != nil {
		t.Fatalf("expected nil after clear, got %+v", got)
	}
}

func TestPendingStoreIgnoresUnknownConversation(t *testing.T) {
	now := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	// conversation id 0 (no conversation yet) must never store or load.
	storePending(0, &PendingAction{ElevatorID: 1}, now)
	if got := loadPending(0, now); got != nil {
		t.Fatalf("conversation id 0 must not store/load, got %+v", got)
	}
}

package scheduling

import (
	"testing"
	"time"
)

func TestFeedbackSettlesExactlyOnce(t *testing.T) {
	store := NewFeedbackStore(0.2)
	start := time.Unix(100, 0)
	lease := store.Begin("endpoint-a", start)
	if got := store.Signals("endpoint-a")[SignalInflight].Value; got != 1 {
		t.Fatalf("inflight after begin = %v, want 1", got)
	}
	if !lease.Complete(start.Add(time.Second), start.Add(100*time.Millisecond), true) {
		t.Fatal("first Complete() returned false")
	}
	if lease.Complete(start.Add(2*time.Second), start.Add(200*time.Millisecond), false) {
		t.Fatal("duplicate Complete() returned true")
	}
	state, ok := store.Snapshot("endpoint-a")
	if !ok {
		t.Fatal("feedback state missing")
	}
	if state.Inflight != 0 || state.FailureEWMA != 0.2 {
		t.Fatalf("state after completion = %+v", state)
	}
	if state.TTFT != 100*time.Millisecond || state.Latency != time.Second {
		t.Fatalf("timings = %+v", state)
	}
}

func TestFeedbackSuccessCancellationAndEWMA(t *testing.T) {
	store := NewFeedbackStore(0.5)
	start := time.Unix(200, 0)
	store.Begin("endpoint-a", start).Complete(start.Add(time.Second), start.Add(10*time.Millisecond), true)
	store.Begin("endpoint-a", start.Add(2*time.Second)).Complete(start.Add(3*time.Second), time.Time{}, false)
	state, _ := store.Snapshot("endpoint-a")
	if state.FailureEWMA != 0.25 {
		t.Fatalf("failure EWMA = %v, want 0.25", state.FailureEWMA)
	}
	if state.TTFT != 10*time.Millisecond {
		t.Fatalf("missing TTFT observation overwrote prior value: %v", state.TTFT)
	}
}

func TestFeedbackCleanupKeepsInflightOnly(t *testing.T) {
	store := NewFeedbackStore(0.2)
	now := time.Unix(300, 0)
	activeLease := store.Begin("active-request", now)
	store.Begin("stale", now).Complete(now.Add(time.Second), time.Time{}, false)
	store.Cleanup(map[string]struct{}{})
	if store.Size() != 1 {
		t.Fatalf("store size = %d, want only inflight endpoint", store.Size())
	}
	activeLease.Complete(now.Add(time.Second), time.Time{}, true)
	store.Cleanup(map[string]struct{}{})
	if store.Size() != 0 {
		t.Fatalf("store size = %d after completion, want 0", store.Size())
	}
}

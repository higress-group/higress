package scheduling

import (
	"sync"
	"time"
)

type EndpointFeedback struct {
	Inflight    int
	FailureEWMA float64
	TTFT        time.Duration
	Latency     time.Duration
}

type FeedbackStore struct {
	mu        sync.Mutex
	alpha     float64
	endpoints map[string]*EndpointFeedback
}

type FeedbackLease struct {
	store     *FeedbackStore
	address   string
	startedAt time.Time
	completed bool
}

func NewFeedbackStore(alpha float64) *FeedbackStore {
	return &FeedbackStore{alpha: alpha, endpoints: map[string]*EndpointFeedback{}}
}

func (s *FeedbackStore) Signals(address string) map[SignalName]SignalValue {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.endpoints[address]
	if state == nil {
		return map[SignalName]SignalValue{
			SignalInflight: available(0, "gateway_local"),
			SignalFailure:  available(0, "gateway_local"),
		}
	}
	return map[SignalName]SignalValue{
		SignalInflight: available(float64(state.Inflight), "gateway_local"),
		SignalFailure:  available(state.FailureEWMA, "gateway_local"),
	}
}

func (s *FeedbackStore) Begin(address string, now time.Time) *FeedbackLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.endpoints[address]
	if state == nil {
		state = &EndpointFeedback{}
		s.endpoints[address] = state
	}
	state.Inflight++
	return &FeedbackLease{store: s, address: address, startedAt: now}
}

// Complete settles a request once. firstByte may be zero when no response data
// was observed (for example, a cancellation).
func (l *FeedbackLease) Complete(now, firstByte time.Time, failed bool) bool {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if l.completed {
		return false
	}
	l.completed = true
	state := l.store.endpoints[l.address]
	if state == nil {
		return true
	}
	if state.Inflight > 0 {
		state.Inflight--
	}
	if !firstByte.IsZero() && !firstByte.Before(l.startedAt) {
		state.TTFT = firstByte.Sub(l.startedAt)
	}
	if !now.Before(l.startedAt) {
		state.Latency = now.Sub(l.startedAt)
	}
	sample := 0.0
	if failed {
		sample = 1
	}
	state.FailureEWMA = l.store.alpha*sample + (1-l.store.alpha)*state.FailureEWMA
	return true
}

func (s *FeedbackStore) Cleanup(active map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for address, state := range s.endpoints {
		if _, ok := active[address]; !ok && state.Inflight == 0 {
			delete(s.endpoints, address)
		}
	}
}

func (s *FeedbackStore) Snapshot(address string) (EndpointFeedback, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.endpoints[address]
	if !ok {
		return EndpointFeedback{}, false
	}
	return *state, true
}

func (s *FeedbackStore) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.endpoints)
}

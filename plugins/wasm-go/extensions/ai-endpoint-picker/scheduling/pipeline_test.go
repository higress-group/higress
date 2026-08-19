package scheduling

import (
	"math"
	"testing"
)

type fixedRandom int

func (r fixedRandom) Intn(n int) int { return int(r) % n }

func testWeights() Weights {
	return Weights{
		SignalQueue: 2, SignalKVCache: 2, SignalPrefixCache: 3,
		SignalLoRAAffinity: 0, SignalInflight: 0, SignalFailure: 0,
	}
}

func endpoint(address string, healthy bool, values map[SignalName]float64) EndpointSnapshot {
	signals := map[SignalName]SignalValue{}
	for name, value := range values {
		signals[name] = SignalValue{Value: value, Available: true, Confidence: 1}
	}
	return EndpointSnapshot{Address: address, Healthy: healthy, Signals: signals}
}

func TestPipelinePrefixAndQueueKVWeightedConflict(t *testing.T) {
	decision := NewPipeline(testWeights(), fixedRandom(0)).Schedule([]EndpointSnapshot{
		endpoint("cache-rich", true, map[SignalName]float64{
			SignalQueue: 10, SignalKVCache: 0, SignalPrefixCache: 1,
		}),
		endpoint("short-queue", true, map[SignalName]float64{
			SignalQueue: 0, SignalKVCache: 0.9, SignalPrefixCache: 0,
		}),
	})
	if decision.Address != "cache-rich" {
		t.Fatalf("selected %q, want cache-rich (score %+v)", decision.Address, decision)
	}
	if decision.Score <= 0 || decision.Score > 1 {
		t.Fatalf("score %v outside [0,1]", decision.Score)
	}
	wantMask := SignalAvailabilityQueue | SignalAvailabilityKVCache | SignalAvailabilityPrefixCache
	if decision.Reason != DecisionReasonMaxScore || decision.SignalAvailability != wantMask {
		t.Fatalf("decision summary reason=%q mask=0x%x", decision.Reason, decision.SignalAvailability)
	}
}

func TestPipelineMissingSignalDoesNotBenefit(t *testing.T) {
	decision := NewPipeline(testWeights(), fixedRandom(0)).Schedule([]EndpointSnapshot{
		endpoint("observed", true, map[SignalName]float64{
			SignalQueue: 0, SignalInflight: 0, SignalFailure: 0,
		}),
		endpoint("missing", true, map[SignalName]float64{
			SignalInflight: 0, SignalFailure: 0,
		}),
	})
	if decision.Address != "observed" {
		t.Fatalf("selected %q, want observed", decision.Address)
	}
	want := float64(2) / 7
	if math.Abs(decision.Score-want) > 1e-9 {
		t.Fatalf("score = %v, want fixed-denominator %v", decision.Score, want)
	}
}

func TestPipelineLoRAAffinityAndFailure(t *testing.T) {
	weights := Weights{SignalLoRAAffinity: 1, SignalInflight: 1, SignalFailure: 1}
	decision := NewPipeline(weights, fixedRandom(0)).Schedule([]EndpointSnapshot{
		endpoint("affinity", true, map[SignalName]float64{
			SignalLoRAAffinity: 1, SignalInflight: 0, SignalFailure: 0,
		}),
		endpoint("failed", true, map[SignalName]float64{
			SignalLoRAAffinity: 0, SignalInflight: 0, SignalFailure: 1,
		}),
	})
	if decision.Address != "affinity" {
		t.Fatalf("selected %q, want affinity", decision.Address)
	}
}

func TestPipelineTieUsesInjectedRandom(t *testing.T) {
	weights := Weights{SignalInflight: 1, SignalFailure: 1}
	decision := NewPipeline(weights, fixedRandom(1)).Schedule([]EndpointSnapshot{
		endpoint("first", true, map[SignalName]float64{SignalInflight: 0, SignalFailure: 0}),
		endpoint("second", true, map[SignalName]float64{SignalInflight: 0, SignalFailure: 0}),
	})
	if decision.Address != "second" {
		t.Fatalf("selected %q, want injected tie choice", decision.Address)
	}
	wantMask := SignalAvailabilityInflight | SignalAvailabilityFailure
	if decision.Reason != DecisionReasonRandomTie || decision.SignalAvailability != wantMask {
		t.Fatalf("tie summary reason=%q mask=0x%x", decision.Reason, decision.SignalAvailability)
	}
}

func TestPipelineFailOpenWhenAllFiltered(t *testing.T) {
	decision := NewPipeline(testWeights(), fixedRandom(0)).Schedule([]EndpointSnapshot{
		endpoint("unhealthy", false, nil),
	})
	if decision.Address != "" || decision.FallbackReason != "no_healthy_endpoint" {
		t.Fatalf("decision = %+v, want fail-open", decision)
	}
}

func TestPipelineFailOpenWithoutAnyConfiguredSignal(t *testing.T) {
	decision := NewPipeline(Weights{SignalQueue: 1}, fixedRandom(0)).Schedule([]EndpointSnapshot{
		endpoint("healthy", true, map[SignalName]float64{SignalInflight: 0, SignalFailure: 0}),
	})
	if decision.Address != "" || decision.FallbackReason != "no_valid_signal" {
		t.Fatalf("decision = %+v, want no-valid-signal fail-open", decision)
	}
	wantMask := SignalAvailabilityInflight | SignalAvailabilityFailure
	if decision.SignalAvailability != wantMask {
		t.Fatalf("fallback availability mask=0x%x want 0x30", decision.SignalAvailability)
	}
}

func TestPipelineEqualMinMaxScoresOne(t *testing.T) {
	decision := NewPipeline(Weights{SignalQueue: 1}, fixedRandom(0)).Schedule([]EndpointSnapshot{
		endpoint("first", true, map[SignalName]float64{SignalQueue: 3}),
		endpoint("second", true, map[SignalName]float64{SignalQueue: 3}),
	})
	if decision.Score != 1 {
		t.Fatalf("score = %v, want 1", decision.Score)
	}
}

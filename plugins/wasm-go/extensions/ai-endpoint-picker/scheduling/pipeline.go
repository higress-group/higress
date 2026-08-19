package scheduling

import (
	"math"
)

type IntnSource interface {
	Intn(n int) int
}

type Pipeline struct {
	weights Weights
	random  IntnSource
}

func NewPipeline(weights Weights, random IntnSource) *Pipeline {
	return &Pipeline{weights: weights, random: random}
}

func (p *Pipeline) Schedule(endpoints []EndpointSnapshot) Decision {
	candidates := filterHealthy(endpoints)
	if len(candidates) == 0 {
		return Decision{FallbackReason: "no_healthy_endpoint"}
	}
	availableSignals := signalAvailability(candidates)

	normalized := normalize(candidates)
	denominator := 0.0
	for _, signal := range SignalNames {
		denominator += p.weights[signal]
	}
	if denominator <= 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return Decision{CandidateCount: len(candidates), SignalAvailability: availableSignals, FallbackReason: "invalid_weights"}
	}

	scores := make([]float64, len(candidates))
	best := -1.0
	bestIndexes := make([]int, 0, len(candidates))
	anyValidSignal := false
	for i := range candidates {
		for _, signal := range SignalNames {
			value := normalized[i][signal]
			if !validSignal(value) || p.weights[signal] == 0 {
				continue
			}
			anyValidSignal = true
			confidence := clamp(value.Confidence)
			scores[i] += p.weights[signal] * clamp(value.Value) * confidence
		}
		scores[i] /= denominator
		if scores[i] > best {
			best = scores[i]
			bestIndexes = bestIndexes[:0]
			bestIndexes = append(bestIndexes, i)
		} else if scores[i] == best {
			bestIndexes = append(bestIndexes, i)
		}
	}
	if !anyValidSignal {
		return Decision{CandidateCount: len(candidates), SignalAvailability: availableSignals, FallbackReason: "no_valid_signal"}
	}

	selected := bestIndexes[0]
	reason := DecisionReasonMaxScore
	if len(bestIndexes) > 1 && p.random != nil {
		selected = bestIndexes[p.random.Intn(len(bestIndexes))]
		reason = DecisionReasonRandomTie
	}
	return Decision{
		Address:            candidates[selected].Address,
		Score:              scores[selected],
		CandidateCount:     len(candidates),
		Reason:             reason,
		SignalAvailability: signalAvailability([]EndpointSnapshot{candidates[selected]}),
	}
}

func signalAvailability(endpoints []EndpointSnapshot) uint64 {
	var mask uint64
	for _, endpoint := range endpoints {
		for _, name := range SignalNames {
			if value, ok := endpoint.Signals[name]; ok && validSignal(value) {
				mask |= signalAvailabilityBit(name)
			}
		}
	}
	return mask
}

func signalAvailabilityBit(name SignalName) uint64 {
	switch name {
	case SignalQueue:
		return SignalAvailabilityQueue
	case SignalKVCache:
		return SignalAvailabilityKVCache
	case SignalPrefixCache:
		return SignalAvailabilityPrefixCache
	case SignalLoRAAffinity:
		return SignalAvailabilityLoRAAffinity
	case SignalInflight:
		return SignalAvailabilityInflight
	case SignalFailure:
		return SignalAvailabilityFailure
	default:
		return 0
	}
}

func filterHealthy(endpoints []EndpointSnapshot) []EndpointSnapshot {
	result := make([]EndpointSnapshot, 0, len(endpoints))
	for _, endpoint := range endpoints {
		if endpoint.Healthy {
			result = append(result, endpoint)
		}
	}
	return result
}

func normalize(endpoints []EndpointSnapshot) []map[SignalName]SignalValue {
	result := make([]map[SignalName]SignalValue, len(endpoints))
	for i, endpoint := range endpoints {
		result[i] = make(map[SignalName]SignalValue, len(endpoint.Signals))
		for name, value := range endpoint.Signals {
			result[i][name] = value
		}
	}
	normalizeLowerIsBetter(result, SignalQueue)
	normalizeLowerIsBetter(result, SignalInflight)
	transformOneMinus(result, SignalKVCache)
	transformOneMinus(result, SignalFailure)
	for _, name := range []SignalName{SignalPrefixCache, SignalLoRAAffinity} {
		for i := range result {
			if value, ok := result[i][name]; ok && value.Available {
				value.Value = clamp(value.Value)
				result[i][name] = value
			}
		}
	}
	return result
}

func normalizeLowerIsBetter(values []map[SignalName]SignalValue, name SignalName) {
	minValue, maxValue := math.Inf(1), math.Inf(-1)
	for _, signals := range values {
		value, ok := signals[name]
		if !ok || !validSignal(value) {
			continue
		}
		minValue = math.Min(minValue, value.Value)
		maxValue = math.Max(maxValue, value.Value)
	}
	for i, signals := range values {
		value, ok := signals[name]
		if !ok || !validSignal(value) {
			continue
		}
		if minValue == maxValue {
			value.Value = 1
		} else {
			value.Value = 1 - (value.Value-minValue)/(maxValue-minValue)
		}
		values[i][name] = value
	}
}

func transformOneMinus(values []map[SignalName]SignalValue, name SignalName) {
	for i, signals := range values {
		value, ok := signals[name]
		if !ok || !validSignal(value) {
			continue
		}
		value.Value = 1 - clamp(value.Value)
		values[i][name] = value
	}
}

func validSignal(value SignalValue) bool {
	return value.Available && !math.IsNaN(value.Value) && !math.IsInf(value.Value, 0) &&
		!math.IsNaN(value.Confidence) && !math.IsInf(value.Confidence, 0)
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

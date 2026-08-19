package scheduling

type SignalName string

const (
	SignalQueue        SignalName = "queue"
	SignalKVCache      SignalName = "kv_cache"
	SignalPrefixCache  SignalName = "prefix_cache"
	SignalLoRAAffinity SignalName = "lora_affinity"
	SignalInflight     SignalName = "inflight"
	SignalFailure      SignalName = "failure"
)

var SignalNames = []SignalName{
	SignalQueue,
	SignalKVCache,
	SignalPrefixCache,
	SignalLoRAAffinity,
	SignalInflight,
	SignalFailure,
}

type SignalValue struct {
	Value      float64
	Available  bool
	Confidence float64
	Source     string
	Reason     string
}

type EndpointSnapshot struct {
	Address string
	Healthy bool
	Signals map[SignalName]SignalValue
}

type Decision struct {
	Address            string
	Score              float64
	CandidateCount     int
	Reason             string
	SignalAvailability uint64
	FallbackReason     string
}

const (
	DecisionReasonMaxScore  = "max_score"
	DecisionReasonRandomTie = "random_tie"
)

const (
	SignalAvailabilityQueue uint64 = 1 << iota
	SignalAvailabilityKVCache
	SignalAvailabilityPrefixCache
	SignalAvailabilityLoRAAffinity
	SignalAvailabilityInflight
	SignalAvailabilityFailure
)

type Weights map[SignalName]float64

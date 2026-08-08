package scheduling

type SignalName string

const (
	SignalQueue        SignalName = "queue"
	SignalKVCache      SignalName = "kv_cache"
	SignalLoRAAffinity SignalName = "lora_affinity"
	SignalInflight     SignalName = "inflight"
	SignalFailure      SignalName = "failure"
)

var SignalNames = []SignalName{
	SignalQueue,
	SignalKVCache,
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
	Address        string
	Score          float64
	CandidateCount int
	FallbackReason string
}

type Weights map[SignalName]float64

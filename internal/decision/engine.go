package decision

type DecisionType string

const (
	Promote  DecisionType = "PROMOTE"
	Pause    DecisionType = "PAUSE"
	Rollback DecisionType = "ROLLBACK"
)

type Telemetry struct {
	ServiceID string
	LatencyMs float64
	IsError   bool
	Timestamp int64
}

type Engine struct {
	Policy *Policy
}

func NewEngine(policy *Policy) *Engine {
	return &Engine{
		Policy: policy,
	}
}

func (e *Engine) Evaluate(events []Telemetry) DecisionType {
	if len(events) == 0 {
		return e.Policy.Actions.OnSuccess
	}

	var errorCount int
	var totalLatency float64

	for _, ev := range events {
		if ev.IsError {
			errorCount++
		}
		totalLatency += ev.LatencyMs
	}

	errorRate := float64(errorCount) / float64(len(events))
	avgLatency := totalLatency / float64(len(events))

	if errorRate > e.Policy.Thresholds.ErrorRate {
		return e.Policy.Actions.OnError
	}

	if avgLatency > e.Policy.Thresholds.LatencyMs {
		return e.Policy.Actions.OnLatency
	}

	return e.Policy.Actions.OnSuccess
}

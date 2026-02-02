package decision

import (
	"time"
)

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
	Timestamp time.Time
}

type WindowStats struct {
	Count        int
	ErrorCount   int
	TotalLatency float64
}

type Engine struct {
	WindowDuration time.Duration
	ErrorThreshold float64
	LatencyLimitMs float64
}

func NewEngine() *Engine {
	return &Engine{
		WindowDuration: 30 * time.Second,
		ErrorThreshold: 0.05,
		LatencyLimitMs: 500,
	}
}

func (e *Engine) Evaluate(events []Telemetry) DecisionType {
	if len(events) == 0 {
		return Promote
	}

	stats := WindowStats{}

	for _, ev := range events {
		stats.Count++
		stats.TotalLatency += ev.LatencyMs
		if ev.IsError {
			stats.ErrorCount++
		}
	}

	errorRate := float64(stats.ErrorCount) / float64(stats.Count)
	avgLatency := stats.TotalLatency / float64(stats.Count)

	if errorRate > e.ErrorThreshold {
		return Rollback
	}

	if avgLatency > e.LatencyLimitMs {
		return Pause
	}

	return Promote
}

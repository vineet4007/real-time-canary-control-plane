package decision

import (
	"math"
	"sort"
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
	Timestamp int64
}

type Engine struct {
	Policy *Policy
	state  adaptiveState
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
	errorThreshold, latencyThreshold := e.effectiveThresholds(len(events))

	if e.Policy.SLO.Enabled {
		minEvents := e.Policy.SLO.MinEvents
		if minEvents <= 0 {
			minEvents = 1
		}
		if len(events) >= minEvents {
			availability := 1 - errorRate
			p95 := p95Latency(events)

			availabilityBreached :=
				e.Policy.SLO.AvailabilityTarget > 0 && availability < e.Policy.SLO.AvailabilityTarget
			latencyBreached :=
				e.Policy.SLO.LatencyP95Ms > 0 && p95 > e.Policy.SLO.LatencyP95Ms

			if availabilityBreached || latencyBreached {
				if e.Policy.SLO.OnBreach != "" {
					return e.Policy.SLO.OnBreach
				}
				return e.Policy.Actions.OnError
			}
		}
	}

	if errorRate > errorThreshold {
		e.updateAdaptive(errorRate, avgLatency, len(events))
		return e.Policy.Actions.OnError
	}

	if avgLatency > latencyThreshold {
		e.updateAdaptive(errorRate, avgLatency, len(events))
		return e.Policy.Actions.OnLatency
	}

	e.updateAdaptive(errorRate, avgLatency, len(events))
	return e.Policy.Actions.OnSuccess
}

type adaptiveState struct {
	initialized     bool
	baselineError   float64
	baselineLatency float64
}

func (e *Engine) effectiveThresholds(sampleCount int) (float64, float64) {
	errorThreshold := e.Policy.Thresholds.ErrorRate
	latencyThreshold := e.Policy.Thresholds.LatencyMs

	if !e.Policy.Adaptive.Enabled {
		return errorThreshold, latencyThreshold
	}

	minEvents := e.Policy.Adaptive.MinEvents
	if minEvents <= 0 {
		minEvents = 1
	}
	if sampleCount < minEvents || !e.state.initialized {
		return errorThreshold, latencyThreshold
	}

	errorMultiplier := e.Policy.Adaptive.ErrorRateMultiplier
	if errorMultiplier <= 0 {
		errorMultiplier = 1.0
	}
	latencyMultiplier := e.Policy.Adaptive.LatencyMultiplier
	if latencyMultiplier <= 0 {
		latencyMultiplier = 1.0
	}

	errorThreshold = e.state.baselineError * errorMultiplier
	latencyThreshold = e.state.baselineLatency * latencyMultiplier

	if e.Policy.Adaptive.ErrorRateMin > 0 && errorThreshold < e.Policy.Adaptive.ErrorRateMin {
		errorThreshold = e.Policy.Adaptive.ErrorRateMin
	}
	if e.Policy.Adaptive.ErrorRateMax > 0 && errorThreshold > e.Policy.Adaptive.ErrorRateMax {
		errorThreshold = e.Policy.Adaptive.ErrorRateMax
	}
	if e.Policy.Adaptive.LatencyMinMs > 0 && latencyThreshold < e.Policy.Adaptive.LatencyMinMs {
		latencyThreshold = e.Policy.Adaptive.LatencyMinMs
	}
	if e.Policy.Adaptive.LatencyMaxMs > 0 && latencyThreshold > e.Policy.Adaptive.LatencyMaxMs {
		latencyThreshold = e.Policy.Adaptive.LatencyMaxMs
	}

	if errorThreshold <= 0 {
		errorThreshold = e.Policy.Thresholds.ErrorRate
	}
	if latencyThreshold <= 0 {
		latencyThreshold = e.Policy.Thresholds.LatencyMs
	}
	return errorThreshold, latencyThreshold
}

func (e *Engine) updateAdaptive(errorRate, avgLatency float64, sampleCount int) {
	if !e.Policy.Adaptive.Enabled {
		return
	}
	minEvents := e.Policy.Adaptive.MinEvents
	if minEvents <= 0 {
		minEvents = 1
	}
	if sampleCount < minEvents {
		return
	}

	if !e.state.initialized {
		e.state.initialized = true
		e.state.baselineError = errorRate
		e.state.baselineLatency = avgLatency
		return
	}

	alpha := e.Policy.Adaptive.Alpha
	if alpha <= 0 || alpha > 1 {
		alpha = 0.2
	}

	e.state.baselineError = alpha*errorRate + (1-alpha)*e.state.baselineError
	e.state.baselineLatency = alpha*avgLatency + (1-alpha)*e.state.baselineLatency
}

func p95Latency(events []Telemetry) float64 {
	if len(events) == 0 {
		return 0
	}

	latencies := make([]float64, len(events))
	for i := range events {
		latencies[i] = events[i].LatencyMs
	}
	sort.Float64s(latencies)

	idx := int(math.Ceil(0.95*float64(len(latencies)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(latencies) {
		idx = len(latencies) - 1
	}
	return latencies[idx]
}

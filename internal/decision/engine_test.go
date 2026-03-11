package decision

import (
	"testing"
	"time"
)

func testPolicy() *Policy {
	p := &Policy{
		Service:       "checkout-service",
		WindowSeconds: 30,
	}
	p.Thresholds.ErrorRate = 0.05
	p.Thresholds.LatencyMs = 500
	p.Actions.OnError = Rollback
	p.Actions.OnLatency = Pause
	p.Actions.OnSuccess = Promote
	return p
}

func TestRollbackOnHighErrorRate(t *testing.T) {
	engine := NewEngine(testPolicy())

	events := make([]Telemetry, 0)

	// 40% error rate
	for i := 0; i < 100; i++ {
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 120,
			IsError:   i%2 == 0,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	result := engine.Evaluate(events)

	if result != Rollback {
		t.Fatalf("expected ROLLBACK, got %s", result)
	}
}

func TestPauseOnHighLatency(t *testing.T) {
	engine := NewEngine(testPolicy())

	events := make([]Telemetry, 0)

	// High latency, low error rate
	for i := 0; i < 100; i++ {
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 1200,
			IsError:   false,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	result := engine.Evaluate(events)

	if result != Pause {
		t.Fatalf("expected PAUSE, got %s", result)
	}
}

func TestPromoteOnHealthyMetrics(t *testing.T) {
	engine := NewEngine(testPolicy())

	events := make([]Telemetry, 0)

	for i := 0; i < 100; i++ {
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 150,
			IsError:   false,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	result := engine.Evaluate(events)

	if result != Promote {
		t.Fatalf("expected PROMOTE, got %s", result)
	}
}

func TestSLOBreachOnAvailability(t *testing.T) {
	p := testPolicy()
	p.SLO.Enabled = true
	p.SLO.MinEvents = 20
	p.SLO.AvailabilityTarget = 0.99
	p.SLO.OnBreach = Rollback

	engine := NewEngine(p)

	events := make([]Telemetry, 0, 100)
	for i := 0; i < 100; i++ {
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 120,
			IsError:   i < 2,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	result := engine.Evaluate(events)
	if result != Rollback {
		t.Fatalf("expected ROLLBACK on SLO availability breach, got %s", result)
	}
}

func TestSLOBreachOnP95Latency(t *testing.T) {
	p := testPolicy()
	p.SLO.Enabled = true
	p.SLO.MinEvents = 20
	p.SLO.LatencyP95Ms = 200
	p.SLO.OnBreach = Pause

	engine := NewEngine(p)

	events := make([]Telemetry, 0, 100)
	for i := 0; i < 100; i++ {
		latency := 120.0
		if i >= 90 {
			latency = 600
		}
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: latency,
			IsError:   false,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	result := engine.Evaluate(events)
	if result != Pause {
		t.Fatalf("expected PAUSE on SLO p95 breach, got %s", result)
	}
}

func TestAdaptiveThresholdRelaxesAfterStableHistory(t *testing.T) {
	p := testPolicy()
	p.Adaptive.Enabled = true
	p.Adaptive.MinEvents = 20
	p.Adaptive.Alpha = 1
	p.Adaptive.ErrorRateMultiplier = 8
	p.Adaptive.LatencyMultiplier = 2
	p.Adaptive.ErrorRateMax = 0.2
	p.Adaptive.LatencyMaxMs = 1000

	engine := NewEngine(p)

	warmup := make([]Telemetry, 0, 100)
	for i := 0; i < 100; i++ {
		warmup = append(warmup, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 120,
			IsError:   i < 1,
			Timestamp: time.Now().UnixMilli(),
		})
	}
	if got := engine.Evaluate(warmup); got != Promote {
		t.Fatalf("expected warmup PROMOTE, got %s", got)
	}

	current := make([]Telemetry, 0, 100)
	for i := 0; i < 100; i++ {
		current = append(current, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 140,
			IsError:   i < 6,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	if got := engine.Evaluate(current); got != Promote {
		t.Fatalf("expected adaptive PROMOTE, got %s", got)
	}
}

func TestAdaptiveThresholdClampMax(t *testing.T) {
	p := testPolicy()
	p.Adaptive.Enabled = true
	p.Adaptive.MinEvents = 20
	p.Adaptive.Alpha = 1
	p.Adaptive.ErrorRateMultiplier = 2
	p.Adaptive.ErrorRateMax = 0.20

	engine := NewEngine(p)

	baseline := make([]Telemetry, 0, 100)
	for i := 0; i < 100; i++ {
		baseline = append(baseline, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 150,
			IsError:   i < 40,
			Timestamp: time.Now().UnixMilli(),
		})
	}
	engine.Evaluate(baseline)

	window := make([]Telemetry, 0, 100)
	for i := 0; i < 100; i++ {
		window = append(window, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 150,
			IsError:   i < 25,
			Timestamp: time.Now().UnixMilli(),
		})
	}

	if got := engine.Evaluate(window); got != Rollback {
		t.Fatalf("expected ROLLBACK because adaptive threshold is clamped at 0.20, got %s", got)
	}
}

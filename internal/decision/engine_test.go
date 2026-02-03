package decision

import (
	"testing"
	"time"
)

func TestRollbackOnHighErrorRate(t *testing.T) {
	engine := NewEngine()

	events := make([]Telemetry, 0)

	// 40% error rate
	for i := 0; i < 100; i++ {
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 120,
			IsError:   i%2 == 0,
			Timestamp: time.Now(),
		})
	}

	result := engine.Evaluate(events)

	if result != Rollback {
		t.Fatalf("expected ROLLBACK, got %s", result)
	}
}

func TestPauseOnHighLatency(t *testing.T) {
	engine := NewEngine()

	events := make([]Telemetry, 0)

	// High latency, low error rate
	for i := 0; i < 100; i++ {
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 1200,
			IsError:   false,
			Timestamp: time.Now(),
		})
	}

	result := engine.Evaluate(events)

	if result != Pause {
		t.Fatalf("expected PAUSE, got %s", result)
	}
}

func TestPromoteOnHealthyMetrics(t *testing.T) {
	engine := NewEngine()

	events := make([]Telemetry, 0)

	for i := 0; i < 100; i++ {
		events = append(events, Telemetry{
			ServiceID: "checkout-service",
			LatencyMs: 150,
			IsError:   false,
			Timestamp: time.Now(),
		})
	}

	result := engine.Evaluate(events)

	if result != Promote {
		t.Fatalf("expected PROMOTE, got %s", result)
	}
}

package main

import (
	"reflect"
	"testing"
	"time"
)

func TestParseServicesDefault(t *testing.T) {
	services := parseServices("")
	want := []string{"checkout-service"}

	if !reflect.DeepEqual(services, want) {
		t.Fatalf("expected %v, got %v", want, services)
	}
}

func TestParseServicesList(t *testing.T) {
	services := parseServices("checkout-service, payments-service")
	want := []string{"checkout-service", "payments-service"}

	if !reflect.DeepEqual(services, want) {
		t.Fatalf("expected %v, got %v", want, services)
	}
}

func TestLoadChaosConfigDefaults(t *testing.T) {
	cfg := loadChaosConfigFromEnv(func(string) string { return "" })

	if cfg.mode != chaosOff {
		t.Fatalf("expected mode=%s got=%s", chaosOff, cfg.mode)
	}
	if cfg.interval != time.Duration(defaultChaosInterval)*time.Second {
		t.Fatalf("unexpected interval: %s", cfg.interval)
	}
	if cfg.duration != time.Duration(defaultChaosDuration)*time.Second {
		t.Fatalf("unexpected duration: %s", cfg.duration)
	}
	if cfg.errorRate != defaultChaosErrRate {
		t.Fatalf("unexpected errorRate: %f", cfg.errorRate)
	}
	if cfg.latencyMs != defaultChaosLatency {
		t.Fatalf("unexpected latencyMs: %f", cfg.latencyMs)
	}
}

func TestLoadChaosConfigClampsInvalidValues(t *testing.T) {
	env := map[string]string{
		envChaosMode:        "mixed",
		envChaosIntervalSec: "10",
		envChaosDurationSec: "20",
		envChaosErrorRate:   "1.5",
		envChaosLatencyMs:   "-8",
	}
	cfg := loadChaosConfigFromEnv(func(k string) string { return env[k] })

	if cfg.mode != chaosMixed {
		t.Fatalf("expected mode=%s got=%s", chaosMixed, cfg.mode)
	}
	if cfg.duration != cfg.interval {
		t.Fatalf("expected duration to be clamped to interval")
	}
	if cfg.errorRate != 1 {
		t.Fatalf("expected errorRate to clamp at 1, got %f", cfg.errorRate)
	}
	if cfg.latencyMs != defaultChaosLatency {
		t.Fatalf("expected default latency fallback, got %f", cfg.latencyMs)
	}
}

func TestChaosIsActive(t *testing.T) {
	cfg := chaosConfig{
		mode:      chaosErrorBurst,
		interval:  10 * time.Second,
		duration:  3 * time.Second,
		errorRate: 0.5,
		latencyMs: 1200,
	}
	start := time.Unix(1000, 0)

	if !cfg.isActive(start, start.Add(2*time.Second)) {
		t.Fatalf("expected active in first window segment")
	}
	if cfg.isActive(start, start.Add(5*time.Second)) {
		t.Fatalf("expected inactive outside window segment")
	}
	if !cfg.isActive(start, start.Add(11*time.Second)) {
		t.Fatalf("expected active in next window segment")
	}
}

func TestApplyChaosModes(t *testing.T) {
	lat, errFlag := applyChaos(100, false, chaosConfig{mode: chaosErrorBurst, errorRate: 1}, true)
	if lat != 100 || !errFlag {
		t.Fatalf("error burst should force error and keep latency")
	}

	lat, errFlag = applyChaos(100, false, chaosConfig{mode: chaosLatencySpike, latencyMs: 900}, true)
	if lat < 900 || errFlag {
		t.Fatalf("latency spike should elevate latency and keep error flag")
	}

	lat, errFlag = applyChaos(100, false, chaosConfig{mode: chaosMixed, latencyMs: 800, errorRate: 0}, true)
	if lat < 800 || errFlag {
		t.Fatalf("mixed mode should elevate latency and use error rate")
	}
}

package main

import (
	"context"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
)

const (
	kafkaBroker              = "localhost:9092"
	topic                    = "telemetry.raw"
	defaultTelemetryServices = "checkout-service"
	defaultErrorRate         = 0.05
	defaultLatencyBaseMs     = 50
	defaultLatencyJitterMs   = 400

	envChaosMode         = "TELEMETRY_CHAOS_MODE"
	envChaosIntervalSec  = "TELEMETRY_CHAOS_INTERVAL_SEC"
	envChaosDurationSec  = "TELEMETRY_CHAOS_DURATION_SEC"
	envChaosErrorRate    = "TELEMETRY_CHAOS_ERROR_RATE"
	envChaosLatencyMs    = "TELEMETRY_CHAOS_LATENCY_MS"
	defaultChaosInterval = 60
	defaultChaosDuration = 15
	defaultChaosErrRate  = 0.35
	defaultChaosLatency  = 1200
)

type chaosMode string

const (
	chaosOff          chaosMode = "off"
	chaosErrorBurst   chaosMode = "error_burst"
	chaosLatencySpike chaosMode = "latency_spike"
	chaosMixed        chaosMode = "mixed"
)

type chaosConfig struct {
	mode      chaosMode
	interval  time.Duration
	duration  time.Duration
	errorRate float64
	latencyMs float64
}

func main() {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaBroker},
		Topic:    topic,
		Balancer: &kafka.Hash{},
	})
	defer writer.Close()

	rand.Seed(time.Now().UnixNano())
	services := parseServices(os.Getenv("TELEMETRY_SERVICES"))
	chaos := loadChaosConfigFromEnv(os.Getenv)
	producerStarted := time.Now()
	lastChaosActive := false
	if chaos.mode != chaosOff {
		log.Printf(
			"chaos mode enabled mode=%s interval=%s duration=%s errorRate=%.2f latencyMs=%.0f",
			chaos.mode,
			chaos.interval,
			chaos.duration,
			chaos.errorRate,
			chaos.latencyMs,
		)
	}

	for {
		now := time.Now()
		chaosActive := chaos.isActive(producerStarted, now)
		if chaosActive != lastChaosActive && chaos.mode != chaosOff {
			if chaosActive {
				log.Printf("chaos window started mode=%s", chaos.mode)
			} else {
				log.Printf("chaos window ended mode=%s", chaos.mode)
			}
			lastChaosActive = chaosActive
		}

		serviceID := services[rand.Intn(len(services))]
		latency := rand.Float64()*defaultLatencyJitterMs + defaultLatencyBaseMs
		isError := rand.Float64() < defaultErrorRate
		latency, isError = applyChaos(latency, isError, chaos, chaosActive)

		event := &rolloutpb.TelemetryEvent{
			ServiceId:       serviceID,
			LatencyMs:       latency,
			Error:           isError,
			TimestampUnixMs: now.UnixMilli(),
		}

		bytes, err := proto.Marshal(event)
		if err != nil {
			log.Fatalf("failed to marshal proto: %v", err)
		}

		msg := kafka.Message{
			Key:   []byte(serviceID),
			Value: bytes,
		}

		if err := writer.WriteMessages(context.Background(), msg); err != nil {
			log.Fatalf("kafka write failed: %v", err)
		}

		log.Printf(
			"sent telemetry service=%s latency=%.2f error=%v chaos=%v",
			serviceID,
			event.LatencyMs,
			event.Error,
			chaosActive,
		)
		time.Sleep(500 * time.Millisecond)
	}
}

func parseServices(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		raw = defaultTelemetryServices
	}

	parts := strings.Split(raw, ",")
	services := make([]string, 0, len(parts))
	for _, part := range parts {
		service := strings.TrimSpace(part)
		if service == "" {
			continue
		}
		services = append(services, service)
	}

	if len(services) == 0 {
		return []string{"checkout-service"}
	}
	return services
}

func loadChaosConfigFromEnv(getenv func(string) string) chaosConfig {
	mode := chaosMode(strings.ToLower(strings.TrimSpace(getenv(envChaosMode))))
	switch mode {
	case chaosErrorBurst, chaosLatencySpike, chaosMixed:
	default:
		mode = chaosOff
	}

	intervalSec := parseIntOrDefault(getenv(envChaosIntervalSec), defaultChaosInterval)
	durationSec := parseIntOrDefault(getenv(envChaosDurationSec), defaultChaosDuration)
	errorRate := parseFloatOrDefault(getenv(envChaosErrorRate), defaultChaosErrRate)
	latencyMs := parseFloatOrDefault(getenv(envChaosLatencyMs), defaultChaosLatency)

	if intervalSec < 1 {
		intervalSec = 1
	}
	if durationSec < 1 {
		durationSec = 1
	}
	if durationSec > intervalSec {
		durationSec = intervalSec
	}
	if errorRate < 0 {
		errorRate = 0
	}
	if errorRate > 1 {
		errorRate = 1
	}
	if latencyMs <= 0 {
		latencyMs = defaultChaosLatency
	}

	return chaosConfig{
		mode:      mode,
		interval:  time.Duration(intervalSec) * time.Second,
		duration:  time.Duration(durationSec) * time.Second,
		errorRate: errorRate,
		latencyMs: latencyMs,
	}
}

func (c chaosConfig) isActive(start, now time.Time) bool {
	if c.mode == chaosOff || c.interval <= 0 || c.duration <= 0 {
		return false
	}
	if now.Before(start) {
		return false
	}

	elapsed := now.Sub(start)
	position := elapsed % c.interval
	return position < c.duration
}

func applyChaos(latency float64, isError bool, chaos chaosConfig, active bool) (float64, bool) {
	if !active {
		return latency, isError
	}

	switch chaos.mode {
	case chaosErrorBurst:
		return latency, rand.Float64() < chaos.errorRate
	case chaosLatencySpike:
		return chaos.latencyMs + rand.Float64()*100, isError
	case chaosMixed:
		return chaos.latencyMs + rand.Float64()*100, rand.Float64() < chaos.errorRate
	default:
		return latency, isError
	}
}

func parseIntOrDefault(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return v
}

func parseFloatOrDefault(raw string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return fallback
	}
	return v
}

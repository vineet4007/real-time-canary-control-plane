package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"

	"github.com/vineet4007/real-time-canary-control-plane/internal/decision"
	grpcsrv "github.com/vineet4007/real-time-canary-control-plane/internal/grpc"
	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
	"github.com/vineet4007/real-time-canary-control-plane/internal/redis"
)

const (
	broker         = "localhost:9092"
	telemetryTopic = "telemetry.raw"
	decisionTopic  = "rollout.decisions"
	consumerGroup  = "decision-engine"
	policiesDir    = "deploy/policies"
	tenantQuotaCfg = "deploy/policies/tenants.yaml"
	dispatchTick   = 1 * time.Second
)

type TenantQuotas struct {
	DefaultQuotaPerMinute int                    `yaml:"default_quota_per_minute"`
	Tenants               map[string]TenantQuota `yaml:"tenants"`
}

type TenantQuota struct {
	MaxEvaluationsPerMinute int `yaml:"max_evaluations_per_minute"`
}

type tenantLimiter struct {
	maxPerMinute int
	windowStart  time.Time
	used         int
}

func (t *tenantLimiter) allow(now time.Time) bool {
	if t == nil || t.maxPerMinute <= 0 {
		return true
	}
	if now.Sub(t.windowStart) >= time.Minute {
		t.windowStart = now
		t.used = 0
	}
	if t.used >= t.maxPerMinute {
		return false
	}
	t.used++
	return true
}

func main() {
	log.Println("starting decision engine")

	// 1️⃣ Load rollout policy bundle (Policy-as-Code)
	policies, err := loadPolicies(policiesDir)
	if err != nil {
		log.Fatalf("failed to load policies: %v", err)
	}
	tenantQuotas, err := loadTenantQuotas(tenantQuotaCfg)
	if err != nil {
		log.Fatalf("failed to load tenant quotas: %v", err)
	}

	engines := make(map[string]*decision.Engine, len(policies))
	windows := make(map[string][]decision.Telemetry, len(policies))
	lastEval := make(map[string]time.Time, len(policies))
	tenantLimiters := newTenantLimiters(policies, tenantQuotas, time.Now())
	startedAt := time.Now()

	for serviceID, policy := range policies {
		engines[serviceID] = decision.NewEngine(policy)
		lastEval[serviceID] = startedAt
		log.Printf(
			"loaded policy service=%s tenant=%s window=%ds",
			serviceID,
			policy.Tenant,
			policy.WindowSeconds,
		)
	}

	// 2️⃣ Redis store (state + idempotency)
	store := redis.New("localhost:6379")

	// 3️⃣ gRPC control plane
	grpcServer := grpcsrv.NewServer(store)
	go grpcsrv.Run(grpcServer)

	// 4️⃣ Kafka reader (telemetry)
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker},
		Topic:   telemetryTopic,
		GroupID: consumerGroup,
	})
	defer reader.Close()

	// 5️⃣ Kafka writer (decisions)
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{broker},
		Topic:   decisionTopic,
	})
	defer writer.Close()

	eventsCh := make(chan decision.Telemetry, 256)

	// 6️⃣ Non-blocking Kafka consumer
	go func() {
		for {
			msg, err := reader.ReadMessage(context.Background())
			if err != nil {
				log.Printf("kafka read error: %v", err)
				continue
			}

			var te rolloutpb.TelemetryEvent
			if err := proto.Unmarshal(msg.Value, &te); err != nil {
				log.Printf("invalid telemetry payload")
				continue
			}

			eventsCh <- decision.Telemetry{
				ServiceID: te.ServiceId,
				LatencyMs: te.LatencyMs,
				IsError:   te.Error,
				Timestamp: te.TimestampUnixMs,
			}
		}
	}()

	ticker := time.NewTicker(dispatchTick)
	defer ticker.Stop()

	// 7️⃣ Main control loop
	for {
		select {
		case ev := <-eventsCh:
			if _, ok := engines[ev.ServiceID]; !ok {
				log.Printf("dropping telemetry for unknown service=%s", ev.ServiceID)
				continue
			}
			windows[ev.ServiceID] = append(windows[ev.ServiceID], ev)

		case now := <-ticker.C:
			for serviceID, engine := range engines {
				windowDuration := time.Duration(engine.Policy.WindowSeconds) * time.Second
				if now.Sub(lastEval[serviceID]) < windowDuration {
					continue
				}
				tenant := engine.Policy.Tenant
				if !tenantLimiters[tenant].allow(now) {
					log.Printf("tenant quota exceeded tenant=%s service=%s", tenant, serviceID)
					continue
				}

				evaluateWindow(serviceID, tenant, engine, store, writer, grpcServer, windows[serviceID], now)
				windows[serviceID] = nil
				lastEval[serviceID] = now
			}
		}
	}
}

func evaluateWindow(
	serviceID string,
	tenant string,
	engine *decision.Engine,
	store *redis.Store,
	writer *kafka.Writer,
	grpcServer *grpcsrv.Server,
	events []decision.Telemetry,
	now time.Time,
) {
	result := engine.Evaluate(events)
	windowID := now.Truncate(time.Duration(engine.Policy.WindowSeconds) * time.Second).String()

	idempotencyKey := tenant + ":" + serviceID
	ok, err := store.IdempotentDecision(context.Background(), idempotencyKey, windowID)
	if err != nil {
		log.Printf("failed idempotency check: %v", err)
		return
	}
	if !ok {
		log.Printf("duplicate decision skipped service=%s", serviceID)
		return
	}

	version := "v1"
	prev, err := store.Get(context.Background(), serviceID)
	if err != nil {
		log.Printf("failed to get previous state service=%s: %v", serviceID, err)
	} else if prev != nil && prev.Version != "" {
		version = prev.Version
	}

	state := &redis.State{
		ServiceID:    serviceID,
		Version:      version,
		LastDecision: string(result),
		State:        mapRolloutState(result),
	}

	if err := store.Save(context.Background(), state); err != nil {
		log.Printf("failed to persist state: %v", err)
		return
	}

	event := &rolloutpb.DecisionEvent{
		ServiceId:       serviceID,
		Decision:        mapDecision(result),
		Reason:          "policy-based window evaluation",
		TimestampUnixMs: now.UnixMilli(),
	}

	bytes, err := proto.Marshal(event)
	if err != nil {
		log.Printf("failed to marshal decision: %v", err)
		return
	}

	if err := writer.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(serviceID),
		Value: bytes,
	}); err != nil {
		log.Printf("failed to publish decision to kafka: %v", err)
	}

	grpcServer.Publish(event)

	log.Printf("service=%s decision=%s events=%d", serviceID, result, len(events))
}

func loadPolicies(dir string) (map[string]*decision.Policy, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read policy directory: %w", err)
	}

	policies := make(map[string]*decision.Policy)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		if name == "tenants.yaml" || name == "tenants.yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		policy, err := decision.LoadPolicy(path)
		if err != nil {
			return nil, fmt.Errorf("load policy %s: %w", path, err)
		}
		if policy.Service == "" {
			return nil, fmt.Errorf("policy %s missing service", path)
		}
		if policy.Tenant == "" {
			policy.Tenant = "default"
		}
		if policy.WindowSeconds <= 0 {
			return nil, fmt.Errorf("policy %s has invalid window_seconds for service=%s", path, policy.Service)
		}
		if _, exists := policies[policy.Service]; exists {
			return nil, fmt.Errorf("duplicate policy service=%s", policy.Service)
		}

		policies[policy.Service] = policy
	}

	if len(policies) == 0 {
		return nil, fmt.Errorf("no policy files found in %s", dir)
	}

	return policies, nil
}

func loadTenantQuotas(path string) (*TenantQuotas, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var q TenantQuotas
	if err := yaml.Unmarshal(data, &q); err != nil {
		return nil, err
	}
	if q.Tenants == nil {
		q.Tenants = make(map[string]TenantQuota)
	}
	return &q, nil
}

func newTenantLimiters(
	policies map[string]*decision.Policy,
	quotas *TenantQuotas,
	now time.Time,
) map[string]*tenantLimiter {
	out := make(map[string]*tenantLimiter)

	for _, policy := range policies {
		tenant := policy.Tenant
		if _, exists := out[tenant]; exists {
			continue
		}

		maxPerMinute := 0
		if quotas != nil {
			maxPerMinute = quotas.DefaultQuotaPerMinute
			if q, ok := quotas.Tenants[tenant]; ok && q.MaxEvaluationsPerMinute > 0 {
				maxPerMinute = q.MaxEvaluationsPerMinute
			}
		}

		out[tenant] = &tenantLimiter{
			maxPerMinute: maxPerMinute,
			windowStart:  now,
		}
	}
	return out
}

func mapDecision(d decision.DecisionType) rolloutpb.DecisionType {
	switch d {
	case decision.Rollback:
		return rolloutpb.DecisionType_ROLLBACK
	case decision.Pause:
		return rolloutpb.DecisionType_PAUSE
	default:
		return rolloutpb.DecisionType_PROMOTE
	}
}

func mapRolloutState(d decision.DecisionType) redis.RolloutState {
	switch d {
	case decision.Rollback:
		return redis.RolledBack
	case decision.Pause:
		return redis.Paused
	default:
		return redis.Promoted
	}
}

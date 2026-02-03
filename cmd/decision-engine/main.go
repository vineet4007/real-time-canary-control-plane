package main

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	"github.com/vineet4007/real-time-canary-control-plane/internal/decision"
	grpcsrv "github.com/vineet4007/real-time-canary-control-plane/internal/grpc"
	"github.com/vineet4007/real-time-canary-control-plane/internal/redis"
	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
)

const (
	broker         = "localhost:9092"
	telemetryTopic = "telemetry.raw"
	decisionTopic  = "rollout.decisions"
	consumerGroup  = "decision-engine"
	serviceID      = "checkout-service"
)

func main() {
	log.Println("starting decision engine")

	// 1️⃣ Load rollout policy (Policy-as-Code)
	policy, err := decision.LoadPolicy("deploy/policies/checkout.yaml")
	if err != nil {
		log.Fatalf("failed to load policy: %v", err)
	}

	engine := decision.NewEngine(policy)

	// 2️⃣ Redis store (state + idempotency)
	store := redis.New("localhost:6379")

	// 3️⃣ gRPC control plane
	grpcServer := grpcsrv.NewServer()
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

	window := make([]decision.Telemetry, 0)
	ticker := time.NewTicker(time.Duration(policy.WindowSeconds) * time.Second)
	defer ticker.Stop()

	// 7️⃣ Main control loop
	for {
		select {
		case ev := <-eventsCh:
			window = append(window, ev)

		case <-ticker.C:
			evaluateWindow(engine, store, writer, grpcServer, window)
			window = nil
		}
	}
}

func evaluateWindow(
	engine *decision.Engine,
	store *redis.Store,
	writer *kafka.Writer,
	grpcServer *grpcsrv.Server,
	events []decision.Telemetry,
) {
	result := engine.Evaluate(events)
	windowID := time.Now().Truncate(30 * time.Second).String()

	ok, err := store.IdempotentDecision(context.Background(), serviceID, windowID)
	if err != nil || !ok {
		log.Printf("duplicate decision skipped")
		return
	}

	state := &redis.State{
		ServiceID:    serviceID,
		Version:      "v1",
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
		TimestampUnixMs: time.Now().UnixMilli(),
	}

	bytes, _ := proto.Marshal(event)

	writer.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(serviceID),
		Value: bytes,
	})

	grpcServer.Publish(event)

	log.Printf("decision=%s events=%d", result, len(events))
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

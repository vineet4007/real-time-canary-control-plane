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
	group          = "decision-engine"
	serviceID      = "checkout-service"
	windowSize     = 30 * time.Second
)

func main() {
	log.Println("starting decision engine")

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker},
		Topic:   telemetryTopic,
		GroupID: group,
	})

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{broker},
		Topic:   decisionTopic,
	})

	engine := decision.NewEngine()
	store := redis.New("localhost:6379")

	grpcServer := grpcsrv.NewServer()
	go grpcsrv.Run(grpcServer)

	eventsCh := make(chan decision.Telemetry, 100)

	// Kafka consumer goroutine (NEVER blocks main)
	go func() {
		for {
			msg, err := reader.ReadMessage(context.Background())
			if err != nil {
				log.Printf("kafka read error: %v", err)
				continue
			}

			var te rolloutpb.TelemetryEvent
			if err := proto.Unmarshal(msg.Value, &te); err != nil {
				log.Printf("invalid telemetry proto")
				continue
			}

			eventsCh <- decision.Telemetry{
				ServiceID: te.ServiceId,
				LatencyMs: te.LatencyMs,
				IsError:   te.Error,
				Timestamp: time.UnixMilli(te.TimestampUnixMs),
			}
		}
	}()

	ticker := time.NewTicker(windowSize)
	defer ticker.Stop()

	var window []decision.Telemetry

	for {
		select {
		case ev := <-eventsCh:
			window = append(window, ev)

		case <-ticker.C:
			processWindow(engine, store, writer, grpcServer, window)
			window = nil
		}
	}
}

func processWindow(
	engine *decision.Engine,
	store *redis.Store,
	writer *kafka.Writer,
	grpcServer *grpcsrv.Server,
	events []decision.Telemetry,
) {
	result := engine.Evaluate(events)
	windowID := time.Now().Truncate(windowSize).String()

	ok, err := store.IdempotentDecision(context.Background(), serviceID, windowID)
	if err != nil || !ok {
		return
	}

	state := &redis.State{
		ServiceID:    serviceID,
		Version:      "v1",
		LastDecision: string(result),
		State:        mapRolloutState(result),
	}

	store.Save(context.Background(), state)

	event := &rolloutpb.DecisionEvent{
		ServiceId:       serviceID,
		Decision:        mapDecision(result),
		Reason:          "grpc streamed decision",
		TimestampUnixMs: time.Now().UnixMilli(),
	}

	bytes, _ := proto.Marshal(event)

	writer.WriteMessages(context.Background(), kafka.Message{
		Key:   []byte(serviceID),
		Value: bytes,
	})

	grpcServer.Publish(event)

	log.Printf("decision=%s streamed events=%d", result, len(events))
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

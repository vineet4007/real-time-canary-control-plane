package main

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	"github.com/vineet4007/real-time-canary-control-plane/internal/decision"
	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
)

const (
	broker        = "localhost:9092"
	telemetryTopic = "telemetry.raw"
	decisionTopic  = "rollout.decisions"
	group          = "decision-engine"
)

func main() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{broker},
		Topic:   telemetryTopic,
		GroupID: group,
	})
	defer reader.Close()

	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers: []string{broker},
		Topic:   decisionTopic,
	})
	defer writer.Close()

	engine := decision.NewEngine()
	window := make([]decision.Telemetry, 0)
	ticker := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-ticker.C:
			result := engine.Evaluate(window)

			event := &rolloutpb.DecisionEvent{
				ServiceId:       "checkout-service",
				Decision:        mapDecision(result),
				Reason:          "window evaluation",
				TimestampUnixMs: time.Now().UnixMilli(),
			}

			bytes, _ := proto.Marshal(event)

			writer.WriteMessages(context.Background(), kafka.Message{
				Key:   []byte(event.ServiceId),
				Value: bytes,
			})

			log.Printf("decision=%s events=%d", result, len(window))
			window = nil

		default:
			msg, err := reader.ReadMessage(context.Background())
			if err != nil {
				log.Fatalf("kafka read failed: %v", err)
			}

			var te rolloutpb.TelemetryEvent
			if err := proto.Unmarshal(msg.Value, &te); err != nil {
				log.Printf("invalid proto payload")
				continue
			}

			window = append(window, decision.Telemetry{
				ServiceID: te.ServiceId,
				LatencyMs: te.LatencyMs,
				IsError:   te.Error,
				Timestamp: time.UnixMilli(te.TimestampUnixMs),
			})
		}
	}
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

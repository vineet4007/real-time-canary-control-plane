package main

import (
	"context"
	"log"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/vineet4007/real-time-canary-control-plane/internal/decision"
)

const (
	broker = "localhost:9092"
	topic  = "telemetry.raw"
	group  = "decision-engine"
)

func main() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{broker},
		Topic:       topic,
		GroupID:     group,
		StartOffset: kafka.LastOffset,
	})

	defer reader.Close()

	engine := decision.NewEngine()
	window := make([]decision.Telemetry, 0)

	ticker := time.NewTicker(30 * time.Second)

	for {
		select {
		case <-ticker.C:
			decisionResult := engine.Evaluate(window)
			log.Printf("decision=%s events=%d", decisionResult, len(window))
			window = nil

		default:
			msg, err := reader.ReadMessage(context.Background())
			if err != nil {
				log.Fatalf("kafka read failed: %v", err)
			}

			event := decision.Telemetry{
				ServiceID: string(msg.Key),
				LatencyMs: 200, // placeholder
				IsError:   false,
				Timestamp: time.Now(),
			}

			window = append(window, event)
		}
	}
}

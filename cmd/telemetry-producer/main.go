package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
)

const (
	kafkaBroker = "localhost:9092"
	topic       = "telemetry.raw"
	serviceID   = "checkout-service"
)

func main() {
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaBroker},
		Topic:    topic,
		Balancer: &kafka.Hash{},
	})
	defer writer.Close()

	rand.Seed(time.Now().UnixNano())

	for {
		event := &rolloutpb.TelemetryEvent{
			ServiceId:        serviceID,
			LatencyMs:        rand.Float64()*400 + 50,
			Error:            rand.Intn(100) < 5,
			TimestampUnixMs:  time.Now().UnixMilli(),
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

		log.Printf("sent telemetry proto latency=%.2f error=%v", event.LatencyMs, event.Error)
		time.Sleep(500 * time.Millisecond)
	}
}

package main

import (
	"context"
	"log"
	"math/rand"
	"time"

	"github.com/segmentio/kafka-go"
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
		latency := rand.Float64()*400 + 50
		isError := rand.Intn(100) < 5

		msg := kafka.Message{
			Key:   []byte(serviceID),
			Value: []byte(buildPayload(latency, isError)),
		}

		if err := writer.WriteMessages(context.Background(), msg); err != nil {
			log.Fatalf("failed to write message: %v", err)
		}

		log.Printf("sent telemetry: latency=%.2f error=%v", latency, isError)
		time.Sleep(500 * time.Millisecond)
	}
}

func buildPayload(latency float64, isError bool) string {
	return time.Now().Format(time.RFC3339)
}

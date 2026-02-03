package main

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
)

func main() {
	conn, err := grpc.Dial(
		"127.0.0.1:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	client := rolloutpb.NewRolloutControlClient(conn)

	stream, err := client.StreamDecisions(
		context.Background(),
		&rolloutpb.StreamDecisionsRequest{ServiceId: "checkout-service"},
	)
	if err != nil {
		log.Fatal(err)
	}

	for {
		event, err := stream.Recv()
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("STREAMED DECISION: %v", event.Decision)
	}
}

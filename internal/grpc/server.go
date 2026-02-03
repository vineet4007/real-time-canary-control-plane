package grpc

import (
	"log"
	"net"
	"sync"

	"google.golang.org/grpc"

	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
)

type Server struct {
	rolloutpb.UnimplementedRolloutControlServer
	subscribers map[string][]chan *rolloutpb.DecisionEvent
	mu          sync.Mutex
}

func NewServer() *Server {
	return &Server{
		subscribers: make(map[string][]chan *rolloutpb.DecisionEvent),
	}
}

func (s *Server) Publish(event *rolloutpb.DecisionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ch := range s.subscribers[event.ServiceId] {
		select {
		case ch <- event:
		default:
			log.Printf("dropping slow subscriber")
		}
	}
}

func (s *Server) StreamDecisions(
	req *rolloutpb.StreamDecisionsRequest,
	stream rolloutpb.RolloutControl_StreamDecisionsServer,
) error {
	ch := make(chan *rolloutpb.DecisionEvent, 10)

	s.mu.Lock()
	s.subscribers[req.ServiceId] = append(s.subscribers[req.ServiceId], ch)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		subs := s.subscribers[req.ServiceId]
		for i, c := range subs {
			if c == ch {
				s.subscribers[req.ServiceId] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}()

	for event := range ch {
		if err := stream.Send(event); err != nil {
			return err
		}
	}
	return nil
}

func Run(server *Server) {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	rolloutpb.RegisterRolloutControlServer(grpcServer, server)

	log.Println("gRPC server listening on :50051")
	grpcServer.Serve(lis)
}

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	controllerrollout "github.com/vineet4007/real-time-canary-control-plane/internal/controller/rollout"
	rolloutpb "github.com/vineet4007/real-time-canary-control-plane/internal/grpc/rolloutpb"
	"github.com/vineet4007/real-time-canary-control-plane/internal/redis"
)

const (
	defaultRedisAddr  = "localhost:6379"
	defaultGRPCAddr   = "127.0.0.1:50051"
	defaultRolloutDir = "deploy/k8s/rollouts"
	defaultStatusDir  = "deploy/k8s/status"
	syncInterval      = 10 * time.Second
)

type grpcStarter struct {
	client rolloutpb.RolloutControlClient
}

func (s *grpcStarter) StartRollout(ctx context.Context, serviceID, version string) error {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.client.StartRollout(callCtx, &rolloutpb.StartRolloutRequest{
		ServiceId: serviceID,
		Version:   version,
	})
	return err
}

func main() {
	grpcAddr := getEnv("ROLLOUT_CONTROLLER_GRPC_ADDR", defaultGRPCAddr)
	redisAddr := getEnv("ROLLOUT_CONTROLLER_REDIS_ADDR", defaultRedisAddr)
	rolloutDir := getEnv("ROLLOUT_CONTROLLER_DIR", defaultRolloutDir)
	statusDir := getEnv("ROLLOUT_CONTROLLER_STATUS_DIR", defaultStatusDir)
	writeStatus := parseBool(getEnv("ROLLOUT_CONTROLLER_WRITE_STATUS", "false"))
	enableStatusChecks := parseBool(getEnv("ROLLOUT_CONTROLLER_ENABLE_STATUS_CHECKS", "true"))

	conn, err := grpc.Dial(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to dial decision engine grpc: %v", err)
	}
	defer conn.Close()

	var deployments controllerrollout.DeploymentStatusProvider
	if enableStatusChecks {
		deployments = &fileDeploymentStatusProvider{dir: statusDir}
	}

	reconciler := &controllerrollout.Reconciler{
		Starter: &grpcStarter{
			client: rolloutpb.NewRolloutControlClient(conn),
		},
		Store:       redis.New(redisAddr),
		Deployments: deployments,
	}

	log.Printf(
		"rollout-controller started grpc=%s redis=%s dir=%s statusDir=%s statusChecks=%t writeStatus=%t",
		grpcAddr,
		redisAddr,
		rolloutDir,
		statusDir,
		enableStatusChecks,
		writeStatus,
	)

	if err := syncOnce(context.Background(), reconciler, rolloutDir, writeStatus); err != nil {
		log.Printf("initial reconcile pass error: %v", err)
	}

	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := syncOnce(context.Background(), reconciler, rolloutDir, writeStatus); err != nil {
			log.Printf("reconcile pass error: %v", err)
		}
	}
}

func syncOnce(
	ctx context.Context,
	reconciler *controllerrollout.Reconciler,
	dir string,
	writeStatus bool,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var hadError bool

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		rollout, err := loadRollout(path)
		if err != nil {
			log.Printf("skip invalid rollout file=%s error=%v", path, err)
			hadError = true
			continue
		}

		if err := reconciler.Reconcile(ctx, rollout); err != nil {
			log.Printf(
				"reconcile failed file=%s service=%s version=%s error=%v",
				path,
				rollout.Spec.ServiceID,
				rollout.Spec.TargetVersion,
				err,
			)
			hadError = true
			continue
		}

		log.Printf(
			"reconciled file=%s service=%s version=%s phase=%s decision=%s",
			path,
			rollout.Spec.ServiceID,
			rollout.Spec.TargetVersion,
			rollout.Status.Phase,
			rollout.Status.LastDecision,
		)

		if writeStatus {
			if err := persistRollout(path, rollout); err != nil {
				log.Printf("persist status failed file=%s error=%v", path, err)
				hadError = true
			}
		}
	}

	if hadError {
		return errors.New("one or more rollout resources failed to reconcile")
	}
	return nil
}

func loadRollout(path string) (*controllerrollout.CanaryRollout, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cr controllerrollout.CanaryRollout
	if err := yaml.Unmarshal(data, &cr); err != nil {
		return nil, err
	}
	if cr.Metadata.Name == "" {
		cr.Metadata.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return &cr, nil
}

func persistRollout(path string, cr *controllerrollout.CanaryRollout) error {
	bytes, err := yaml.Marshal(cr)
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

func getEnv(key, fallback string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	return raw
}

func parseBool(raw string) bool {
	v, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return v
}

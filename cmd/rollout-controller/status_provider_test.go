package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileDeploymentStatusProviderParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkout-service.yaml")

	content := `serviceId: checkout-service
canary:
  readyReplicas: 1
  desiredReplicas: 1
stable:
  readyReplicas: 3
  desiredReplicas: 3
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	p := &fileDeploymentStatusProvider{dir: dir}
	status, err := p.GetStatus(context.Background(), "default", "checkout-service")
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if status == nil {
		t.Fatalf("expected status, got nil")
	}
	if !status.CanaryReady() {
		t.Fatalf("expected canary ready")
	}
	if !status.StableReady() {
		t.Fatalf("expected stable ready")
	}
}

func TestFileDeploymentStatusProviderMissingFile(t *testing.T) {
	p := &fileDeploymentStatusProvider{dir: t.TempDir()}
	status, err := p.GetStatus(context.Background(), "default", "does-not-exist")
	if err != nil {
		t.Fatalf("GetStatus error: %v", err)
	}
	if status != nil {
		t.Fatalf("expected nil status for missing file")
	}
}

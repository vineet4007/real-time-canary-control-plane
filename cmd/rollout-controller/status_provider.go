package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	controllerrollout "github.com/vineet4007/real-time-canary-control-plane/internal/controller/rollout"
)

type deploymentReplicas struct {
	ReadyReplicas   int `yaml:"readyReplicas"`
	DesiredReplicas int `yaml:"desiredReplicas"`
}

type deploymentStatusSnapshot struct {
	ServiceID string             `yaml:"serviceId"`
	Canary    deploymentReplicas `yaml:"canary"`
	Stable    deploymentReplicas `yaml:"stable"`
}

type fileDeploymentStatusProvider struct {
	dir string
}

func (p *fileDeploymentStatusProvider) GetStatus(
	_ context.Context,
	_ string,
	serviceID string,
) (*controllerrollout.DeploymentStatus, error) {
	path := filepath.Join(p.dir, serviceID+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var snap deploymentStatusSnapshot
	if err := yaml.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse deployment snapshot %s: %w", path, err)
	}

	return &controllerrollout.DeploymentStatus{
		CanaryReadyReplicas:   snap.Canary.ReadyReplicas,
		CanaryDesiredReplicas: snap.Canary.DesiredReplicas,
		StableReadyReplicas:   snap.Stable.ReadyReplicas,
		StableDesiredReplicas: snap.Stable.DesiredReplicas,
	}, nil
}

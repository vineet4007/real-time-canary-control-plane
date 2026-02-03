package decision

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Policy struct {
	Service       string `yaml:"service"`
	WindowSeconds int    `yaml:"window_seconds"`

	Thresholds struct {
		ErrorRate float64 `yaml:"error_rate"`
		LatencyMs float64 `yaml:"latency_ms"`
	} `yaml:"thresholds"`

	Actions struct {
		OnError   DecisionType `yaml:"on_error"`
		OnLatency DecisionType `yaml:"on_latency"`
		OnSuccess DecisionType `yaml:"on_success"`
	} `yaml:"actions"`
}

func LoadPolicy(path string) (*Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}

	return &p, nil
}

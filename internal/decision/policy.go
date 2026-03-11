package decision

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Policy struct {
	Service       string `yaml:"service"`
	Tenant        string `yaml:"tenant"`
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

	SLO struct {
		Enabled            bool         `yaml:"enabled"`
		MinEvents          int          `yaml:"min_events"`
		AvailabilityTarget float64      `yaml:"availability_target"`
		LatencyP95Ms       float64      `yaml:"latency_p95_ms"`
		OnBreach           DecisionType `yaml:"on_breach"`
	} `yaml:"slo"`

	Adaptive struct {
		Enabled             bool    `yaml:"enabled"`
		MinEvents           int     `yaml:"min_events"`
		Alpha               float64 `yaml:"alpha"`
		ErrorRateMultiplier float64 `yaml:"error_rate_multiplier"`
		LatencyMultiplier   float64 `yaml:"latency_multiplier"`
		ErrorRateMin        float64 `yaml:"error_rate_min"`
		ErrorRateMax        float64 `yaml:"error_rate_max"`
		LatencyMinMs        float64 `yaml:"latency_min_ms"`
		LatencyMaxMs        float64 `yaml:"latency_max_ms"`
	} `yaml:"adaptive"`
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

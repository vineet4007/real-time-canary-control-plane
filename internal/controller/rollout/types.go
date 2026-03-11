package rollout

type ObjectMeta struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type CanaryRollout struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   ObjectMeta          `yaml:"metadata"`
	Spec       CanaryRolloutSpec   `yaml:"spec"`
	Status     CanaryRolloutStatus `yaml:"status,omitempty"`
}

type CanaryRolloutSpec struct {
	ServiceID     string `yaml:"serviceId"`
	TargetVersion string `yaml:"targetVersion"`
	PolicyRef     string `yaml:"policyRef"`
}

type CanaryRolloutStatus struct {
	Phase           Phase  `yaml:"phase,omitempty"`
	Reason          string `yaml:"reason,omitempty"`
	LastDecision    string `yaml:"lastDecision,omitempty"`
	ObservedVersion string `yaml:"observedVersion,omitempty"`
	LastUpdatedUnix int64  `yaml:"lastUpdatedUnixMs,omitempty"`
}

type Phase string

const (
	PhasePending    Phase = "Pending"
	PhaseCanary     Phase = "Canary"
	PhasePaused     Phase = "Paused"
	PhasePromoted   Phase = "Promoted"
	PhaseRolledBack Phase = "RolledBack"
)

type DeploymentStatus struct {
	CanaryReadyReplicas   int
	CanaryDesiredReplicas int
	StableReadyReplicas   int
	StableDesiredReplicas int
}

func (d DeploymentStatus) CanaryReady() bool {
	if d.CanaryDesiredReplicas <= 0 {
		return false
	}
	return d.CanaryReadyReplicas >= d.CanaryDesiredReplicas
}

func (d DeploymentStatus) StableReady() bool {
	if d.StableDesiredReplicas <= 0 {
		return false
	}
	return d.StableReadyReplicas >= d.StableDesiredReplicas
}

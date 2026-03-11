package rollout

import (
	"context"
	"fmt"

	"github.com/vineet4007/real-time-canary-control-plane/internal/redis"
)

type StartRolloutClient interface {
	StartRollout(ctx context.Context, serviceID, version string) error
}

type StateStore interface {
	Get(ctx context.Context, serviceID string) (*redis.State, error)
}

type DeploymentStatusProvider interface {
	GetStatus(ctx context.Context, namespace, serviceID string) (*DeploymentStatus, error)
}

type Reconciler struct {
	Starter     StartRolloutClient
	Store       StateStore
	Deployments DeploymentStatusProvider
}

func (r *Reconciler) Reconcile(ctx context.Context, cr *CanaryRollout) error {
	if cr == nil {
		return fmt.Errorf("nil rollout")
	}
	if cr.Spec.ServiceID == "" {
		return fmt.Errorf("spec.serviceId is required")
	}
	if cr.Spec.TargetVersion == "" {
		return fmt.Errorf("spec.targetVersion is required")
	}
	if r.Starter == nil {
		return fmt.Errorf("start rollout client is not configured")
	}
	if r.Store == nil {
		return fmt.Errorf("state store is not configured")
	}

	if cr.Status.Phase == "" {
		cr.Status.Phase = PhasePending
	}

	// Trigger rollout once for each new target version.
	if cr.Status.ObservedVersion != cr.Spec.TargetVersion {
		if err := r.Starter.StartRollout(ctx, cr.Spec.ServiceID, cr.Spec.TargetVersion); err != nil {
			return fmt.Errorf("start rollout: %w", err)
		}
		cr.Status.ObservedVersion = cr.Spec.TargetVersion
		cr.Status.Phase = PhaseCanary
		cr.Status.Reason = "rollout started"
	}

	state, err := r.Store.Get(ctx, cr.Spec.ServiceID)
	if err != nil {
		return fmt.Errorf("read rollout state: %w", err)
	}
	if state == nil {
		return nil
	}

	cr.Status.LastDecision = state.LastDecision
	cr.Status.LastUpdatedUnix = state.LastUpdated
	cr.Status.Phase = phaseFromState(state.State, cr.Status.Phase)
	cr.Status.Reason = reasonFromState(state.State)

	if r.Deployments != nil {
		namespace := cr.Metadata.Namespace
		if namespace == "" {
			namespace = "default"
		}
		deployStatus, err := r.Deployments.GetStatus(ctx, namespace, cr.Spec.ServiceID)
		if err != nil {
			return fmt.Errorf("read deployment status: %w", err)
		}
		if deployStatus != nil {
			applyDeploymentStatus(&cr.Status, *deployStatus)
		}
	}

	return nil
}

func phaseFromState(s redis.RolloutState, fallback Phase) Phase {
	switch s {
	case redis.Canary:
		return PhaseCanary
	case redis.Paused:
		return PhasePaused
	case redis.Promoted:
		return PhasePromoted
	case redis.RolledBack:
		return PhaseRolledBack
	default:
		return fallback
	}
}

func reasonFromState(s redis.RolloutState) string {
	switch s {
	case redis.Canary:
		return "canary in progress"
	case redis.Paused:
		return "paused by decision engine"
	case redis.Promoted:
		return "promoted by decision engine"
	case redis.RolledBack:
		return "rolled back by decision engine"
	default:
		return ""
	}
}

func applyDeploymentStatus(status *CanaryRolloutStatus, d DeploymentStatus) {
	if status == nil {
		return
	}

	if status.Phase != PhaseRolledBack && !d.CanaryReady() {
		status.Phase = PhasePaused
		status.Reason = fmt.Sprintf(
			"canary deployment not ready (%d/%d)",
			d.CanaryReadyReplicas,
			d.CanaryDesiredReplicas,
		)
		return
	}

	if status.Phase == PhasePromoted {
		if !d.StableReady() {
			status.Phase = PhasePaused
			status.Reason = fmt.Sprintf(
				"promotion waiting for stable readiness (%d/%d)",
				d.StableReadyReplicas,
				d.StableDesiredReplicas,
			)
			return
		}
		status.Reason = fmt.Sprintf(
			"promoted with stable ready (%d/%d)",
			d.StableReadyReplicas,
			d.StableDesiredReplicas,
		)
		return
	}

	if status.Phase == PhaseRolledBack && d.StableReady() {
		status.Reason = fmt.Sprintf(
			"rolled back with stable ready (%d/%d)",
			d.StableReadyReplicas,
			d.StableDesiredReplicas,
		)
		return
	}

	if status.Phase == PhaseCanary {
		status.Reason = fmt.Sprintf(
			"canary ready (%d/%d)",
			d.CanaryReadyReplicas,
			d.CanaryDesiredReplicas,
		)
	}
}

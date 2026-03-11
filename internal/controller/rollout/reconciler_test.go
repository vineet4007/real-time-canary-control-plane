package rollout

import (
	"context"
	"testing"

	"github.com/vineet4007/real-time-canary-control-plane/internal/redis"
)

type fakeStarter struct {
	calls int
	last  struct {
		service string
		version string
	}
	err error
}

func (f *fakeStarter) StartRollout(_ context.Context, serviceID, version string) error {
	f.calls++
	f.last.service = serviceID
	f.last.version = version
	return f.err
}

type fakeStore struct {
	st  *redis.State
	err error
}

func (f *fakeStore) Get(_ context.Context, _ string) (*redis.State, error) {
	return f.st, f.err
}

type fakeDeployments struct {
	status *DeploymentStatus
	err    error
}

func (f *fakeDeployments) GetStatus(_ context.Context, _, _ string) (*DeploymentStatus, error) {
	return f.status, f.err
}

func TestReconcileStartsNewVersion(t *testing.T) {
	starter := &fakeStarter{}
	store := &fakeStore{
		st: &redis.State{
			ServiceID:    "checkout-service",
			Version:      "v2",
			State:        redis.Canary,
			LastDecision: "START_ROLLOUT",
			LastUpdated:  123,
		},
	}
	r := &Reconciler{Starter: starter, Store: store}

	cr := &CanaryRollout{
		Spec: CanaryRolloutSpec{
			ServiceID:     "checkout-service",
			TargetVersion: "v2",
			PolicyRef:     "checkout.yaml",
		},
	}

	if err := r.Reconcile(context.Background(), cr); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	if starter.calls != 1 {
		t.Fatalf("expected 1 rollout start call, got %d", starter.calls)
	}
	if cr.Status.ObservedVersion != "v2" {
		t.Fatalf("expected observed version v2, got %s", cr.Status.ObservedVersion)
	}
	if cr.Status.Phase != PhaseCanary {
		t.Fatalf("expected phase=%s got=%s", PhaseCanary, cr.Status.Phase)
	}
}

func TestReconcileDoesNotRestartSameVersion(t *testing.T) {
	starter := &fakeStarter{}
	store := &fakeStore{
		st: &redis.State{
			ServiceID:    "checkout-service",
			Version:      "v2",
			State:        redis.Promoted,
			LastDecision: "PROMOTE",
			LastUpdated:  456,
		},
	}
	r := &Reconciler{Starter: starter, Store: store}

	cr := &CanaryRollout{
		Spec: CanaryRolloutSpec{
			ServiceID:     "checkout-service",
			TargetVersion: "v2",
			PolicyRef:     "checkout.yaml",
		},
		Status: CanaryRolloutStatus{
			ObservedVersion: "v2",
			Phase:           PhaseCanary,
		},
	}

	if err := r.Reconcile(context.Background(), cr); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	if starter.calls != 0 {
		t.Fatalf("expected no rollout start call, got %d", starter.calls)
	}
	if cr.Status.Phase != PhasePromoted {
		t.Fatalf("expected phase=%s got=%s", PhasePromoted, cr.Status.Phase)
	}
	if cr.Status.Reason == "" {
		t.Fatalf("expected reason to be populated")
	}
}

func TestReconcilePausesWhenCanaryNotReady(t *testing.T) {
	starter := &fakeStarter{}
	store := &fakeStore{
		st: &redis.State{
			ServiceID:    "checkout-service",
			Version:      "v2",
			State:        redis.Canary,
			LastDecision: "PAUSE",
			LastUpdated:  456,
		},
	}
	deployments := &fakeDeployments{
		status: &DeploymentStatus{
			CanaryReadyReplicas:   0,
			CanaryDesiredReplicas: 1,
			StableReadyReplicas:   3,
			StableDesiredReplicas: 3,
		},
	}
	r := &Reconciler{
		Starter:     starter,
		Store:       store,
		Deployments: deployments,
	}

	cr := &CanaryRollout{
		Spec: CanaryRolloutSpec{
			ServiceID:     "checkout-service",
			TargetVersion: "v2",
		},
		Status: CanaryRolloutStatus{
			ObservedVersion: "v2",
			Phase:           PhaseCanary,
		},
	}

	if err := r.Reconcile(context.Background(), cr); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	if cr.Status.Phase != PhasePaused {
		t.Fatalf("expected phase=%s got=%s", PhasePaused, cr.Status.Phase)
	}
}

func TestReconcilePausesPromotionWhenStableNotReady(t *testing.T) {
	starter := &fakeStarter{}
	store := &fakeStore{
		st: &redis.State{
			ServiceID:    "checkout-service",
			Version:      "v2",
			State:        redis.Promoted,
			LastDecision: "PROMOTE",
			LastUpdated:  456,
		},
	}
	deployments := &fakeDeployments{
		status: &DeploymentStatus{
			CanaryReadyReplicas:   1,
			CanaryDesiredReplicas: 1,
			StableReadyReplicas:   1,
			StableDesiredReplicas: 3,
		},
	}
	r := &Reconciler{
		Starter:     starter,
		Store:       store,
		Deployments: deployments,
	}

	cr := &CanaryRollout{
		Spec: CanaryRolloutSpec{
			ServiceID:     "checkout-service",
			TargetVersion: "v2",
		},
		Status: CanaryRolloutStatus{
			ObservedVersion: "v2",
			Phase:           PhaseCanary,
		},
	}

	if err := r.Reconcile(context.Background(), cr); err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	if cr.Status.Phase != PhasePaused {
		t.Fatalf("expected phase=%s got=%s", PhasePaused, cr.Status.Phase)
	}
}

func TestReconcileValidation(t *testing.T) {
	r := &Reconciler{
		Starter: &fakeStarter{},
		Store:   &fakeStore{},
	}

	cr := &CanaryRollout{
		Spec: CanaryRolloutSpec{
			ServiceID:     "",
			TargetVersion: "",
		},
	}

	if err := r.Reconcile(context.Background(), cr); err == nil {
		t.Fatalf("expected validation error, got nil")
	}
}

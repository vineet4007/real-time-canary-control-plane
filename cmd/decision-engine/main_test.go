package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vineet4007/real-time-canary-control-plane/internal/decision"
)

const testPolicyBody = `window_seconds: 30
thresholds:
  error_rate: 0.05
  latency_ms: 500
actions:
  on_error: ROLLBACK
  on_latency: PAUSE
  on_success: PROMOTE
`

func TestLoadPolicies(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(
		filepath.Join(dir, "checkout.yaml"),
		[]byte("service: checkout-service\n"+testPolicyBody),
		0o644,
	)
	if err != nil {
		t.Fatalf("write policy: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(dir, "payments.yaml"),
		[]byte("service: payments-service\n"+testPolicyBody),
		0o644,
	)
	if err != nil {
		t.Fatalf("write policy: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(dir, "tenants.yaml"),
		[]byte("default_quota_per_minute: 10\n"),
		0o644,
	)
	if err != nil {
		t.Fatalf("write tenants config: %v", err)
	}

	policies, err := loadPolicies(dir)
	if err != nil {
		t.Fatalf("loadPolicies returned error: %v", err)
	}

	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(policies))
	}
	if _, ok := policies["checkout-service"]; !ok {
		t.Fatalf("missing checkout-service policy")
	}
	if _, ok := policies["payments-service"]; !ok {
		t.Fatalf("missing payments-service policy")
	}
	if policies["checkout-service"].Tenant != "default" {
		t.Fatalf("expected default tenant, got %s", policies["checkout-service"].Tenant)
	}
}

func TestLoadPoliciesDuplicateService(t *testing.T) {
	dir := t.TempDir()

	err := os.WriteFile(
		filepath.Join(dir, "a.yaml"),
		[]byte("service: checkout-service\n"+testPolicyBody),
		0o644,
	)
	if err != nil {
		t.Fatalf("write policy: %v", err)
	}

	err = os.WriteFile(
		filepath.Join(dir, "b.yaml"),
		[]byte("service: checkout-service\n"+testPolicyBody),
		0o644,
	)
	if err != nil {
		t.Fatalf("write policy: %v", err)
	}

	_, err = loadPolicies(dir)
	if err == nil {
		t.Fatalf("expected duplicate-service error, got nil")
	}
}

func TestLoadTenantQuotas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenants.yaml")
	content := `default_quota_per_minute: 10
tenants:
  team-a:
    max_evaluations_per_minute: 20
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write quotas: %v", err)
	}

	quotas, err := loadTenantQuotas(path)
	if err != nil {
		t.Fatalf("loadTenantQuotas error: %v", err)
	}

	if quotas.DefaultQuotaPerMinute != 10 {
		t.Fatalf("expected default quota 10, got %d", quotas.DefaultQuotaPerMinute)
	}
	if quotas.Tenants["team-a"].MaxEvaluationsPerMinute != 20 {
		t.Fatalf("expected team-a quota 20, got %d", quotas.Tenants["team-a"].MaxEvaluationsPerMinute)
	}
}

func TestTenantLimiter(t *testing.T) {
	now := time.Now()
	limiter := &tenantLimiter{
		maxPerMinute: 2,
		windowStart:  now,
	}

	if !limiter.allow(now) {
		t.Fatalf("first allow should pass")
	}
	if !limiter.allow(now.Add(1 * time.Second)) {
		t.Fatalf("second allow should pass")
	}
	if limiter.allow(now.Add(2 * time.Second)) {
		t.Fatalf("third allow should fail in same minute window")
	}
	if !limiter.allow(now.Add(61 * time.Second)) {
		t.Fatalf("allow should reset after minute boundary")
	}
}

func TestNewTenantLimiters(t *testing.T) {
	policies := map[string]*decision.Policy{
		"checkout-service": {
			Service: "checkout-service",
			Tenant:  "team-retail",
		},
		"payments-service": {
			Service: "payments-service",
			Tenant:  "team-payments",
		},
	}
	quotas := &TenantQuotas{
		DefaultQuotaPerMinute: 7,
		Tenants: map[string]TenantQuota{
			"team-retail": {MaxEvaluationsPerMinute: 15},
		},
	}

	limiters := newTenantLimiters(policies, quotas, time.Now())
	if limiters["team-retail"].maxPerMinute != 15 {
		t.Fatalf("expected team-retail quota 15, got %d", limiters["team-retail"].maxPerMinute)
	}
	if limiters["team-payments"].maxPerMinute != 7 {
		t.Fatalf("expected team-payments default quota 7, got %d", limiters["team-payments"].maxPerMinute)
	}
}

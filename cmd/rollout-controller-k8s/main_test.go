package main

import "testing"

func TestServiceDeploymentNames(t *testing.T) {
	canary, stable := serviceDeploymentNames("checkout-service")
	if canary != "checkout-canary" {
		t.Fatalf("unexpected canary name: %s", canary)
	}
	if stable != "checkout-stable" {
		t.Fatalf("unexpected stable name: %s", stable)
	}
}

func TestDesiredReplicas(t *testing.T) {
	if got := desiredReplicas(nil); got != 1 {
		t.Fatalf("expected default replicas=1 got=%d", got)
	}
	v := int32(3)
	if got := desiredReplicas(&v); got != 3 {
		t.Fatalf("expected replicas=3 got=%d", got)
	}
}

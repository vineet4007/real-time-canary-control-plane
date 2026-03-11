# Real-Time Canary Control Plane

A production-grade, real-time deployment decision engine that evaluates live
telemetry streams and autonomously **promotes, pauses, or rolls back canary
deployments** within seconds.

This project focuses on the hardest part of modern CD systems:
**safe, low-latency, stateful decision-making under noisy telemetry**.

---

## Problem Statement

Modern distributed systems emit **millions of telemetry signals per minute**.
Human-driven deployments and static CI/CD pipelines cannot react fast enough to
prevent user impact during faulty rollouts.

Key challenges:
- Telemetry is noisy and bursty
- Decisions must be **fast but stable**
- Rollouts must be **restart-safe and idempotent**
- Operators need **real-time visibility**, not polling dashboards

This project implements a **real-time canary control plane** that continuously
ingests telemetry, evaluates rollout health, and emits deterministic deployment
decisions within **sub-5-second latency**.

---

## High-Level Architecture

Telemetry Producer
|
v
Kafka (telemetry.raw)
|
v
Decision Engine

Sliding window evaluation

Error-rate + latency thresholds

Redis-backed idempotency
|
+--> Kafka (rollout.decisions)
|
+--> gRPC Streaming API
|
v
Redis (rollout state)


---

## Core Capabilities

- Real-time telemetry ingestion via Kafka
- Typed contracts using Protobuf + gRPC
- Sliding window evaluation (service-specific windows)
- Deterministic decisions: PROMOTE / PAUSE / ROLLBACK
- Idempotent rollout handling using Redis
- Restart-safe control plane
- Live decision streaming over gRPC
- Multi-service + multi-tenant policy routing
- Tenant quota enforcement (evaluation budgets per minute)
- Adaptive thresholds (online EWMA-based tuning)
- Kubernetes-ready canary deployments + controller-runtime reconciler

---

## Decision Logic (Simplified)

For each service and time window:

- Compute:
  - Error rate
  - Average latency
- Apply rules:
  - Error rate > 5% → **ROLLBACK**
  - Avg latency > 500ms → **PAUSE**
  - Otherwise → **PROMOTE**

Decisions are:
- Idempotent (same window never applied twice)
- Stateful (persisted in Redis)
- Auditable (decision events published to Kafka)

---

## Technology Stack

### Control Plane
- Go – decision engine and gRPC server
- gRPC (streaming) – real-time decision subscription
- Protobuf – stable API contracts

### Data Plane
- Kafka – telemetry ingestion and decision events
- Redis – rollout state and idempotency

### Infrastructure
- Docker / Docker Compose – local infrastructure
- Kubernetes – canary and stable deployments
- HPA – auto-scaling canary workloads

---

## Repository Structure

cmd/
decision-engine/ # Core control plane
telemetry-producer/ # Synthetic telemetry generator
rollout-controller/ # CRD-style reconciliation scaffold
rollout-controller-k8s/ # controller-runtime Kubernetes reconciler

internal/
decision/ # Sliding window decision logic
grpc/ # gRPC server and streaming
redis/ # Rollout state and idempotency
controller/ # rollout reconciliation domain logic
k8s/ # Kubernetes API types (CanaryRollout)

proto/
rollout.proto # API contracts

deploy/
k8s/
canary-deployment.yaml
stable-deployment.yaml
hpa.yaml
crd/
canaryrollouts.controlplane.io.yaml
rollouts/
checkout-rollout.yaml
payments-rollout.yaml
status/
checkout-service.yaml
payments-service.yaml
policies/
checkout.yaml
payments.yaml
tenants.yaml


---

## Running Locally

### 1. Start Infrastructure
```bash
docker compose up -d
```

### 2. Run Decision Engine
```bash
go run cmd/decision-engine/main.go
```

### 3. Run Rollout Controller Scaffold
```bash
go run cmd/rollout-controller/main.go
```

Optional: write reconciled status back to rollout manifests.
```bash
ROLLOUT_CONTROLLER_WRITE_STATUS=true \
go run cmd/rollout-controller/main.go
```

Optional: control deployment status checks (defaults to enabled).
```bash
ROLLOUT_CONTROLLER_ENABLE_STATUS_CHECKS=true \
ROLLOUT_CONTROLLER_STATUS_DIR=deploy/k8s/status \
go run cmd/rollout-controller/main.go
```

### 4. Stream Decisions (optional observer)
```bash
go run scripts/grpc_client.go
```

### 5. Run Kubernetes controller-runtime reconciler (cluster mode)
```bash
go run cmd/rollout-controller-k8s/main.go
```

### 6. Run Telemetry Producer (single service default)
```bash
go run cmd/telemetry-producer/main.go
```

### 7. Run Telemetry Producer (multi-service)
```bash
TELEMETRY_SERVICES=checkout-service,payments-service \
go run cmd/telemetry-producer/main.go
```

### 8. Enable Chaos Injection (failure simulation)
```bash
TELEMETRY_SERVICES=checkout-service,payments-service \
TELEMETRY_CHAOS_MODE=mixed \
TELEMETRY_CHAOS_INTERVAL_SEC=60 \
TELEMETRY_CHAOS_DURATION_SEC=15 \
TELEMETRY_CHAOS_ERROR_RATE=0.35 \
TELEMETRY_CHAOS_LATENCY_MS=1200 \
go run cmd/telemetry-producer/main.go
```

Supported `TELEMETRY_CHAOS_MODE` values:
- `off` (default)
- `error_burst`
- `latency_spike`
- `mixed`

### 9. (Kubernetes) Apply CRD and sample rollouts
```bash
kubectl apply -f deploy/k8s/crd/canaryrollouts.controlplane.io.yaml
kubectl apply -f deploy/k8s/rollouts/
```

### 10. Configure tenant quotas
```bash
cat deploy/policies/tenants.yaml
```

This file controls per-tenant evaluation budgets (`max_evaluations_per_minute`).

### 11. Configure adaptive thresholds
Each service policy now supports:
- `adaptive.enabled`
- `adaptive.alpha`
- `adaptive.error_rate_multiplier` / `adaptive.latency_multiplier`
- clamp bounds (`error_rate_min/max`, `latency_min_ms/max_ms`)

See:
- `deploy/policies/checkout.yaml`
- `deploy/policies/payments.yaml`

## Why This Is Not a Demo Project

This project intentionally focuses on depth over breadth:

- No UI dashboards
- No fake metrics
- No stateless shortcuts
- No polling APIs

Instead, it demonstrates:

- Correct concurrency models
- Failure-aware control planes
- Real-time streaming semantics
- Restart-safe rollout handling

This is the core logic behind systems like Argo Rollouts, Spinnaker,
and internal CD platforms at large-scale tech companies.


## Non-Goals

- Long-term metrics storage
- UI dashboards
- Full CI/CD platform
- Multi-cluster orchestration

These are deliberate exclusions to keep the system focused and reviewable.

## Future Extensions

- Model-based anomaly detection (beyond current EWMA adaptive thresholds)
- Kubernetes admission policy integration for rollout guardrails
- Multi-cluster rollout orchestration

#
This project was built to demonstrate senior-level distributed systems
engineering, not to maximize feature count.

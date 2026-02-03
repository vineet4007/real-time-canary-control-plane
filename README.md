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
- Sliding window evaluation (30s windows)
- Deterministic decisions: PROMOTE / PAUSE / ROLLBACK
- Idempotent rollout handling using Redis
- Restart-safe control plane
- Live decision streaming over gRPC
- Kubernetes-ready canary deployments

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

internal/
decision/ # Sliding window decision logic
grpc/ # gRPC server and streaming
redis/ # Rollout state and idempotency

proto/
rollout.proto # API contracts

deploy/
k8s/
canary-deployment.yaml
stable-deployment.yaml
service.yaml
hpa.yaml


---

## Running Locally

### 1. Start Infrastructure
```bash
docker compose up -d

go run cmd/decision-engine/main.go

gRPC server listening on :50051

go run cmd/telemetry-producer/main.go

go run scripts/grpc_client.go

STREAMED DECISION: PROMOTE
STREAMED DECISION: ROLLBACK


Why This Is Not a Demo Project

This project intentionally focuses on depth over breadth:

No UI dashboards

No fake metrics

No stateless shortcuts

No polling APIs

Instead, it demonstrates:

Correct concurrency models

Failure-aware control planes

Real-time streaming semantics

Restart-safe rollout handling

This is the core logic behind systems like Argo Rollouts, Spinnaker,
and internal CD platforms at large-scale tech companies.


Non-Goals

Long-term metrics storage

UI dashboards

Full CI/CD platform

Multi-cluster orchestration

These are deliberate exclusions to keep the system focused and reviewable.

Future Extensions

Failure injection and chaos testing

Custom Kubernetes controller (CRD-based rollouts)

Adaptive thresholds via ML

Multi-service, multi-tenant support

SLO-based rollout policies

#
This project was built to demonstrate senior-level distributed systems
engineering, not to maximize feature count.
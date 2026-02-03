# Interview Walkthrough — Real-Time Canary Control Plane

This document explains **how to present and defend** this project in technical
interviews for Senior Backend / Platform / Infra roles.

---

## 30-Second Summary (Elevator Pitch)

> I built a real-time canary control plane that ingests live telemetry via Kafka,
evaluates rollout health using sliding windows, persists rollout state with
Redis for idempotency, and streams deployment decisions live over gRPC.
Rollout behavior is defined using policy-as-code (YAML), so operators can change
behavior without redeploying the system.

---

## 2-Minute Deep Dive (Most Common Interview Flow)

> Most CI/CD systems focus on pipelines. I focused on the **decision engine**.
Telemetry events are consumed from Kafka as protobuf messages, aggregated into
time windows, and evaluated deterministically. Decisions are idempotent and
restart-safe using Redis. The control plane exposes a gRPC streaming API so
consumers can react to rollout decisions in real time. Policies are externalized
as YAML instead of hardcoded thresholds.

---

## Architecture Walkthrough (Step by Step)

1. **Telemetry Ingestion**
   - Services emit telemetry events
   - Events are published to Kafka (`telemetry.raw`)
   - Payloads are protobuf, not JSON

2. **Decision Engine**
   - Consumes telemetry in real time
   - Maintains sliding evaluation windows
   - Computes error rate and latency
   - Applies policy rules

3. **Policy-as-Code**
   - Thresholds and actions are defined in YAML
   - Operators can change rollout behavior without code changes
   - Separates business policy from execution logic

4. **State & Idempotency**
   - Redis stores rollout state
   - Prevents duplicate decisions across restarts
   - Makes the system safe under replays and crashes

5. **Decision Distribution**
   - Decisions are published to Kafka
   - Decisions are streamed live over gRPC
   - Enables real-time control-plane consumers

---

## Why gRPC Streaming?

> Polling APIs introduce latency and complexity. Streaming gives us immediate,
push-based delivery of decisions, which is critical for deployment safety.

---

## Why Redis?

> Redis provides fast state access and atomic operations, which makes it ideal
for idempotency and restart-safe control planes.

---

## Why Policy-as-Code?

> Hardcoded thresholds don’t scale operationally. Policy-as-code allows SREs and
platform teams to change rollout behavior without redeploying the system.

---

## Failure & Chaos Handling

- High error rate → ROLLBACK
- High latency → PAUSE
- Healthy metrics → PROMOTE
- Process restarts do not duplicate decisions

These behaviors are validated via unit tests and live chaos testing.

---

## What I Deliberately Did NOT Build

- UI dashboards
- Full CI/CD pipelines
- Custom Kubernetes operators
- ML-based anomaly detection

> The goal was to demonstrate **depth in control-plane design**, not to build
a large but shallow system.

---

## If Asked: “How would you extend this?”

- Custom Kubernetes controller (CRDs)
- Traffic weighting via service mesh
- SLO-based policies
- Multi-service, multi-tenant support

I stopped before these to keep the project focused and reviewable.

---

## Closing Statement

> This project shows how I think about distributed systems: correctness first,
clear ownership boundaries, operator-driven configuration, and real-time
feedback loops.

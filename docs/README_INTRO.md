# Real-Time Canary Control Plane

A production-grade **deployment decision and control plane** that evaluates
live telemetry streams and autonomously **promotes, pauses, or rolls back
canary deployments** in seconds.

This project demonstrates **senior-level backend and platform engineering**
through real-time data processing, deterministic decision-making,
stateful control planes, and operator-driven policy design.

Unlike CI/CD demos that focus on pipelines or UI dashboards, this project
intentionally focuses on the **control-plane brain** behind systems like
Argo Rollouts, Spinnaker, and internal CD platforms at large-scale companies.

### What this system does
- Ingests live telemetry via Kafka
- Evaluates rollout health using sliding time windows
- Applies rollout policy defined as **YAML (policy-as-code)**
- Persists rollout state with Redis (idempotent, restart-safe)
- Streams decisions in real time over gRPC

### What this project is (and is not)
- ✅ A real-time rollout decision engine
- ✅ A control plane, not a pipeline
- ❌ Not a UI-heavy demo
- ❌ Not a full CI/CD system

The goal of this project is to demonstrate **depth in distributed systems and
control-plane design**, not feature breadth.

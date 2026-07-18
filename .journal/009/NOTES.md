---
id: 009
title: Continue phase 6 service hardening
started: 2026-07-18
---

## 2026-07-18 11:43 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 6 service hardening.
Current state of the world: Phases 0 through 4 are complete on `master` at `56eaf85`; phase 5 has deterministic recovery coverage plus live hot-standby replacement and idle/busy restart evidence, while bounded concurrent demand and deliberately timed provisioning/terminal-cleanup restart proofs remain open. Session 001's primary roadmap and focused controller/image proposals define phase 6 around observed systemd, timeout, outage, signal, credential, ownership, logging, and diagnostic behavior.
Plan: Inspect the current runtime and deployment surfaces, choose the smallest phase 6 proof that exposes real behavior, implement and validate that slice, then refine the remaining hardening work from what it teaches us.

## 2026-07-18 11:56 — First phase 6 recovery slice
Inspection found that the initial GitHub message-session preflight was fail-fast as intended, but any listener/session failure after startup terminated the whole controller. Implemented capped exponential session recreation on `feat/phase-6-github-reconnect`: the initial session still opens synchronously, each failed post-start session is closed within a fresh bounded context, retry delay is configurable through `retry.initial` and `retry.maximum`, and healthy polling resets the delay. Deterministic adapter tests prove cap and reset behavior without external services; `moon run root:check --summary minimal` and `go test -race ./internal/adapters/github ./internal/app` pass. Commit `3f28047` is published as PR #16. Remaining phase 6 work includes systemd deployment assets, bounded escalation for components that ignore cancellation, signal behavior under the shipped unit, and targeted Incus/GitHub outage evidence.

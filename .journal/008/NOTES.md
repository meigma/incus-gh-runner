---
id: 008
title: Continue phase 5 hot pool recovery
started: 2026-07-18
---

## 2026-07-18 10:18 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 5, proving hot standby capacity, runner replacement, bounded concurrent demand, and restart reconciliation.
Current state of the world: Phases 0 through 4 are merged on `master` at `8357882`; one genuine GitHub job has run successfully on one JIT-configured Incus 7.2 VM and cleanup returned owned inventory to zero. Session 001's implementation plan and focused controller proposal remain the governing phase 5 references.
Plan: Start with the smallest phase 5 proof, use observed behavior to refine the implementation, and expand toward the full hot-pool and recovery exit evidence.

## 2026-07-18 10:44 — Deterministic hot-pool proof
Created `feat/phase-5-hot-pool` from exact `origin/master` commit `8357882`. Inspection showed that the existing reconciler already implements the phase 5 target formula and bounded operations, but its tests did not prove standby replacement or restart behavior across all lifecycle states.

Added behavior-first controller coverage for preserving a replacement while deleting a consumed runner and for restart reconciliation of provisioning, idle, busy, and terminal inventory. The focused test passed 20 consecutive runs, and `mise exec -- moon run root:check --summary minimal` passed after clearing stale golangci-lint cache entries from a removed worktree. Committed the slice as `a00f8cd` (`test(controller): prove hot pool recovery`).

Next: build the repeatable phase 5 live proof around a preconnected standby, job dispatch/replacement, bounded concurrent demand, and controlled restarts.

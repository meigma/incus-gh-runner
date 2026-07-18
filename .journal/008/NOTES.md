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

## 2026-07-18 11:20 — Live standby replacement proof
Merged PR #14 at `ed5688b`, adding the repeatable phase 5 workflow and live harness. Provisioned a temporary Latitude `c3-small-x86` host (`sv_Yx2za1e9naVrL`) with Incus 7.2, then ran the controller and runner image built from exact commit `ed5688b`.

The successful proof `phase5-20260718T181416Z-328771` demonstrated a preconnected idle standby, assignment of workflow run `29655469742` to `incus-gh-runner-56774368-a09e-4ae0-b5da-3d3ee2e08a0a`, creation of replacement standby `incus-gh-runner-830d206a-5323-48af-9c37-49a18ce55e0b` while the first runner was busy, successful assignment of cleanup run `29655491424` to the replacement, and preservation of active work across controller restarts. Final owned inventory was empty.

The first live harness assumption was wrong: GitHub scale-set runners were not visible through the repository self-hosted-runners endpoint, while organization-wide inventory would require broader authorization. Revised the proof to use exact owner-scoped Incus inventory and guest `Listening for Jobs` evidence for readiness, plus the repository workflow-jobs endpoint for assignment. This keeps the live credential least-privileged. The correction passed the full repository gate and merged as PR #15 at `56eaf85`.

Local evidence archive: `build/live-phase5-evidence/20260718-run-29655469742/incus-gh-runner-phase5-evidence.tar.gz` (`sha256:03a31018877c57c7cf4eecf940e5f7e9e037ce0754459adc5a1db82181e76c6f`). The credential was shredded, the paid host was destroyed, and a direct provider lookup confirmed the server no longer exists.

Next: extend the live proof only where it adds information beyond deterministic coverage: bounded concurrent demand and deliberately timed restarts during provisioning and terminal cleanup. Keep session 008 open until those remaining phase 5 exit conditions are either proven or explicitly deferred.

## 2026-07-18 11:40 — Close
Closed the session after PR #14 (`ed5688b`) and PR #15 (`56eaf85`) were squash-merged and local `master` was verified clean and synchronized with `origin/master`. The session partially met the full phase 5 goal: deterministic coverage proves replacement and restart reconstruction, while the disposable Incus 7.2 proof demonstrated a preconnected standby, genuine job assignment, replacement while busy, idle/busy controller restarts, and exact cleanup. Live bounded concurrent demand and deliberately timed provisioning/terminal-cleanup restarts remain explicit follow-up work in `SUMMARY.md`.

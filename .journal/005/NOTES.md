---
id: 005
title: Continue phase 3 Incus lifecycle
started: 2026-07-17
---

## 2026-07-17 20:08 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 3, integrating the real Incus lifecycle behind the existing controller core.
Current state of the world: Phases 0 through 2 are merged on `master` through PRs #7, #8, and #9 at `85f273a`; the typed controller core, reproducible reference VM, one-shot guest payload contract, hosted offline proof, and live Incus validator exist, while real runtime adapters remain unwired and live Incus boot validation still requires an Incus-capable host.
Plan: Start with the smallest phase 3 proof: map the existing controller port to ownership-scoped Incus inventory and lifecycle operations, preserve bounded contexts and idempotent reconciliation, and prove behavior with focused functional evidence before expanding recovery cases.

## 2026-07-17 20:27 — First Incus lifecycle slice
Created `feat/phase-3-incus-lifecycle` from the fetched `origin/master` and implemented the first real Incus backend at commit `046d564`. The adapter preflights an existing image and profiles, creates a VM with exact ownership metadata, starts it, retries agent transfer, writes the versioned payload before its ready-marker commit point, observes guest/Incus state, collects serial-console diagnostics, refuses to delete unowned instances, and deletes owned instances idempotently.
The controller now refreshes owned inventory through the bounded worker pool, serializing refresh against lifecycle mutations so provisioning runners can be observed as running or terminal after restart without racing stale snapshots over completed creates.
Evidence: focused adapter/controller tests pass, the repository Go test task passes, and the repository lint task passes. An opt-in `TestIncusLifecycleFunctional` composes fake demand with the real adapter in a uniquely owned, non-default Incus project and requires the inventory to return to zero. It could not run on the development Mac because no Incus CLI, socket, or functional-test environment is available.
Next: review the slice for partial-operation and terminal-state gaps, run the aggregate repository gate, then publish the branch for hosted review.

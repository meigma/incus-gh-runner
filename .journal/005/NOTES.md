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

## 2026-07-17 20:30 — Phase 3 review boundary
Review tightened payload delivery so the ready marker is retried independently after the payload is durably written, and added explicit restart evidence that existing provisioning capacity prevents duplicate creation. The final branch head is `c5cabd1`.
Validation passed with `moon run root:check --summary minimal` and `go test -race ./... -count=1`. PR #10 (`feat(incus): implement owned runner lifecycle`) is open, ready, cleanly mergeable, and exact-head GitHub CI, Pages, and Kusari Inspector all passed. The live Incus functional gate remains unexecuted because this Mac has no Incus environment; the opt-in test and safety boundary are included in the PR.
Current boundary: pause for human review of PR #10. Do not merge or close session 005 without explicit approval.

## 2026-07-17 20:42 — Close
PR #10 was approved and squash-merged to `master` as `d03cace7bbde85c7365c13fda541c87243daddfc`. Local `master` was fast-forwarded to the same commit, and the implementation Worktrunk plus local and remote `feat/phase-3-incus-lifecycle` branches were removed.
The merged exact head passed CI, GitHub Pages, and Kusari Inspector; local `moon run root:check --summary minimal` and `go test -race ./... -count=1` also passed. The Incus-capable live test remains unexecuted and is the explicit follow-up before claiming real-host lifecycle evidence.
Handoff: phase 4 can build on the merged ownership-scoped Incus backend and periodic inventory to add real scale-set demand, fresh JIT payloads, executable composition, and one genuine GitHub Actions job.

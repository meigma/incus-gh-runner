---
id: 005
title: Continue phase 3 Incus lifecycle
date: 2026-07-17
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 002, 003, 004]
---

## Goal

Continue the v1 implementation plan with phase 3: keep GitHub demand fake while replacing the fake runner backend with an ownership-scoped real Incus lifecycle and restart-aware inventory.

## Outcome

The goal was met for the planned implementation and automated evidence slice. PR #10 was squash-merged to `master` as `d03cace` after the exact head `c5cabd1` passed CI, GitHub Pages, and Kusari Inspector. The controller can now observe real lifecycle transitions through periodic inventory, and the Incus adapter preflights existing references, creates and starts owned VMs, commits the guest payload, inspects status, collects diagnostics, and deletes only exact-marker instances.

The opt-in live lifecycle test was not run because the development Mac has no Incus CLI, socket, or configured disposable project. It remains the Incus-capable acceptance gate for the merged slice.

## Key Decisions

- Refresh inventory through the bounded worker pool and serialize refresh against lifecycle mutations -> a stale snapshot cannot overwrite a completed create, while demand callbacks remain free of Incus I/O.
- Require exact `user.incus-gh-runner.owner` metadata both when listing and immediately before deletion -> names and project membership alone never authorize mutation.
- Write `payload.json` completely before independently retrying the `payload.ready` commit marker -> the guest never observes a partial JIT payload.
- Treat running guest work as busy, stopped/error/failed guests as terminal, and over-age unready instances as terminal -> scale-down preserves active work while failed bootstrap eventually cleans up.
- Keep the live test opt-in, uniquely owned, and forbidden from using the default project -> destructive evidence remains narrowly scoped to disposable infrastructure.

## Changes

- `internal/adapters/incus/backend.go` - added preflight, durable ownership metadata, VM create/start, agent payload transfer, guest-state mapping, bootstrap expiry, diagnostic collection, and ownership-checked idempotent deletion.
- `internal/adapters/incus/client.go` - wrapped the Incus SDK in the narrow context-aware operations required by the backend.
- `internal/controller/controller.go` - added periodic inventory operations and reconciler-owned snapshot replacement without concurrent lifecycle races.
- `internal/adapters/incus/*_test.go` and `internal/controller/controller_test.go` - proved ownership filtering, payload ordering, terminal cleanup, restart capacity safety, and the opt-in real Incus lifecycle.
- `README.md` and `docs/docs/index.md` - documented the phase 3 boundary and disposable live-test interface.

## Open Threads

- Run `TestIncusLifecycleFunctional` on an Incus-capable host with the phase 2 image imported into an explicitly disposable non-default project.
- Phase 4 must add real GitHub scale-set demand and fresh JIT payload generation, then compose the existing Incus backend into the executable for one genuine job.
- The merged reference image still needs its separate live import/boot validation on an Incus-capable host.

## Lessons

- `gh pr merge --delete-branch` can report a local checkout conflict after GitHub has already completed the squash merge; verify remote PR state before retrying, then fast-forward `master` and clean up the Worktrunk and remote branch separately.

## References

- [PR #10: feat(incus): implement owned runner lifecycle](https://github.com/meigma/incus-gh-runner/pull/10)
- [Exact-head CI run 29628911693](https://github.com/meigma/incus-gh-runner/actions/runs/29628911693)
- `master` squash commit `d03cace7bbde85c7365c13fda541c87243daddfc`
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`
- `.journal/001/CONTROLLER_PROPOSAL.md`
- `.journal/004/SUMMARY.md`

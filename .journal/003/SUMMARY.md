---
id: 003
title: Continue phase 1 controller core
date: 2026-07-17
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 002]
---

## Goal

Continue the v1 implementation plan from sessions 001 and 002 by completing
phase 1: prove the controller's orchestration shape with fake demand and runner
ports before adding real GitHub or Incus lifecycle complexity.

## Outcome

The goal was met. PR [#8](https://github.com/meigma/incus-gh-runner/pull/8)
was squash-merged to `master` as `9bd37f7` after the exact final head
`544c78d` passed CI, GitHub Pages, and Kusari Inspector. The local `master`
checkout was fast-forwarded, and the implementation worktree plus local and
remote feature branches were removed.

Phase 1 now has functional evidence that fake scale-set demand converges while
slow backend operations remain bounded and do not block newer demand or clean
cancellation. Real GitHub and Incus runtime adapters remain intentionally
unwired until their later evidence-producing phases.

## Key Decisions

- Keep the scale-set-facing path as a one-slot latest-demand mailbox so synchronous callbacks can publish without waiting for runner operations.
- Make the reconciler the single owner of desired, observed, deleting, and in-flight capacity so duplicate work is prevented without shared mutable orchestration state.
- Execute create and delete operations through a fixed worker pool, retry failed operations on periodic reconciliation, and preserve busy runners during scale-down.
- Allow in-flight operations a bounded shutdown grace period, then cancel them; return a fatal timeout if a backend still ignores cancellation.
- Load configuration once into validated immutable types with precedence `flags > environment > YAML > defaults`; Viper remains at the CLI boundary.
- Prove the complete application path with deterministic fake ports without shipping a fake production backend; the executable reports unavailable runtime adapters until real integrations land.
- Document every Go type, function, and method with concise identifier-led Godoc, including private declarations and test helpers, because IntelliSense is part of the review workflow.

## Changes

- `internal/controller/` - added the demand mailbox, owned runner model, single-owner reconciliation state, idempotent operation results, bounded workers, retry, scale-down, logging, and shutdown behavior.
- `internal/app/` - added supervision for the demand source and controller under one cancelable application context.
- `internal/config/` - replaced the template message setting with typed capacity, concurrency, reconciliation, and timeout configuration plus validation and explicit environment bindings.
- `internal/cli/root.go` - added configuration-file, environment, and flag precedence and passed the Cobra execution context into an injected application runner.
- `internal/controller/*_test.go`, `internal/app/app_test.go`, `internal/config/config_test.go`, `internal/cli/root_test.go` - added deterministic behavior tests for convergence, concurrency bounds, retries, busy-runner preservation, cancellation, component failure, invalid configuration, and source precedence.
- `README.md`, `docs/docs/index.md` - documented the phase 1 boundary and current configuration surface.
- `cmd/incus-gh-runner/main.go` and phase 1 internals - added the project's private-declaration Godoc convention.

## Open Threads

- Phase 2 must prove the guest and reference-image contract: disposable runtime payload injection, readiness, diagnostics, secret cleanup, and terminal poweroff.
- Phase 3 must replace the fake runner backend with the ownership-scoped Incus lifecycle and restart inventory.
- Phase 4 must add the real scale-set demand/JIT adapter and run one genuine GitHub Actions job.
- The executable intentionally remains non-operational without injected runtime adapters; do not mistake phase 1's fake proof for a deployable service.

## Lessons

- A coalescing latest-value mailbox plus single-owner reconciliation kept demand ingestion responsive even while all backend workers were blocked.
- The hosted linter required `slog.DiscardHandler` even though the initial local gate accepted a discard text handler; pin hosted conclusions to the exact PR head before merge.

## References

- [PR #8: feat(controller): prove phase 1 controller core](https://github.com/meigma/incus-gh-runner/pull/8)
- `master` squash commit `9bd37f7`
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`
- `.journal/001/CONTROLLER_PROPOSAL.md`
- `.journal/001/IMAGE_PROPOSAL.md`
- `.journal/002/SUMMARY.md`

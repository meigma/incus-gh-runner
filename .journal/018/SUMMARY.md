---
id: 018
title: Implement job machine proof phase 3
date: 2026-07-20
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [014, 015, 017]
---

## Goal

Review Session 014's job machine-proof design and implementation plan, then
implement Phase 3's authenticated GitHub job/JIT correlation and supervised
signing coordinator without broadening into the live proof or TPM phases.

## Outcome

The goal was met. Phase 3 was implemented, verified, reviewed, and
squash-merged through PR #38 as `c32e134`. The controller now binds each JIT
runner registration to the exact owned Incus VM before boot, queues
authenticated job-start events without blocking the GitHub listener,
reconstructs and verifies the machine snapshot at proof time, and signs and
delivers the version 1 proof through the Phase 2 immutable guest channel.

The final review fix keeps disabled proof wiring genuinely nil rather than
placing a nil queue pointer in a non-nil interface. Local repository checks,
race-enabled focused tests, hosted CI, CodeQL, GitHub Pages, and Kusari all
passed on the reviewed head `9079942`. Phase 4's genuine GitHub/Incus proof was
intentionally not attempted.

## Key Decisions

- Persist the validated JIT runner ID, name, and scale-set ID while the VM is
  stopped, using its ETag, then re-read the same UUID and metadata before boot
  -> stale writes and same-name replacement fail before guest execution.
- Save the instance UUID and launch digest at allocation, then reconstruct and
  compare the launch digest from the exact running VM at proof time -> proofs
  cannot silently describe drifted controller inputs or a replacement VM.
- Keep the synchronous GitHub callback non-blocking with a bounded queue sized
  to `max(1, capacity.max_runners)` -> Incus I/O and signing remain outside the
  listener while overload drops proofs explicitly rather than stalling demand.
- Supervise one coordinator and isolate failures per event -> one invalid job
  or delivery failure does not stop controller reconciliation.
- Fence the GitHub registration around provisioning failure and before owned
  Incus deletion -> a registration cannot outlive a failed or deleted runner
  without an explicit fencing error.
- Assign `JobStartedSink` only when the queue exists -> disabled proof mode
  avoids Go's typed-nil interface trap and retains the pre-proof callback path.
- Stop at the Phase 3 review gate -> live end-to-end evidence and TPM-bound key
  storage remain independent, reviewable phases.

## Changes

- `internal/adapters/github/scaleset.go` - validated JIT identity, registration
  fencing, authenticated job-start projection, and non-blocking proof enqueue.
- `internal/adapters/incus/backend.go` - reserved metadata protection,
  stopped-VM JIT persistence, ETag/read-back fencing, launch digest storage,
  deletion fencing, and exact proof-time machine snapshots.
- `internal/provenance/coordinator.go` - bounded job-start queue and supervised
  snapshot/sign/deliver coordinator.
- `internal/app/app.go` - supervised auxiliary application components with
  bounded shutdown behavior.
- `internal/runtime/runtime.go` - optional proof queue/coordinator composition
  and disabled-mode-safe callback wiring.
- `internal/**/*_test.go` - adversarial JIT, ETag, replacement, fencing,
  snapshot, queue, supervision, and disabled-proof callback coverage.
- `docs/docs/reference/configuration.md` - queue saturation, per-event failure,
  and acknowledged-event crash behavior.

## Open Threads

- Phase 4 must run a genuine GitHub Actions job on an Incus runner, retrieve the
  delivered proof in the workflow, verify it externally, and archive bounded
  evidence.
- Phase 5 must validate the same PKCS#8 Ed25519 credential through
  systemd-250+ TPM-bound storage; this remains storage binding rather than
  TPM-native signing or measured boot.
- The upstream listener acknowledges messages before the callback. A controller
  crash in that gap can lose a proof event; proof-required jobs continue to
  fail through the guest helper timeout rather than receiving an unbound proof.

## Lessons

- In Go, assigning a nil concrete pointer to an interface produces a non-nil
  interface; optional adapter wiring needs an explicit concrete nil check
  before interface assignment.
- Holding the VM stopped through conditional metadata persistence and exact
  read-back gives the JIT binding a small, testable fail-closed boundary before
  any untrusted guest workload runs.

## References

- PR #38: https://github.com/meigma/incus-gh-runner/pull/38
- Merge commit: `c32e134a3cbba57e3aaea1add095fec356d8bb13`
- Reviewed head: `907994237cdfbf9d74acc3b344868196565bda8b`
- Hosted CI run: https://github.com/meigma/incus-gh-runner/actions/runs/29790841627
- Hosted CodeQL run: https://github.com/meigma/incus-gh-runner/actions/runs/29790839655
- Design and plan: `.journal/014/JOB_MACHINE_PROOF_DESIGN.md` and
  `.journal/014/JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md`
- Phase 1: `.journal/015/SUMMARY.md`
- Phase 2: `.journal/017/SUMMARY.md`

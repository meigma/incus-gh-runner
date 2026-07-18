---
id: 009
title: Continue phase 6 service hardening
date: 2026-07-18
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 006, 008]
---

## Goal

Complete phase 6 of the v1 implementation plan by hardening the controller's
systemd deployment, timeouts, outage recovery, signal handling, credentials,
ownership boundary, logs, and diagnostics from observed failure behavior.

## Outcome

The goal was met. PRs #16 through #19 added resilient GitHub message-session
reconnection, bounded application shutdown escalation, a hardened production
systemd deployment, and capped Incus operation-failure cooldown. Exact-head
hosted checks passed for every slice and `master` finished clean and synchronized
at `4979f7d`.

A disposable Latitude server running Incus 7.0.1 proved the shipped service with
real GitHub jobs. SIGTERM completed in 1.61 seconds without terminating active
work; restart reconstructed capacity and cleanup. Scoped GitHub and Incus
outages recovered without process restart, failed create/boot/delete paths
recovered after repair, and an unowned sentinel survived every controller
operation. Credentials and temporary resources were removed, evidence was
exported, and the exact paid server was destroyed and verified absent.

## Key Decisions

- Keep startup fail-fast but recreate post-start GitHub sessions -> invalid configuration and credentials remain immediately actionable while transient outages recover with capped backoff.
- Escalate cancellation-ignoring components to process failure -> Go cannot forcibly stop a wedged goroutine, so systemd owns the final recovery boundary.
- Use a dynamic systemd user plus `incus-admin` -> local Incus socket access is root-equivalent, while dynamic identity and filesystem restrictions still minimize unrelated host access.
- Inject credentials through systemd credentials in production -> GitHub App private-key material stays out of arguments, ordinary configuration, and logs.
- Reuse `retry.initial` and `retry.maximum` for Incus failures -> one bounded policy fixed the observed retry storm without introducing speculative knobs.
- Isolate delete cooldown by exact runner and require fresh inventory after uncertain results -> one protected runner does not block unrelated cleanup, while timed-out operations cannot immediately trigger stale-state mutations.
- Use a disposable Latitude host for final evidence -> direct Incus and systemd behavior could be observed without leaving paid infrastructure or test resources behind.

## Changes

- `internal/adapters/github/reconnect.go` - recreates failed message sessions with capped exponential backoff and bounded cleanup.
- `internal/app/app.go` - bounds shutdown across both long-lived components and returns a fatal error when a component ignores cancellation.
- `deploy/systemd/` - supplies the hardened `Type=simple` unit, production configuration example, verification script, and deployment guidance.
- `internal/controller/controller.go` - applies per-target Incus failure cooldown and pauses mutation until stale inventory refreshes.
- `internal/controller/*_test.go` and `internal/app/app_test.go` - prove reconnect, shutdown escalation, retry suppression, per-runner delete isolation, and inventory-outage recovery.
- `README.md` and deployment documentation - describe transient recovery, signal behavior, credentials, privileges, and operational verification.

## Open Threads

- Phase 7 still owns release-ready controller packages, reference-image publication, operator documentation, and consolidated v1 acceptance evidence.
- Session 008's optional live phase 5 gates remain: bounded concurrent demand and deliberately timed restarts during provisioning and terminal cleanup.

## Lessons

- Real hardware exposed behavior that offline checks did not: failed create and protected-delete operations retried every reconciliation tick until bounded cooldown was added.
- An artificially short Incus client deadline can expire while an asynchronous create continues, producing partial state; keep the production five-minute operation budget and refresh inventory before retrying mutation.
- A test PAT conflicts with the production unit's GitHub App credential variable unless the test-only drop-in explicitly clears that variable; the shipped production credential boundary remains correct.
- Exact server IDs, owner markers, and exported evidence checksums make destructive hardware cleanup auditable and deterministic.

## References

- [PR #16: fix(github): reconnect failed message sessions](https://github.com/meigma/incus-gh-runner/pull/16)
- [PR #17: fix(app): bound component shutdown](https://github.com/meigma/incus-gh-runner/pull/17)
- [PR #18: feat(systemd): add hardened service deployment](https://github.com/meigma/incus-gh-runner/pull/18)
- [PR #19: fix(controller): back off failed Incus operations](https://github.com/meigma/incus-gh-runner/pull/19)
- [Active-job SIGTERM workflow run 29658672280](https://github.com/meigma/incus-gh-runner/actions/runs/29658672280)
- `build/live-phase6-evidence/20260718-sv_y9815XYwxavEk/incus-gh-runner-phase6-evidence-v2.tar.gz`
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`
- `.journal/008/SUMMARY.md`

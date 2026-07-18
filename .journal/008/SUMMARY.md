---
id: 008
title: Continue phase 5 hot pool recovery
date: 2026-07-18
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 002, 003, 004, 005, 006]
---

## Goal

Continue phase 5 by proving hot standby capacity, replacement after consumption,
bounded concurrent demand, and restart reconciliation across the runner lifecycle.

## Outcome

The goal was partially met. PR #14 landed deterministic controller coverage and
a repeatable live proof harness, and PR #15 corrected the harness to collect
least-privileged, owner-scoped evidence. A temporary Incus 7.2 host proved that
a JIT-connected idle runner accepted a genuine GitHub Actions job, a replacement
became ready while it was busy, active work survived controller restarts, the
replacement accepted the cleanup job, and final owned inventory returned to
zero.

The reconciler tests now cover replacement behavior and restart reconstruction
of provisioning, idle, busy, and terminal inventory. Live bounded concurrent
demand and deliberately timed restarts during provisioning and terminal cleanup
were not run and remain future phase 5 work.

## Key Decisions

- Prove the existing target-capacity behavior before changing controller code -> inspection showed the reconciler already implemented the phase 5 capacity formula and bounded operations; the missing artifact was behavioral evidence.
- Use exact owner-scoped Incus inventory and guest readiness evidence -> scale-set runners were not visible through the repository runner endpoint, and organization-wide runner inventory would have required broader authorization.
- Correlate assignment through the repository workflow-jobs endpoint -> this proves which VM accepted a job without granting organization administration scope.
- Keep the hardware window disposable -> build inputs were checksummed, evidence was exported locally, the credential was shredded, and the exact paid server was destroyed and verified absent.

## Changes

- `internal/controller/controller_test.go` - proves consumed-standby replacement and restart reconstruction across provisioning, idle, busy, and terminal states.
- `.github/workflows/runner-functional.yml` - accepts proof correlation and hold-time inputs for repeatable live jobs.
- `scripts/live/phase5-hot-standby.sh` - automates standby readiness, job assignment, replacement, restart, cleanup, and evidence capture using scoped signals.
- `README.md` - documents the phase 5 live proof boundary and invocation.

## Open Threads

- Run bounded concurrent GitHub demand against the hot pool and preserve exact assignment and capacity evidence.
- Deliberately restart the controller while a VM is provisioning and while a terminal VM is being cleaned up; confirm no duplicate capacity and exact deletion.
- Continue to phase 6 only after deciding whether those remaining live phase 5 gates are required or whether deterministic coverage is sufficient.

## Lessons

- Repository self-hosted-runner inventory is not a reliable observation surface for scale-set runners; combine exact Incus ownership, guest readiness, and repository workflow-job assignment instead.
- A minimal Incus host installation needs `nftables`, and a partially initialized default profile may need an explicit root disk before disposable VM proofs can run.
- Avoid long-running command substitutions in shell proof harnesses because they isolate trap state and can delay cleanup; store observed IDs in the parent shell instead.

## References

- [PR #14: test: add phase 5 hot pool recovery proof](https://github.com/meigma/incus-gh-runner/pull/14)
- [PR #15: fix(live): use scoped runner evidence](https://github.com/meigma/incus-gh-runner/pull/15)
- [Primary live workflow run 29655469742](https://github.com/meigma/incus-gh-runner/actions/runs/29655469742)
- [Replacement cleanup workflow run 29655491424](https://github.com/meigma/incus-gh-runner/actions/runs/29655491424)
- `build/live-phase5-evidence/20260718-run-29655469742/incus-gh-runner-phase5-evidence.tar.gz`
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`
- `.journal/006/SUMMARY.md`

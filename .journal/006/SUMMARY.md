---
id: 006
title: Continue phase 4 GitHub lifecycle
date: 2026-07-18
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 002, 003, 004, 005]
---

## Goal

Complete phase 4 of the v1 plan by wiring real GitHub scale-set demand and fresh
JIT configuration into the ownership-scoped Incus lifecycle, then prove that
one genuinely queued job runs on exactly one disposable VM and leaves no owned
capacity behind.

## Outcome

The goal was met. PRs #11, #12, and #13 landed the GitHub scale-set adapter and
runtime composition, corrected the Noble reference image and live diagnostic
contract, fixed hardware-discovered Incus integration gaps, and established
Incus 7.0 as the minimum supported server. GitHub Actions run `29652301896`
completed successfully on one JIT-configured Incus VM; controller evidence
showed demand returning from one to zero, exact runner deletion, and an empty
owned inventory.

The final hardware proof ran on Incus 7.2 using the checksummed reference image.
It covered image boot, Incus agent readiness, root-only payload consumption,
unprivileged runner execution, secret-free serial diagnostics, poweroff, and
cleanup. The protected evidence is stored under
`build/live-phase4-evidence/20260718-run-29652301896/`. Both paid Latitude hosts
used during investigation were destroyed and verified absent. `master` is clean
and synchronized at `8357882`.

## Key Decisions

- Keep GitHub scale-set callbacks free of Incus I/O -> callbacks publish current demand into the existing coalescing mailbox while bounded controller workers own VM operations.
- Generate one fresh JIT configuration per VM -> each ephemeral runner accepts exactly one job and requires no persistent runner-registration state.
- Support environment-only development credentials alongside GitHub App configuration -> local functional proofs can reuse authenticated operator credentials without placing secrets in configuration files or logs.
- Keep runner output in the guest journal and write only lifecycle metadata to `ttyS0` -> the controller can retain useful serial evidence without copying arbitrary workload output into its protected diagnostics store.
- Capture the live console during a bounded 30-second guest grace window -> Incus did not reliably preserve the VM ring buffer after guest-driven QEMU poweroff.
- Resolve image aliases explicitly and stop owned terminal VMs before deletion -> the Incus SDK treats aliases separately from fingerprints, and guest terminal status can precede physical VM shutdown.
- Require Incus 7.0 or newer -> the accepted hardware path uses Incus 7.2, and carrying an Incus 6 compatibility promise provides no project value.
- Consolidate live gates into short Latitude hardware windows -> slow provisioning was amortized across image, lifecycle, and genuine-job proofs, with evidence exported before exact-ID teardown.

## Changes

- `internal/adapters/github/` - added persistent scale-set resolution, message polling, non-blocking demand publication, fresh JIT generation, and real GitHub preflight coverage.
- `internal/runtime/runtime.go`, `internal/config/`, and `cmd/incus-gh-runner/` - composed the real GitHub and Incus adapters with validated PAT/GitHub App credential interfaces.
- `internal/adapters/incus/` - added protected directory diagnostics, alias resolution, live functional logging, and stop-before-delete behavior for terminal VMs.
- `image/image.yaml` and `image/guest/` - installed the working signed UEFI boot path and implemented serial-only lifecycle metadata with a bounded diagnostic grace.
- `image/validate-incus.sh` and `image/tests/guest-entrypoint-test.sh` - proved the live guest contract, exact cleanup, and the Incus 7.0 minimum.
- `.github/workflows/runner-functional.yml` - added the manual exact-label one-shot job used for real acceptance.
- `scripts/live/` - added the local bundle and host-preparation path used to rehearse the paid hardware window.
- `README.md` and `docs/docs/reference-image.md` - documented functional-test safety, credentials, diagnostics, and the Incus 7 support policy.

## Open Threads

- Continue with phase 5: prove hot standby, replacement after consumption, bounded concurrent demand, and restart reconciliation during provisioning, idle, busy, and cleanup states.
- Phase 6 still owns the production systemd unit, protected deployment paths, and failure/restart hardening; the temporary hardware proof unit was intentionally removed.
- Phase 7 still owns packaged release and installation artifacts after the hot-standby and service-hardening evidence exists.

## Lessons

- The real hardware gate found behavior that unit and hosted offline tests could not: Noble's removable GRUB path did not boot, a powered-off VM lost serial history, aliases were not fingerprints to the SDK, and a terminal guest could still be a running Incus VM.
- Publish diagnostic lifecycle metadata before a bounded live-capture window; do not assume a stopped VM retains the same console evidence that was visible while running.
- Validate against the supported Incus major early. Starting from Ubuntu's native Incus 6 package created avoidable compatibility investigation before the host was upgraded to Incus 7.2.
- Preserve the exact paid-server ID independently of hostname and IP so evidence export and teardown remain deterministic across pauses.

## References

- [PR #11: feat(github): integrate runner scale-set lifecycle](https://github.com/meigma/incus-gh-runner/pull/11)
- [PR #12: fix(incus): boot and clean up Noble runners](https://github.com/meigma/incus-gh-runner/pull/12)
- [PR #13: chore(incus): require version 7 or newer](https://github.com/meigma/incus-gh-runner/pull/13)
- [Genuine one-shot runner proof](https://github.com/meigma/incus-gh-runner/actions/runs/29652301896)
- [Final exact-head reference-image proof](https://github.com/meigma/incus-gh-runner/actions/runs/29653011279)
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`
- `.journal/005/SUMMARY.md`

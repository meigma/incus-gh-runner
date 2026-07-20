---
id: 017
title: Implement job machine proof phase 2
started: 2026-07-20
---

## 2026-07-20 15:53 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and plan, then begin Phase 2 implementation.
Current state of the world: Phase 1 is merged on `master` at `ce7c89c`, providing the proof format, signer, verifier, optional configuration, and shared profile digest; GitHub job correlation and host-to-VM proof delivery are not implemented.
Plan: Review the Phase 2 gate and current controller seams, create an isolated Worktrunk implementation branch from fetched `master`, implement the smallest testable Phase 2 slice, and pause at the plan's review gate.

## 2026-07-20 16:04 — Phase 2 thin slice checkpoint
Reviewed Session 014's design and five-phase plan in full and kept this branch inside the Phase 2 boundary: no GitHub event correlation, JIT metadata, coordinator, or TPM work entered the slice.

Created `feat/job-proof-phase-2` from fetched `master` at `ce7c89c` and committed `c1f730f` (`feat(provenance): deliver proofs to runner guests`). The checkpoint adds the separate `0755` proof directory, unprivileged wait/copy helper, verified and ownership-fenced Incus proof sink, immutable marker protocol, deterministic unit/contract coverage, guest-contract documentation, and a skipped-by-default real Incus functional harness.

`mise exec -- moon run root:check` passes all 11 tasks. The live Phase 2 gate is not claimed: `INCUS_GH_RUNNER_TEST_PROJECT` and `INCUS_GH_RUNNER_TEST_IMAGE` are unset, and the local Incus 7.2 client reports its server unreachable. Next: review the thin slice, run the functional harness against a freshly built reference image on a disposable Incus 7+ project, record delivery time and permissions, then decide whether to keep or revise the fixed path and marker protocol.

## 2026-07-20 16:34 — Live Phase 2 proof passed
Pushed `feat/job-proof-phase-2` at exact commit `c1f730ff074c0e5488ede4a0989f68b4484fa712` and ran hosted Reference Image workflow `29786457401`; it completed successfully on that SHA. Downloaded and verified the resulting image archive at SHA-256 `f966012a93d6d8dcf9b3d892a34b798c40f6c39d566648203fb9567f16922d5c`.

Provisioned temporary Latitude server `sv_x3egaQZVQ046Q` (`c3.small.x86`, MEX2, Ubuntu 24.04), installed Incus 7.2, and created disposable project `runner-proof-phase2`. The existing image validator passed the reference-image guest contract. The Phase 2 functional harness then launched a fresh VM, delivered the signed envelope through the Incus guest agent, retrieved the exact bytes as the unprivileged `actions-runner` user, and verified the fixed paths, ownership, permissions, immutable readiness marker, and successful cleanup. Measured proof delivery was `78.13291ms`; the complete functional run passed in `32.49s`.

After the test, the disposable project contained zero instances. Deleted the imported image and confirmed zero instances and zero images, then destroyed the Latitude server. A direct lookup returned 404 and the project server list returned no results. Evidence is retained locally under `build/live-phase2-evidence/20260720T231117Z` in the implementation worktree; no GitHub credential or other secret was transferred to the host.

Review-gate recommendation: keep the fixed path, immutable marker, and wait/copy helper protocol as implemented. The live run validated the security boundary and behavior without exposing a code defect. The two rehearsal failures were host setup/wrapper issues (open SSH stdin and a missing default-profile root disk), not Phase 2 implementation failures. Do not begin Phase 3 until this checkpoint is reviewed.

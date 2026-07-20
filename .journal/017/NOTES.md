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

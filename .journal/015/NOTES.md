---
id: 015
title: Implement job machine proof phase 1
started: 2026-07-20
---

## 2026-07-20 15:09 — Kickoff
Goal for the session: Review Session 014's job-bound machine-proof design and implementation plan, then begin phase 1.
Current state of the world: Session 014 completed and reviewed the design and five-phase plan; implementation has not started, and phase 1 is the approved first review-gated slice.
Plan: Read the two handoff documents, create an isolated implementation worktree from the fetched default branch, implement the smallest phase 1 proof primitives and tests, verify the phase gate, and pause for review.

## 2026-07-20 15:29 — Phase 1 review gate
Implemented the complete first phase on `feat/job-machine-proof-phase-1` at `d87d125`: strict version 1 payload and launch/profile digest primitives, local SPKI-derived Ed25519 key IDs, one-signature DSSE signing and verification, bounded PKCS#8/SPKI filesystem adapters, disabled-by-default paired configuration, startup key loading, the configuration-independent `proof verify` command, and operator key enrollment/rotation documentation. Reused the provenance profile digest implementation from the Incus adapter so the golden vector protects the live existing format.

Verification passed: the three planned module pins remain current; adversarial tests cover payload/signature tampering, wrong key and host, payload type, extra signatures, envelope/payload bounds, invalid IDs, unknown fields, malformed/multiple/non-Ed25519 keys, and exact golden vectors; `mise exec -- moon run root:check` and `mise exec -- go mod verify` passed locally. The existing systemd verifier skipped only its Linux-specific check on macOS. GitHub Advisory Database queries returned no entries for the new direct module or its newly introduced test dependency.

Draft PR #36 (`https://github.com/meigma/incus-gh-runner/pull/36`) is clean and fully green on exact head `d87d125`: CI, both Go CodeQL analyses, Actions CodeQL, Pages, CodeQL aggregation, and Kusari passed; release-only jobs skipped as designed. Paused at the Phase 1 human review gate. Phase 2 has not started, the PR remains draft, and no merge was attempted.

## 2026-07-20 15:42 — Close
Maintainer approval was received for the exact reviewed head. PR #36 was promoted from draft and squash-merged as `ce7c89c920ac16cf0422bb8e554498b0549524cf`; local `master` was fast-forwarded to that commit, and the session-owned feature worktree plus local and remote branches were removed. Session 015 closes with Phase 1 complete and Phase 2 not started.

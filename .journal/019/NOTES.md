---
id: 019
title: Implement job machine proof phase 4
started: 2026-07-20
---

## 2026-07-20 17:57 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and implementation plan, then begin Phase 4.
Current state of the world: Phases 1 through 3 are merged on `master` at `c32e134`; the repository can correlate an authenticated GitHub job with an exact JIT runner and owned Incus VM, sign its reconstructed machine snapshot, and deliver the proof through the immutable guest channel. Phase 4's genuine GitHub/Incus consumption proof and bounded evidence remain open, while TPM-bound credential validation remains deferred to Phase 5.
Plan: Re-read Session 014's Phase 4 acceptance contract, inspect the merged implementation and live harnesses, then implement the smallest proof-driven Phase 4 slice and validate it before expanding.

## 2026-07-20 18:07 — Phase 4 enabling slice ready
Reviewed Session 014's complete design and implementation plan against merged `master` at `c32e134`. The design remains internally consistent: Phase 4 is an operational proof gate around the merged claim, not authorization to add artifact provenance, TPM behavior, persistence, or a guest request API. The first incomplete dependency was a default-branch workflow surface and deployable file-backed systemd credential.

Created `feat/job-proof-phase-4-live` from `origin/master` and committed `94b7240` (`feat(provenance): add file-backed live proof gate`). The slice adds a proof-key credential drop-in that composes with both GitHub credential choices, makes the existing disposable runner workflow optionally retrieve the proof before any other work and preserve it as a one-day artifact, and documents file ownership, enrollment, and installation.

Local `actionlint` and the complete `moon run root:check` gate passed after clearing stale `golangci-lint` cache entries left by the removed Phase 3 worktree. PR #39 is open at exact head `94b7240273a1bf642de8adf2640f2aa231e5009a`; CI, CodeQL for Go and Actions, GitHub Pages, and Kusari all passed. The live gate is intentionally still open: merge/review approval, two genuine jobs (hot standby and demand-created), external payload comparison, receipt timing, tamper and wrong-enrollment rejection, controlled delivery failure, secret review, and final zero owned inventory remain next.

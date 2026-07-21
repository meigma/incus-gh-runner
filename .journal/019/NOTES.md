---
id: 019
title: Implement job machine proof phase 4
started: 2026-07-20
---

## 2026-07-20 17:57 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and implementation plan, then begin Phase 4.
Current state of the world: Phases 1 through 3 are merged on `master` at `c32e134`; the repository can correlate an authenticated GitHub job with an exact JIT runner and owned Incus VM, sign its reconstructed machine snapshot, and deliver the proof through the immutable guest channel. Phase 4's genuine GitHub/Incus consumption proof and bounded evidence remain open, while TPM-bound credential validation remains deferred to Phase 5.
Plan: Re-read Session 014's Phase 4 acceptance contract, inspect the merged implementation and live harnesses, then implement the smallest proof-driven Phase 4 slice and validate it before expanding.

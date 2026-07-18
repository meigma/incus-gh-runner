---
id: 006
title: Continue phase 4 GitHub lifecycle
started: 2026-07-17
---

## 2026-07-17 20:52 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 4, integrating the real GitHub scale-set lifecycle and proving one genuine queued job on one JIT-configured Incus runner VM.
Current state of the world: Phases 0 through 3 are merged on `master` at `d03cace`; the controller core, reference image and guest contract, and ownership-scoped Incus lifecycle are implemented, while GitHub demand and JIT composition remain unwired and the Incus-capable live gates have not yet been run.
Plan: Start with the smallest phase 4 proof, settle the development credential interface, add the real scale-set adapter and composition, then exercise the narrowest available end-to-end evidence at `min_runners: 0` and `max_runners: 1`.

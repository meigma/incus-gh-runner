---
id: 005
title: Continue phase 3 Incus lifecycle
started: 2026-07-17
---

## 2026-07-17 20:08 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 3, integrating the real Incus lifecycle behind the existing controller core.
Current state of the world: Phases 0 through 2 are merged on `master` through PRs #7, #8, and #9 at `85f273a`; the typed controller core, reproducible reference VM, one-shot guest payload contract, hosted offline proof, and live Incus validator exist, while real runtime adapters remain unwired and live Incus boot validation still requires an Incus-capable host.
Plan: Start with the smallest phase 3 proof: map the existing controller port to ownership-scoped Incus inventory and lifecycle operations, preserve bounded contexts and idempotent reconciliation, and prove behavior with focused functional evidence before expanding recovery cases.

---
id: 018
title: Implement job machine proof phase 3
started: 2026-07-20
---

## 2026-07-20 16:52 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and implementation plan, then begin Phase 3.
Current state of the world: Phases 1 and 2 are merged on `master` at `ea7e504`; strict proof primitives and the ownership-fenced host-to-VM delivery channel exist, while GitHub job/JIT correlation and the supervised signing coordinator remain unimplemented.
Plan: Re-read the Phase 3 design boundary, inspect the current GitHub and Incus lifecycle seams, implement the smallest testable correlation/coordinator slice, and verify it before expanding.

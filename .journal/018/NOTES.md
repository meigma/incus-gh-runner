---
id: 018
title: Implement job machine proof phase 3
started: 2026-07-20
---

## 2026-07-20 16:52 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and implementation plan, then begin Phase 3.
Current state of the world: Phases 1 and 2 are merged on `master` at `ea7e504`; strict proof primitives and the ownership-fenced host-to-VM delivery channel exist, while GitHub job/JIT correlation and the supervised signing coordinator remain unimplemented.
Plan: Re-read the Phase 3 design boundary, inspect the current GitHub and Incus lifecycle seams, implement the smallest testable correlation/coordinator slice, and verify it before expanding.

## 2026-07-20 17:03 — JIT-to-VM binding slice
Implemented the first Phase 3 slice on `feat/job-proof-phase-3` as commit `a91edeb`. The GitHub adapter now preserves and validates the JIT runner ID, requested name, and resolved scale-set ID. Incus preflight rejects reserved profile metadata and calculates the exact version 1 launch digest; runner creation records that digest, keeps the VM stopped, conditionally persists the JIT reference with its ETag, re-reads the same UUID and metadata, and only then boots and sends the opaque payload.

Provisioning failures after VM allocation fence the GitHub registration while retaining the original error, and every recovered owned-instance deletion fences before Incus mutation. Behavior tests cover invalid JIT references, stale ETags, failed updates, same-name replacement, fencing failure, metadata re-read, and the no-boot/no-payload guarantee. `root:format`, `root:lint`, `root:test`, and `root:build` all pass through the pinned mise/moon toolchain.

Next: add the bounded nonblocking `JobStarted` queue, exact authenticated event projection, supervised proof coordinator, and live signing/delivery wiring without broadening into Phase 4.

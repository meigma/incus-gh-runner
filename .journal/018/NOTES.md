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

## 2026-07-20 17:15 — Phase 3 draft review gate
Completed the remaining Phase 3 implementation as `9f66c51` and published draft PR #38 (`feat/provenance: bind authenticated jobs to Incus machines`) at exact head `9f66c51261ed61ad4df1840da4053c382de1cd6c`. The authenticated callback now copies only the required GitHub fields plus controller-resolved scale-set context into a nonblocking queue of `max(1, capacity.max_runners)`. Invalid or saturated events produce no proof while busy-runner demand tracking continues.

The new single-owner coordinator is a supervised application component. It re-fetches the exact owned running VM, checks the durable JIT ID/name/scale-set binding and saved UUID, reconstructs the launch digest after excluding Incus/controller metadata, signs the version 1 payload, and delivers through the Phase 2 immutable proof sink. Per-event failures are isolated; application shutdown remains bounded. Configuration reference now records saturation and the intentional acknowledged-event crash limitation.

Verification passed: full `mise exec -- moon run root:check`; race-enabled tests for `internal/provenance`, `internal/adapters/github`, and `internal/app`; hosted CI run `29789714059`; hosted CodeQL actions run `29789712983`; hosted CodeQL Go run `29789712824`; GitHub Pages and Kusari checks. Release/image packaging jobs skipped by path policy. No Phase 4 live GitHub/Incus proof was attempted, and PR #38 remains draft for human review.

## 2026-07-20 17:38 — Disabled-proof callback fix
Addressed the review finding that assigning a nil `*JobStartedQueue` to the `JobStartedSink` interface made the interface non-nil. Runtime now assigns the sink only when the proof queue exists, in commit `9079942` on draft PR #38. A disabled-mode wiring test sends a real job-start message through the upstream listener and verifies normal job logging continues without a proof-drop error.

Verification passed: `mise exec -- go test -race ./internal/runtime ./internal/adapters/github ./internal/provenance ./internal/app` and full `mise exec -- moon run root:check`. The updated PR head is `907994237cdfbf9d74acc3b344868196565bda8b`; hosted checks were queued after the push.

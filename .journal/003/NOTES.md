---
id: 003
title: Continue phase 1 controller core
started: 2026-07-17
---

## 2026-07-17 16:55 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 1 of the controller core.
Current state of the world: Session 001's design artifacts are loaded, and phase 0 landed through PR #7 at master commit `468c0a9`; the next proof is fake demand converging through coalesced reconciliation and bounded, cancellation-aware workers.
Plan: Start with the smallest runnable fake-demand reconciliation experiment, use its behavior to refine the orchestration seams, and expand phase 1 incrementally toward its exit evidence.

## 2026-07-17 17:04 — Bounded reconciliation proof
Implemented and committed the first phase 1 slice on `feat/controller-core` at `6423856`. The controller now owns a coalescing latest-demand mailbox, inventory-derived runner state, idempotent operation tracking, periodic retry, and a bounded backend worker pool. Deterministic fake-backend tests prove that slow creates do not block newer demand, concurrency stays within the worker limit, failed creates retry, busy runners survive scale-down, and shutdown cancels slow work after a grace period. Focused lint and repeated race-detector runs pass. Next: compose this core behind typed startup configuration and the Cobra command context, then exercise the complete fake application path.

## 2026-07-17 17:12 — Phase 1 exit evidence
Completed the phase 1 implementation on `feat/controller-core` through commits `ec9131c` and `242600f`. Typed immutable configuration now resolves with flags over environment over YAML over defaults, explicit config files are required while `/etc/incus-gh-runner/config.yaml` remains optional, and invalid capacity/concurrency/timeout settings fail before startup. The application supervisor runs a demand source and controller under the Cobra execution context, propagates component failures, and cancels its peer cleanly. Structured controller logs contain only operation, capacity, and log-safe runner identifiers. The deterministic fake application converges to demanded capacity; focused packages pass ten repeated race-detector runs; and `moon run root:check` passes formatting, lint, build, all Go tests, and documentation build. The executable intentionally reports that runtime adapters are unavailable until the real GitHub and Incus lifecycle phases wire them. Phase 1's documented exit evidence is satisfied; next step is review and publication of this branch, then phase 2 or the next evidence-producing slice chosen by the developer.

## 2026-07-17 17:19 — Private declaration Godoc convention
The developer clarified the house style: every Go type, function, and method should have concise identifier-led Godoc even when unexported, including test helpers, because IntelliSense is part of the review workflow. Applied the convention across the phase 1 implementation and the existing command entrypoint, and recorded it in `TECH_NOTES.md` for future sessions.

## 2026-07-17 17:29 — Phase 1 draft PR
Pushed `feat/controller-core` at reviewed head `ca6d6d6` and opened draft PR [#8](https://github.com/meigma/incus-gh-runner/pull/8), titled `feat(controller): prove phase 1 controller core`, against `master` at `468c0a9`. The PR body records the phase 1 scope, intentional deferral of real GitHub/Incus lifecycle wiring, `moon run root:check`, and repeated race-detector evidence. CI, GitHub Pages, and Kusari Inspector checks started on the exact PR head.

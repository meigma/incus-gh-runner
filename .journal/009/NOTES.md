---
id: 009
title: Continue phase 6 service hardening
started: 2026-07-18
---

## 2026-07-18 11:43 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 6 service hardening.
Current state of the world: Phases 0 through 4 are complete on `master` at `56eaf85`; phase 5 has deterministic recovery coverage plus live hot-standby replacement and idle/busy restart evidence, while bounded concurrent demand and deliberately timed provisioning/terminal-cleanup restart proofs remain open. Session 001's primary roadmap and focused controller/image proposals define phase 6 around observed systemd, timeout, outage, signal, credential, ownership, logging, and diagnostic behavior.
Plan: Inspect the current runtime and deployment surfaces, choose the smallest phase 6 proof that exposes real behavior, implement and validate that slice, then refine the remaining hardening work from what it teaches us.

## 2026-07-18 11:56 — First phase 6 recovery slice
Inspection found that the initial GitHub message-session preflight was fail-fast as intended, but any listener/session failure after startup terminated the whole controller. Implemented capped exponential session recreation on `feat/phase-6-github-reconnect`: the initial session still opens synchronously, each failed post-start session is closed within a fresh bounded context, retry delay is configurable through `retry.initial` and `retry.maximum`, and healthy polling resets the delay. Deterministic adapter tests prove cap and reset behavior without external services; `moon run root:check --summary minimal` and `go test -race ./internal/adapters/github ./internal/app` pass. Commit `3f28047` is published as PR #16. Remaining phase 6 work includes systemd deployment assets, bounded escalation for components that ignore cancellation, signal behavior under the shipped unit, and targeted Incus/GitHub outage evidence.

## 2026-07-18 11:57 — Exact-head hosted verification
PR #16 exact head `3f280476cd885f67f41a2e67b94e08dac7d7f392` passed hosted CI run `29656843615`, GitHub Pages, and Kusari Inspector. The slice is ready for review; it has not been merged.

## 2026-07-18 12:03 — Reconnect and bounded-supervision slices merged
PR #16 was squash-merged as `a76994b`. A second phase 6 slice on PR #17 added application-level shutdown escalation: after cancellation, the supervisor now waits across the controller's graceful and forced-cancellation windows, then returns `app.ErrShutdownTimeout` if a long-lived component still has not stopped. This prevents a cancellation-ignoring demand source from wedging the process forever and preserves the triggering component error on unsolicited failure. PR #17 exact head `141c297b36a0397e39733d1f7ffeb99571390b46` passed hosted CI run `29657015485`, GitHub Pages, and Kusari Inspector, then squash-merged as `439ca19`. Both implementation worktrees and remote feature branches were removed; local `master` is clean and synchronized.

## 2026-07-18 12:22 — Hardened systemd deployment merged
PR #18 added a `Type=simple` systemd unit, example production configuration, deployment/credential/ownership guidance, and an automated unit-hardening verifier. The service uses a dynamic user plus the root-equivalent `incus-admin` supplementary group, systemd credentials for the GitHub App private key, restricted filesystem/kernel/process surfaces, and a stop timeout that exceeds the application's two shutdown windows. Local `moon run root:check --summary minimal` and shell syntax checks passed; an Ubuntu 24.04 container passed `systemd-analyze verify` and reported a 2.9 exposure score. Exact head `99261bb47c260c096f35c7efdd604e86b04f3cfb` passed CI run `29657327623`, Pages, Kusari, and reference-image run `29657327587`, then squash-merged as `956a34a8e73fb011d9431ef90a54cac456ad88da`. Local `master` is clean and synchronized; the implementation worktree and local/remote feature branches were removed. Remaining phase 6 evidence needs a disposable Incus-capable host: observed systemd start/stop/restart with real credentials, plus targeted GitHub/Incus outage and create/boot/delete failure exercises.

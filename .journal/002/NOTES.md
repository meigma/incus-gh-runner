---
id: 002
title: Continue Incus runner v1
started: 2026-07-17
---

## 2026-07-17 16:31 — Kickoff

Goal for the session: Familiarize with session 001's design artifacts and prepare a new session to continue the Incus GitHub runner v1 work.

Current state of the world: Session 001 is closed and produced the focused controller and reference-image proposals plus an eight-phase implementation plan. The repository production tree remains the unmodified template at `151746d`; phase 0, repository foundation, is the earliest unfinished evidence gate.

Plan: Bind this session, await the user's concrete continuation request, then choose the smallest useful proof and revise later steps from implementation evidence.

## 2026-07-17 16:41 — Phase 0 repository foundation

Completed phase 0 on implementation branch `feat/phase-0-foundation` at commit `3022f2e` (`chore: establish repository foundation`). Renamed the Go module, binary, CLI identity, environment prefix, Moon/release/container metadata, docs, repository settings, and release helper fixtures from the template identity to `incus-gh-runner`; removed the template onboarding file and stale template changelog; reset release state to `0.0.0` because this repository has no tags or releases.

Pinned `github.com/actions/scaleset v0.4.0` and `github.com/lxc/incus/v7 v7.0.0`, verified against their current upstream stable tags, and added minimal adapter constructors without prematurely defining controller ports. Added project-specific development and disposable Incus/GitHub test-environment guidance.

Verification passed: focused Moon format/lint/build/test tasks, CLI version/default/environment smoke tests, 11 release-helper Python tests, `moon run root:check --summary minimal`, `moon ci --summary minimal`, `git diff --check`, and the invariant that the implementation branch tracks no `.journal` files. Phase 1 can now begin with the smallest fake-demand reconciliation proof.

## 2026-07-17 16:50 — Close

PR [#7](https://github.com/meigma/incus-gh-runner/pull/7) was squash-merged to `master` as `468c0a9`. Before merge, Kusari Inspector rejected the original Incus `v7.0.0` pin for nine unmitigated critical/high CVEs; the branch moved to `v7.2.0`, all local gates and hosted CI/Pages/Kusari checks passed, and then the main checkout was fast-forwarded. The `feat/phase-0-foundation` worktree plus local and remote branches were removed.

Phase 0 is complete. The next implementation session should start phase 1 with the smallest fake-demand reconciler proof and let that behavior define the controller ports. Packaging remains renamed but not release-proven, the live GitHub repository-settings manifest was not applied, and the project still needs a license before public release.

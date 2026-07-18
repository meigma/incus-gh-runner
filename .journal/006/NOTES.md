---
id: 006
title: Continue phase 4 GitHub lifecycle
started: 2026-07-17
---

## 2026-07-17 20:52 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 4, integrating the real GitHub scale-set lifecycle and proving one genuine queued job on one JIT-configured Incus runner VM.
Current state of the world: Phases 0 through 3 are merged on `master` at `d03cace`; the controller core, reference image and guest contract, and ownership-scoped Incus lifecycle are implemented, while GitHub demand and JIT composition remain unwired and the Incus-capable live gates have not yet been run.
Plan: Start with the smallest phase 4 proof, settle the development credential interface, add the real scale-set adapter and composition, then exercise the narrowest available end-to-end evidence at `min_runners: 0` and `max_runners: 1`.

## 2026-07-17 21:06 — Live test environment
The preferred live-test environment is a minimal Latitude.sh server provisioned through the already authenticated and billing-enabled local `lsh` CLI. Because provisioning and teardown take minutes and billed runtime should stay minimal, finish and rehearse the implementation, configuration, image transfer, validation commands, and cleanup locally before provisioning. Use one scripted hardware window to run the pending phase 2 image boot gate, phase 3 Incus lifecycle gate, and phase 4 genuine GitHub job proof, capture evidence, remove all test resources, and then destroy the server.

## 2026-07-17 21:50 — Phase 4 local slice and GitHub preflight
Created the isolated `feat/phase-4-github-lifecycle` Worktrunk worktree from `origin/master` at `7290aa5`. Commits `1374e2c` and `05b8f88` add persistent scale-set resolution, message polling, non-blocking demand publication, per-VM JIT generation, production Incus composition, environment-only PAT and GitHub App credential interfaces, protected diagnostics, an opt-in real GitHub preflight, the manual one-shot runner workflow, and pre-hardware live-test scripts. The real repository GitHub preflight passed with the existing `gh` credential by resolving `incus-gh-runner-phase4` and opening and closing a message session; `mise exec -- moon run root:check --summary minimal` also passed. Next: publish the branch, validate hosted CI, merge the workflow to the default branch, prepare the reference-image/binary bundle, and only then allocate the minimal Latitude host for the phase 2 through 4 acceptance window.

## 2026-07-17 23:10 — Hardware boot proof and Noble GRUB correction
Merged phase 4 as PR #11 (`e778ef1`) after local and hosted gates passed, then allocated the minimal Latitude.sh `c3-small-x86` host for one consolidated hardware window. Incus 6.0 on Ubuntu 24.04 exposed two reference-image compatibility gaps before the controller proof: the validator used an unsupported `incus image info --format json` invocation, and the image had no UEFI bootloader. PR #12 corrects the validator and installs signed UEFI GRUB. A disposable VM then isolated an Ubuntu Noble regression where `grub-install --removable` reaches a GRUB prompt despite a correct config; rerunning the same signed install without `--removable` booted successfully, the Incus agent became ready, and systemd reported `running`. Commit `6b012cc` carries the verified flag change and its local full gate passes. Next: consume the rebuilt PR artifact, run the controller plus one genuine GitHub Actions job, capture cleanup evidence, merge PR #12, and destroy the Latitude server.

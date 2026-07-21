---
id: 020
title: Implement job machine proof phase 5
started: 2026-07-20
---

## 2026-07-20 19:56 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and plan, then begin Phase 5 TPM-bound credential validation.
Current state of the world: Phases 1 through 4 are merged on `master` at `e1d5979`; genuine file-backed proof consumption is complete, and Phase 5 remains the bounded validation of the same PKCS#8 Ed25519 key through systemd-250+ TPM-bound credential storage.
Plan: Re-read the Session 014 design and plan, inspect the current deployment and test surfaces, then implement the smallest evidence-producing Phase 5 slice in an isolated Worktrunk branch.

## 2026-07-20 20:06 — Local TPM credential slice
Reviewed Session 014's design and five-phase plan against the Phase 4 implementation. The bounded Phase 5 contract remains correct: use the same PKCS#8 Ed25519 key and runtime path, change only systemd storage, retain `PrivateDevices=yes`, and make no TPM-native signing or measured-boot claim. The three pinned Go modules remain their latest published versions.

Created `feat/job-proof-phase-5-tpm` from fetched `origin/master` and committed `143b940` (`feat(provenance): add TPM-bound proof key storage`). The slice adds the `LoadCredentialEncrypted=` drop-in, exercises all four GitHub App/PAT with file/TPM proof-key combinations, adds installed-host ownership/mode/presence checks, and documents encryption, empty-PCR policy, origin and cross-host checks, rotation, escrow, replacement, reboot, and external proof verification.

Verification passed: Ubuntu 24.04 sandbox matrix; installed file and TPM verifier modes; full serial `root:check`; and explicit docs build. The first parallel full check hit a stale golangci-lint cache referencing a deleted Worktrunk path plus a concurrently killed isolation fixture; cleaning the linter cache and rerunning the affected checks serially passed. The live TPM host reboot, genuine proof, rotation, and optional second-host binding gates remain open.

## 2026-07-20 20:07 — Draft review gate
Pushed `feat/job-proof-phase-5-tpm` and opened draft PR #41: https://github.com/meigma/incus-gh-runner/pull/41. The PR explicitly remains draft until the enrolled-TPM host, reboot, genuine proof, rotation, and optional second-host binding evidence is complete.

## 2026-07-20 20:09 — Hosted checks
Confirmed draft PR #41 at exact head `143b9409ac9ec95c2341510d2c00ae5b4a36ff1f`. Hosted CI, CodeQL for Go and Actions, GitHub Pages, and Kusari Inspector passed; release dry-run jobs skipped by the draft/non-release path as expected.

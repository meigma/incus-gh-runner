---
id: 021
title: Start new work session
started: 2026-07-21
---

## 2026-07-21 14:06 — Kickoff
Goal for the session: Start a fresh journal session; the substantive work goal has not yet been provided.
Current state of the world: Master is at `4d891a8` after job machine-proof Phase 5 landed through PR #41, and the latest closed session recorded successful TPM-bound proof-key validation.
Plan: Bind this session to the current task and wait for the user's actual request.

## 2026-07-21 14:10 — Dependabot release-prep inventory
Goal for the session: Prepare for the first release by resolving every open Dependabot pull request.
Current state of the world: `master` is clean at `4d891a8`; PRs #1–#6 are the complete open Dependabot queue, covering five GitHub Actions bumps and one MkDocs dependency bump. Their narrow diffs and prior checks are green, but their heads predate substantial changes on `master`.
Plan: Process the PRs sequentially, ask Dependabot to rebase each branch, verify the refreshed signed head and current hosted checks, squash-merge with head-SHA pinning, then confirm the queue is empty and `master` is healthy.

## 2026-07-21 14:24 — Dependabot queue cleared
Resolved all six open Dependabot pull requests after sequential Dependabot-owned rebases, verified signatures, exact current-`master` parents, fresh hosted checks, and head-SHA-pinned squash merges: #1 `4c6555c`, #2 `4599562`, #3 `ae7c457`, #4 `675e568`, #5 `609e6b3`, and #6 `ae53f0e`.
Local and remote `master` now agree at `ae53f0ef9a49df01dd63e13db71abf70223c0f3a`, the working tree is clean, and the open Dependabot PR query returns an empty list.
Post-merge CI, GitHub Pages, Release Please, Dependency Graph, and both CodeQL analyses all passed on the final commit. Dependabot also reported that configured `dependencies` and `github-actions` labels do not exist; this did not block the updates but remains a small repository-hygiene follow-up.

## 2026-07-21 14:34 — Release rehearsal passed
Because Dependabot pull requests intentionally skip the expensive release jobs, manually dispatched `Release Dry Run` run `29869902836` on final `master` commit `ae53f0e`.
All five jobs passed: binary/SBOM/checksum rehearsal, native amd64 and arm64 Melange package builds, container assembly and smoke test, and reference Incus image construction and inspection.

## 2026-07-21 19:01 — Release 1.0.0 merge gate
User authorized merging Release Please PR #24 and monitoring the first release end to end.
Verified the exact PR head `ec89ebe61a0e889e647d3edd1b26f150dc79b24d`: it is GitHub-verified, has current `master` commit `e69fd1c2c2d1e9175b9a3022330eff2c72bbdb2c` as its parent, changes only the manifest, changelog, and Melange/apko version markers, and all CI, CodeQL, Pages, Kusari, binary, native-package install, and container rehearsal checks passed.
No `v1.0.0` tag or GitHub release exists before the merge. The expected flow is Release Please creating a protected tag and draft release, followed by the tag-triggered publish workflow; after artifacts, packages, image, signatures, SBOMs, and attestations verify, publish the inspected draft as the user-authorized first release.

## 2026-07-21 19:09 — Release 1.0.0 published and verified
Squash-merged PR #24 as `2ad80f9f2143435e0eec70294eade31750fe4211`. Release Please run `29884707969` created protected tag `v1.0.0` and the draft release; tag-triggered Release run `29884714489` passed every job, including binary assets, amd64/arm64 DEB and RPM installs, isolated binary provenance, signed Melange packages, multi-arch container publication, SBOM, keyless Cosign signature, isolated image provenance, and inspection summary.
Downloaded all 13 release assets before publication. All 12 payloads matched `checksums.txt`; every payload's GitHub provenance verified against `.github/workflows/attest.yml` and `refs/tags/v1.0.0`; the native Darwin arm64 binary reported version `1.0.0`, commit `2ad80f9`, and passed its CLI smoke check.
Resolved `ghcr.io/meigma/incus-gh-runner:v1.0.0` to `sha256:4445f7285ad45914495f0b847a25ac553f4495cfa65ab4120c1a4131d397f726`, confirmed linux/amd64 and linux/arm64 manifests plus version/revision annotations, ran the image successfully by digest, and independently verified its GitHub provenance and keyless Cosign signatures, including the SPDX predicate.
Published the inspected draft as the public latest release at https://github.com/meigma/incus-gh-runner/releases/tag/v1.0.0. Final-sha Release, Release Please, CI, Pages, and both CodeQL runs passed; local and remote `master` agree and the worktree is clean.
Non-blocking follow-ups: the successful release run warned that `actions/attest-sbom` is deprecated and that optional artifact-metadata storage records were not created, while direct GitHub attestation checks succeeded. Repository releases are not immutable, so `gh release verify` / `verify-asset` report no release-level attestation; payload checksums and build provenance were verified independently.

---
id: 024
title: Add LVM isolation baseline support
started: 2026-07-22
---

## 2026-07-22 17:48 — Kickoff
Goal for the session: Add a closed LVM thin-pool storage variant to the Incus isolation baseline and packaged validator while preserving the existing default ZFS contract, then deliver it as a tested ready PR.
Current state of the world: Version 1.0.0 embeds a ZFS-only CUE policy, while Catalyst needs the package-installed validator to accept its `build-vms` LVM pool after normalizing only Incus's server-generated `volatile.initial_source` field.
Plan: Create an isolated Worktrunk feature branch from current `master`, prototype the narrow CUE input and rendered contract, extend validator coverage and docs, run local and hosted validation, then hand off the ready PR without merging or releasing.

## 2026-07-22 17:55 — LVM policy prototype passes focused checks
Created `feat/incus-lvm-storage` from fetched `master` commit `dfd1e29` in its own Worktrunk worktree. The CUE input is now a closed default-ZFS/LVM disjunction; LVM derives `lvm.vg_name` from `source`, renders a bounded GiB size, and fixes the exact description/configuration. Added the Catalyst-shaped `build-vms` example and JSON fixture, embedded-policy negative cases, LVM snapshot drift coverage, generalized documentation, and Moon inputs. Focused Go tests pass, both CUE examples vet, both rendered outputs are semantically equal to their fixtures, and the original ZFS fixture is unchanged. Next: review the diff, run the full Moon gates, commit, push, open the ready PR, and wait for hosted CI.

## 2026-07-22 17:59 — Ready PR passes hosted CI
Committed the implementation as `11bfb8c` (`feat(incus): support LVM isolation baselines`), pushed `feat/incus-lvm-storage`, and opened ready PR #49 with the compatibility and validation contracts. Final local `mise exec -- moon run root:check`, `mise exec -- moon ci`, direct CUE vet/export comparisons, and in-process render/fixture tests all pass. Hosted CI, CodeQL, GitHub Pages, and Kusari passed on the exact head; release dry-run jobs skipped as expected. The worktree is clean, the PR is open and mergeable, and no release, tag, merge, release-version edit, or cleanup was performed. Expected Release Please version is 1.1.0; after release Catalyst must repin the tag/version, source commit, DEB SHA-256, binary SHA-256, unit SHA-256, and tmpfiles SHA-256.

## 2026-07-22 18:03 — PR #49 merged and branch cleaned
After explicit approval, reverified PR #49 at reviewed head `11bfb8c` with all applicable checks green and squash-merged it on GitHub as `c41f108`. The initial `gh pr merge --delete-branch` reported a local worktree conflict after the remote merge completed, so remote state was verified before continuing. Fast-forwarded the clean local `master` to the exact merge commit, removed the integrated Worktrunk worktree and local branch, and deleted the remaining remote feature branch. Post-merge CI, GitHub Pages, both CodeQL runs, Dependency Graph, and Release Please all succeeded on `c41f108`; Release Please updated ready PR #48 to `chore(master): release 1.1.0`, which remains unmerged and untouched.

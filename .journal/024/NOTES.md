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

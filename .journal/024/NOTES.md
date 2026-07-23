---
id: 024
title: Add LVM isolation baseline support
started: 2026-07-22
---

## 2026-07-22 17:48 — Kickoff
Goal for the session: Add a closed LVM thin-pool storage variant to the Incus isolation baseline and packaged validator while preserving the existing default ZFS contract, then deliver it as a tested ready PR.
Current state of the world: Version 1.0.0 embeds a ZFS-only CUE policy, while Catalyst needs the package-installed validator to accept its `build-vms` LVM pool after normalizing only Incus's server-generated `volatile.initial_source` field.
Plan: Create an isolated Worktrunk feature branch from current `master`, prototype the narrow CUE input and rendered contract, extend validator coverage and docs, run local and hosted validation, then hand off the ready PR without merging or releasing.

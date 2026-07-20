---
id: 016
title: Assess bootc image migration
started: 2026-07-20
---

## 2026-07-20 15:42 — Kickoff
Goal for the session: Preserve the completed bootc-versus-distrobuilder assessment, its live x86_64/KVM experiment, and the maintainer's final decision to reject the migration and retain distrobuilder.
Current state of the world: A disposable Fedora 44 bootc prototype passed the repository's Incus guest-contract validator on Latitude bare metal, but exposed additional conversion, compatibility, artifact-size, and provenance-boundary costs. The prototype exists only as uncommitted files in `feat/bootc-image-experiment`; no PR was opened and the Latitude server was deleted.
Plan: Record the measured evidence and decision, abandon the temporary prototype worktree without merging it, promote the durable distrobuilder decision to technical notes, and immediately close this historical session.

## 2026-07-20 15:45 — Assessment recorded
Captured the complete experiment, measurements, compatibility findings, attestation boundary, and final rejection in `BOOTC_ASSESSMENT.md`. The Fedora 44 path was technically viable and passed `image/validate-incus.sh`, while CentOS Stream 10 lacked the 9p kernel support required by the Incus agent. The maintainer chose to retain the existing distrobuilder-based Ubuntu 24.04 plan; the prototype will be abandoned without a PR.

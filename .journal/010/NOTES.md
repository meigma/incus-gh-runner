---
id: 010
title: Continue phase 7 release readiness
started: 2026-07-18
---

## 2026-07-18 14:36 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 7 release readiness.
Current state of the world: Session 001 established the controller, image, and phased v1 design; phases 0 through 6 have since landed through PRs #7 through #19, with `master` clean at `4979f7d` and live evidence covering the genuine runner lifecycle, hot standby recovery, and service hardening. Phase 7 now owns publishable controller and reference-image artifacts, complete operator documentation, and consolidated end-to-end acceptance evidence; bounded concurrent demand and deliberately timed provisioning or terminal-cleanup restarts remain optional phase 5 proof gaps.
Plan: Inspect the current release and documentation surfaces, choose the smallest phase 7 proof that reduces the most uncertainty, implement and validate it, then refine the remaining release work from observed behavior.

## 2026-07-18 14:50 — Reference-image release slice implemented locally
Inspection found that Release Please, GoReleaser binary assets, and the melange/apko controller OCI image path already existed, while the Incus reference VM was retained only as a one-day CI proof artifact and no GitHub release had yet been created. Chose reference-image publication as the smallest phase 7 uncertainty-reducing slice.
Implemented versioned reference-image staging with checksum validation and deterministic tests; extended the tag release to build, inspect, upload, and attest the VM archive; added a faithful release-PR dry run; wired all release script tests into `moon run root:check`; and documented download, checksum, provenance, and boot verification. The local aggregate gate passed with 13 tasks, including 17 release-script tests and the strict documentation build.
Next: Review and commit the bounded slice, publish it as a PR, dispatch the hosted release rehearsal on the exact branch head, and revise any workflow assumptions exposed by the real Ubuntu build.

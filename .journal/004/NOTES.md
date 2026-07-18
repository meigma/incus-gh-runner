---
id: 004
title: Continue phase 2 guest image work
started: 2026-07-17
---

## 2026-07-17 17:52 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 2, using session 001's design artifacts as the working context.
Current state of the world: Phases 0 and 1 are complete on `master` at `9bd37f7`; the repository foundation and fake-demand controller core are landed, while the guest/image contract, image build, runtime payload injection, readiness, diagnostics, secret cleanup, and terminal poweroff remain unproved. Session 001's `V1_IMPLEMENTATION_PLAN.md` is the primary roadmap, `IMAGE_PROPOSAL.md` and `CONTROLLER_PROPOSAL.md` are the focused working designs, and `TECHNICAL_PROPOSAL.md` is historical umbrella context where they differ.
Plan: Start with the smallest phase 2 evidence slice, use an offline image-build experiment to expose assumptions, then refine the one-shot guest contract and real Incus boot validation from observed behavior.

## 2026-07-17 18:03 — First guest-contract proof
Created implementation branch `feat/phase-2-guest-image` from fetched `origin/master` at `9bd37f7`. The working proof uses an Incus-agent file handoff rather than cloud-init: the controller writes a root-only versioned JSON payload, then creates a ready marker as the commit point. A systemd path unit triggers the guest launcher, which validates the payload, deletes transient files before starting the runner, exposes a secret-free status file and serial-console transitions, runs the pinned runner as an unprivileged user, and powers off on every terminal path.

Added an Ubuntu 24.04 x86_64 distrobuilder definition with Actions Runner `2.335.1`, a verified source-build path for distrobuilder `3.3.1`, a hosted offline-image workflow, the independent compliance contract, and success/fail-closed guest tests. `moon run root:check --summary minimal` passes locally. Initial implementation commit: `bf77780`; the offline VM artifact still needs hosted proof, and boot/import validation remains an explicitly separate Incus-capable gate because the local macOS host has no Incus daemon.

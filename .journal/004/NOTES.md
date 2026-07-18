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

## 2026-07-17 18:50 — Exact-head offline proof
Draft PR #9 now points at `49f71c4c97bf10fd2f4d6ae3116023c00b5036b7`. The exact-head Reference Image run `29625571125` completed successfully in 11m34s: it built the unified Incus VM archive without KVM, verified its checksum, extracted and inspected the qcow2 root disk as 8 GiB, and uploaded a 668,644,283-byte proof artifact with digest `sha256:d08277f8f19bf46e0f938764251c140bf674212d0406d0d8bc3549203a4522ff`. CI, GitHub Pages, Reference Image, and Kusari Inspector all pass on the same head.

Added `image/validate-incus.sh` as the next evidence gate. It uses an explicitly non-default Incus project, scopes cleanup to unique test resources, imports and boots the built image, injects payload then ready marker through the Incus agent, verifies the transient secret files disappear before the runner probe completes, checks secret-free status and serial-console lifecycle evidence, and requires terminal shutdown. This Mac has no Incus daemon, so that live gate has not run; PR #9 remains draft until it is exercised on an Incus-capable host.

## 2026-07-17 19:54 — Distrobuilder moved under mise
Reworked the image tool bootstrap after review: `mise.toml` now owns distrobuilder 3.3.1 as a Linux-only `http:` tool. mise verifies the upstream release source archive with its pinned SHA-256, extracts its vendored dependencies, compiles once into the tool cache with the required container-integration stub tags, and exposes the binary through `mise exec`. `image/build.sh` now only requires the mise-provided executable and builds the image; it no longer downloads or compiles project tooling itself.

The first hosted attempt exposed that mise tool-level post-install hooks preserve dependency ordering but do not put configured dependencies on hook-time `PATH`; Ubuntu's Go 1.24.13 was selected despite mise having installed Go 1.26.4. The hook now resolves the exact mise-managed Go install. A clean Linux container with system Go 1.24.13 proved the locked distrobuilder install and executable resolution, and exact-head Reference Image run `29627521172` then passed in 11m1s on commit `588bdc8a9705e8f94dca15379cbdf22becab4788`. Its artifact is 670,410,079 bytes with digest `sha256:54b8518113b7d5ddd81e156fa9fe50f1dc9282f04c7331691e0de74a10da6efe`; all required PR checks pass. The live Incus boot gate remains pending and the PR remains draft.

---
title: incus-gh-runner documentation
slug: /
description: Incus-backed ephemeral GitHub Actions runners.
---

# incus-gh-runner

`incus-gh-runner` is an early-stage controller for running one-job GitHub
Actions runners in Incus virtual machines.

Phase 1 provides the typed configuration, signal-aware application supervisor,
coalesced demand reconciliation, and bounded runner-operation core. GitHub and
Incus lifecycle integration follows in later evidence-producing slices.

The current repository foundation provides the renamed CLI, locked development
toolchain, CI gates, and isolated GitHub and Incus client adapters. Controller,
guest-image, deployment, and troubleshooting documentation will grow from
working lifecycle slices.

See the repository [README](https://github.com/meigma/incus-gh-runner#readme)
for current scope and development instructions.

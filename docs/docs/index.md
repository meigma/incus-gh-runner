---
title: incus-gh-runner documentation
slug: /
description: Incus-backed ephemeral GitHub Actions runners.
---

# incus-gh-runner

`incus-gh-runner` is an early-stage controller for running one-job GitHub
Actions runners in Incus virtual machines.

Phase 1 provides the typed configuration, signal-aware application supervisor,
coalesced demand reconciliation, and bounded runner-operation core. Phase 2
adds the reference VM and one-shot guest contract. Phase 3 adds ownership-scoped
Incus inventory and the real create, start, guest-payload, observation,
diagnostic, and delete lifecycle. Phase 4 connects a persistent GitHub runner
scale set, current demand statistics, and fresh one-runner JIT configuration to
that existing Incus lifecycle.

The current repository foundation provides the renamed CLI, locked development
toolchain, CI gates, and isolated GitHub and Incus client adapters. Controller,
guest-image, deployment, and troubleshooting documentation will grow from
working lifecycle slices. The phase 4 hardware gate completed one genuine job
through registration, poweroff, diagnostics, and deletion on Incus 7.2. Phase
5 adds deterministic hot-pool recovery coverage and a repeatable live proof for
preconnected standby dispatch and replacement. Phase 6 hardening now recreates
failed GitHub message sessions with capped reconnect backoff while preserving
fail-fast startup preflight. Application supervision also bounds cancellation
across both controller shutdown windows and fails the process when a component
remains wedged.

See the repository [README](https://github.com/meigma/incus-gh-runner#readme)
for current scope and development instructions.

---
id: 001
title: Incus runner kickoff
started: 2026-07-17
---

## 2026-07-17 13:53 — Kickoff
Goal for the session: Begin the initial work on `incus-gh-runner`; the substantive implementation goal is pending the user's next request.
Current state of the world: The private repository was created from `meigma/template-go`, `master` is clean, and the personal `journal/jmgilman` worktree is initialized and published.
Plan: Wait for the user's actual request, then inspect the relevant repository surface and proceed incrementally from working behavior.

## 2026-07-17 14:00 — Ephemeral runner architecture research
Goal clarified: Teach the expected architecture and lifecycle of self-hosted ephemeral GitHub Actions runners, with emphasis on the provisioner contract that will inform `incus-gh-runner`.
Current findings: GitHub schedules jobs by `runs-on`; capacity demand reaches a custom provisioner either through the scale-set message API/long-poll listener or `workflow_job` webhooks. The provisioner creates clean capacity, obtains a per-runner JIT configuration, starts `run.sh --jitconfig`, observes execution, exports diagnostics, and destroys the Incus instance after the one-job runner exits. GitHub automatically deregisters a successfully used ephemeral runner, while reconciliation must clean up failed starts, stale runner records, and orphaned instances.
Design direction to explore: Prefer the official Go `actions/scaleset` client for a prototype because it exposes current demand statistics, acknowledgments, JIT configuration generation, and max-capacity reporting without requiring Kubernetes; keep a webhook-driven implementation as a simpler alternative with more reconciliation burden.

## 2026-07-17 14:49 — Working design recorded
Created `TECHNICAL_PROPOSAL.md` for an Incus-backed runner scale-set controller and reusable VM image. The v1 boundary assumes a preconfigured Incus environment and limits controller ownership to image readiness plus explicitly marked runner instances.
Key decisions: use `actions/scaleset` and `github.com/lxc/incus/v7/client`; run one scale set from a systemd-supervised controller; start at zero idle runners; use one JIT configuration and one job per VM; let the guest power off after the runner exits; reconstruct state from GitHub and Incus rather than adding a database.
Next proof: build the smallest Incus lifecycle spike with fake demand and a pre-imported image, then replace fake demand with one real scale-set job. The JIT injection mechanism and release-asset import path remain deliberate prototype questions.

## 2026-07-17 15:04 — Hot standby runners clarified
Confirmed that `actions/scaleset` supports pre-provisioned `minRunners`. A true hot pool consists of fully booted, JIT-registered, connected, idle Incus VMs; desired capacity is `min(maxRunners, minRunners + TotalAssignedJobs)`.
Each standby remains ephemeral: once assigned, it runs one job, powers off, is deleted, and the controller creates a replacement to restore the idle floor. A booted-but-unregistered warm pool is possible but has higher dispatch latency and more lifecycle complexity.
The proposal's zero-idle choice remains the first proof slice rather than an architectural constraint; add `min_runners` after the single-runner lifecycle is proven.

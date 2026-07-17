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

## 2026-07-17 15:13 — Image build direction researched
The hot standby pool is now a v1 requirement because downstream consumers intend to avoid GitHub-hosted runner queue latency; the earlier zero-idle v1 boundary is superseded, though a zero-idle lifecycle spike remains useful.
For the reusable image, prefer `lxc/distrobuilder` over Canonical's `lxd-imagebuilder`: both use the same declarative, offline image-building model, but distrobuilder directly targets Incus through `build-incus --vm`.
The VM artifact can be assembled without KVM or booting a guest. Distrobuilder creates and partitions a sparse disk, attaches loop devices, formats and mounts filesystems, customizes through a chroot, and converts the result to qcow2. It therefore requires root, mount and loop-device access plus host utilities such as `qemu-img`; a standard Ubuntu GitHub-hosted VM is a plausible build environment, while `ubuntu-slim` or a normal job container is not.
Separate reproducible artifact construction from functional validation. Hosted CI can build and checksum the unified Incus image; an Incus-capable self-hosted environment should later import, boot, verify the guest contract, and power it off. Prove the hosted build path with a small spike before fixing the release workflow.

## 2026-07-17 15:17 — Lightweight image proposal recorded
Created `IMAGE_PROPOSAL.md` to capture the reusable image as an optional reference implementation rather than a controller dependency. The working path uses `lxc/distrobuilder` to build a unified Incus VM image offline on a standard GitHub-hosted Ubuntu runner, with functional boot validation kept as a separate Incus-hosted gate.
The guest contract is intentionally limited to its durable invariants: unattended boot, one runtime JIT configuration, one runner job, no embedded GitHub credentials, terminal poweroff, and collectible diagnostics. Transport details and workflow tooling remain open for the controller and image spikes to settle together.

## 2026-07-17 15:31 — Controller requirements sharpened
The controller proposal now has enough input to specify a working v1: preserve the template's hexagonal boundaries; run as a foreground systemd service; propagate SIGINT/SIGTERM through Cobra's command context; add Viper config-file loading with `/etc/incus-gh-runner/config.yaml` as the default search target; bound external operations; and use goroutines without letting Incus work block GitHub polling.
The template already uses `signal.NotifyContext`, `ExecuteContext`, `PersistentPreRunE`, and an instance-scoped Viper. Extend those seams, load configuration once, validate it into an immutable struct, and avoid concurrent Viper mutation or hot reload in v1.
The upstream `actions/scaleset/listener` invokes scaler callbacks synchronously. Our scaler adapter must therefore perform no Incus I/O: it should coalesce desired capacity and publish lifecycle hints to a reconciliation loop, then return immediately. A single-owner reconciler schedules bounded provisioning and cleanup workers and consumes their results; no unbounded goroutine-per-message model.
Normal long-poll expiry returns no message and should reconnect immediately. After transport/API failure, recreate the message session with capped exponential backoff and jitter. The Incus v7 client supports request contexts and `Operation.WaitContext`; each request and asynchronous operation should have a deadline, with timed-out operations canceled and reconciled. A hard failure that cannot be canceled should escape the application boundary so systemd can restart the process.
Hot standby remains a v1 behavior: target capacity is `min(max_runners, min_runners + TotalAssignedJobs)`. On SIGTERM, stop polling and scheduling, bound shutdown cleanup with a fresh context, and preserve already-created runner VMs for restart reconciliation rather than terminating active jobs.

---
id: 001
title: Incus runner v1 design
date: 2026-07-17
status: complete
repos_touched: [incus-gh-runner]
related_sessions: []
---

## Goal

Establish the working architecture for an Incus-backed ephemeral GitHub
Actions runner service, split the controller and reusable-image concerns, and
leave a phased v1 plan that future implementation sessions can follow without
treating early design assumptions as fixed.

## Outcome

The goal was met. The repository was bootstrapped from `meigma/template-go`,
then the session researched GitHub's scale-set and ephemeral runner lifecycle,
selected the working upstream clients, defined focused controller and image
proposals, and organized the entire v1 slice into evidence-based implementation
phases.

No implementation code was changed after the initial repository bootstrap. No
implementation branch or PR was needed; the repository's `master` branch
remained clean at `151746d` and matched `origin/master` at closeout. All design
and planning outputs are on the personal `journal/jmgilman` branch.

## Key Decisions

- Build the controller on `actions/scaleset` and the Incus v7 Go client so v1
  uses the current scale-set long-poll/JIT workflow and a native Incus adapter.
- Manage one scale set per process inside a preconfigured Incus environment;
  only explicitly marked runner instances and optional image readiness are in
  the controller's ownership boundary.
- Make hot standby a v1 requirement. Capacity follows
  `min(max_runners, min_runners + TotalAssignedJobs)`, and a standby counts as
  hot only when its JIT runner is connected and idle.
- Keep every runner ephemeral: one fresh JIT configuration, at most one job,
  guest poweroff on runner exit, host-side diagnostics, and VM deletion.
- Preserve hexagonal boundaries. The synchronous scale-set callback performs
  no Incus I/O; it feeds coalesced demand to a single-owner reconciler that
  schedules a bounded worker pool.
- Reconstruct state from current GitHub demand and Incus ownership metadata
  rather than adding a v1 controller database.
- Run as a foreground `Type=simple` systemd service with SIGINT/SIGTERM context
  propagation, bounded shutdown, request/operation deadlines, retry backoff,
  and process restart as the escape hatch for irrecoverably wedged work.
- Publish a reusable image as an optional reference, not a controller
  requirement. Build it offline with `lxc/distrobuilder`; hosted CI may build
  without KVM, while boot validation remains an Incus-capable functional gate.

## Changes and Output Artifacts

Future agents should use the artifacts in this order:

- `meigma/incus-gh-runner` repository — private GitHub repository created from
  `meigma/template-go`; its production tree remains the unmodified template at
  the initial `master` commit.
- `V1_IMPLEMENTATION_PLAN.md` — primary roadmap and v1 scope. It defines eight
  high-level phases from repository foundation through release readiness, with
  a behavioral exit proof for each phase.
- `CONTROLLER_PROPOSAL.md` — current focused controller design. It covers
  capacity, concurrency, ports/adapters, reconciliation, configuration,
  reliability, systemd behavior, acceptance evidence, and open spike
  questions.
- `IMAGE_PROPOSAL.md` — current focused reference-image design. It defines the
  optional image position, offline distrobuilder path, minimum guest
  invariants, hosted build, Incus validation, and open questions.
- `TECHNICAL_PROPOSAL.md` — original umbrella proposal and useful background.
  Its zero-idle v1 statements are superseded by the focused controller proposal
  and implementation plan; zero idle remains only an early proof slice.
- `NOTES.md` — chronological research and decision trail, including the
  upstream behavior that motivated the focused designs.

No production files, workflows, service units, image definitions, or release
artifacts were implemented in this session.

## Open Threads

- Prove the exact controller-to-guest JIT injection, readiness, diagnostic, and
  transient-secret cleanup contract.
- Select and implement the initial GitHub App and development credential
  interfaces.
- Prove distrobuilder's root, loop-device, disk-space, and artifact-size
  assumptions on a standard hosted Ubuntu runner.
- Determine from real operations whether context cancellation plus systemd
  restart is sufficient for hung Incus calls.
- Decide whether an intentional long shutdown should remove idle standbys while
  preserving busy runners.
- Begin with phase 0 of `V1_IMPLEMENTATION_PLAN.md`; no implementation phase has
  started.

## Lessons

- `actions/scaleset` listener callbacks are synchronous, so slow Incus work in
  a callback would block GitHub polling.
- A healthy scale-set long-poll expiry returns HTTP 202/no message and should
  poll again without error backoff; transport or session failures need bounded
  reconnect backoff.
- A booted but unregistered VM is warm, not hot. Avoiding queue latency requires
  already connected idle runners and replacement after every one-job use.
- Offline VM image assembly does not require KVM, but it still needs privileged
  filesystem, mount, loop-device, and image-conversion operations.

## References

- [GitHub Actions Runner Scale Set Client](https://github.com/actions/scaleset)
- [Incus Go client](https://pkg.go.dev/github.com/lxc/incus/v7/client)
- [Distrobuilder documentation](https://linuxcontainers.org/distrobuilder/docs/latest/)
- [GitHub JIT runner security guidance](https://docs.github.com/en/actions/reference/security/secure-use#using-just-in-time-runners)

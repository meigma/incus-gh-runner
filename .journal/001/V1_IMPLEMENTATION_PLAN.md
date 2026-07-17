---
title: v1 implementation plan
status: working-plan
date: 2026-07-17
session: 001
---

# v1 implementation plan

## Purpose

This plan organizes the work needed to deliver the first useful version of
`incus-gh-runner`. It is a map for future sessions, not a fixed sequence or a
detailed backlog. Each phase should prove a behavior, record what was learned,
and revise later phases when implementation exposes a better design.

Phases may overlap when that shortens feedback. In particular, the controller
skeleton and reference image can advance independently until they meet at the
guest bootstrap contract.

## v1 outcome

The finished slice consists of:

- a Go controller that runs under systemd and manages one GitHub runner scale
  set in a preconfigured Incus environment;
- a configurable pool of fully connected hot standby runners;
- one-job ephemeral runner VMs with bounded provisioning, cleanup, and restart
  reconciliation;
- a documented contract allowing operator-supplied runner images;
- an optional reusable Incus VM image built and published by this repository;
- configuration, credentials, logs, service packaging, and operator guidance
  sufficient for a real deployment; and
- functional evidence that the complete lifecycle works and recovers from the
  important v1 failure modes.

## Working method

- Choose the smallest proof in the current phase rather than implementing the
  whole phase at once.
- Keep the controller core behind hexagonal ports and move third-party details
  into adapters.
- Prefer functional lifecycle tests over extensive isolated tests that do not
  prove the system behavior.
- Use fake adapters only long enough to make concurrency and reconciliation
  behavior deterministic; replace them with real Incus and GitHub paths early.
- Treat proposal decisions as hypotheses. Record material changes in the
  journal and update this plan when a discovery changes scope or ordering.
- Do not expand into Incus environment management, multiple scale sets, or a
  GitHub-hosted-runner-compatible tool catalog during v1.

## Phase overview

| Phase | Focus | Completion evidence |
|---|---|---|
| 0 | Repository foundation | Correctly named project with a green baseline and agreed dependency seams |
| 1 | Controller core | Fake demand converges through a signal-aware, bounded reconciler |
| 2 | Guest and image contract | A reproducible VM accepts a disposable runtime payload and powers off |
| 3 | Incus lifecycle | The controller creates, starts, observes, and deletes one owned VM |
| 4 | GitHub scale-set lifecycle | One real queued job runs on one JIT-configured VM and cleans up |
| 5 | Hot pool and recovery | Connected standby capacity is maintained without duplication across failures |
| 6 | Service hardening | The controller behaves predictably under systemd, timeouts, outages, and signals |
| 7 | Release readiness | Controller, reference image, documentation, and end-to-end v1 evidence are publishable |

## Phase 0 — Establish the repository foundation

Turn the Go template into the real project without designing more structure
than the first implementation needs.

High-level work:

- rename the module, command, metadata, and user-facing template remnants;
- establish the initial hexagonal package boundaries and composition root;
- add the scale-set and Incus client dependencies at intentionally pinned
  versions;
- preserve the template's standard formatting, test, lint, security, and CI
  gates; and
- document how local development reaches a disposable Incus environment and
  GitHub test scope.

Exit evidence: the renamed CLI builds, its existing baseline tests pass, and
the two external clients can be constructed behind adapters without entering
the controller core.

## Phase 1 — Prove the controller core

Build the smallest runnable controller using fake scale-set and runner
adapters. This phase settles the orchestration shape before external lifecycle
details make failures harder to interpret.

High-level work:

- implement typed startup configuration across file, environment, and flags;
- compose signal-aware application supervision from the existing Cobra command
  context;
- implement coalesced demand, a single-owner reconciler, and bounded workers;
- model owned runner capacity and idempotent operation results;
- add structured, secret-safe lifecycle logs; and
- exercise desired-capacity changes, cancellation, worker failure, and bounded
  shutdown with deterministic tests.

Exit evidence: fake demand converges to the expected runner count while slow
fake operations do not block demand ingestion, exceed concurrency bounds, or
prevent clean cancellation.

## Phase 2 — Prove the guest and reference image

Build only enough image machinery to discover and validate the durable
controller-to-guest contract.

High-level work:

- create a minimal pinned distrobuilder definition and one-shot guest service;
- prove an Incus VM image can be assembled on a standard hosted Ubuntu runner
  without KVM;
- boot the artifact in a real Incus environment with a disposable runtime
  payload;
- settle the first JIT injection, readiness, terminal poweroff, diagnostics,
  and transient-secret cleanup mechanisms; and
- write the resulting image compliance contract independently of the
  repository's reference image.

Exit evidence: an offline-built image imports, boots unattended, consumes one
runtime payload, reports enough state to diagnose it, leaves no prohibited
secret residue, and powers off.

This phase may proceed alongside phase 1. Its guest contract becomes an input
to phases 3 and 4 rather than an up-front specification.

## Phase 3 — Integrate the real Incus lifecycle

Replace the fake runner backend while keeping GitHub demand fake. This isolates
Incus behavior and proves the controller's ownership boundary.

High-level work:

- connect to the configured existing Incus project and preflight required
  image and profile references;
- optionally make the selected image locally ready without managing unrelated
  Incus infrastructure;
- inventory only controller-owned instances using durable metadata;
- implement bounded create, injection, start, inspect, diagnostics, poweroff
  observation, and delete operations;
- reconcile partial, timed-out, failed-bootstrap, and already-stopped
  instances; and
- prove restart discovery does not duplicate or destroy valid capacity.

Exit evidence: one unit of fake demand drives a real VM through the complete
guest lifecycle and returns the environment to zero owned instances, including
after selected interruption and failure cases.

## Phase 4 — Integrate one real GitHub job

Add the scale-set adapter and close the first true end-to-end loop with no idle
floor. Keep capacity at one so protocol and guest behavior remain easy to
observe.

High-level work:

- settle the initial GitHub App and development credential interfaces;
- resolve or create the configured persistent runner scale set;
- run the long-poll message session without Incus work blocking callbacks;
- translate current scale-set statistics into coalesced desired capacity;
- request and deliver a fresh one-runner JIT configuration; and
- run one representative repository workflow through registration, job
  completion, GitHub deregistration, VM poweroff, diagnostics, and deletion.

Exit evidence: with `min_runners: 0` and `max_runners: 1`, one queued job causes
exactly one runner VM to execute exactly one job and then disappear from both
the usable runner pool and Incus.

## Phase 5 — Deliver hot standby and reconciliation

Turn the single-runner proof into the v1 behavior downstream consumers need.

High-level work:

- maintain `min_runners` JIT-registered and connected idle runners while
  respecting `max_runners`;
- account for provisioning, idle, busy, and terminal capacity without
  over-provisioning;
- replace each consumed ephemeral standby after its one job;
- support multiple bounded Incus operations without slowing the GitHub poll
  loop;
- reconcile demand and inventory periodically as well as from messages; and
- validate controller restart during provisioning, idle standby, job
  execution, and cleanup.

Exit evidence: a hot runner accepts a job with no provisioning delay, is
deleted after completion, and is replaced. Concurrent demand remains bounded,
and restarts neither duplicate capacity nor terminate active jobs.

## Phase 6 — Harden the system service

Add the operational behavior needed to leave the controller running
unattended. This phase should harden observed failure paths rather than invent
an exhaustive reliability framework.

High-level work:

- add capped GitHub reconnect backoff while treating healthy long-poll expiry
  as normal;
- enforce request, Incus operation, bootstrap, instance-age, and shutdown
  deadlines;
- escalate irrecoverably wedged work to process failure so systemd can restart
  it;
- finalize SIGINT/SIGTERM ordering and the policy for preserving active
  runners;
- provide a least-privilege `Type=simple` systemd unit and protected
  credential/configuration path;
- verify secret redaction, ownership protections, actionable logs, and useful
  diagnostics; and
- exercise temporary GitHub and Incus outages plus failed create, boot, and
  delete paths.

Exit evidence: the service starts, stops, and restarts predictably under
systemd; important transient failures recover; bounded failures do not hang
forever; and the controller never mutates an unowned instance.

## Phase 7 — Package and release v1

Turn the proven implementation into artifacts that another operator can use.

High-level work:

- complete CLI, configuration, credential, image-contract, deployment, and
  troubleshooting documentation;
- automate controller binary/package release using the repository's existing
  release conventions;
- automate the reproducible reference image build, checksum, and release-asset
  publication;
- keep real Incus boot validation as a distinct Incus-capable gate;
- provide example configuration and systemd installation assets;
- run the full v1 acceptance scenario against a prepared environment; and
- resolve, defer, or document the remaining proposal questions based on the
  implementation evidence.

Exit evidence: a fresh prepared Incus host can install the released controller,
use either the released reference image or another compliant image, maintain a
hot standby, execute a real job, recover from a controller restart, and cleanly
retire the runner.

## Cross-cutting validation

Each phase should add the narrowest useful automated coverage, but these
functional proofs define the final confidence bar:

- configuration precedence and invalid startup behavior;
- desired-capacity and concurrency convergence with fake adapters;
- offline reference image construction;
- real Incus image import and one-shot guest lifecycle;
- one real GitHub JIT job from queue to deletion;
- hot standby dispatch and replacement;
- restart during each meaningful lifecycle state;
- bounded GitHub and Incus failure behavior; and
- SIGTERM under the shipped systemd unit without killing an active job.

## Explicitly deferred beyond v1

- management of Incus networking, storage, projects, profiles, clustering, or
  firewall policy;
- multiple scale sets or heterogeneous runner pools in one process;
- dynamic configuration reload and live capacity reconfiguration;
- remote Incus setup and certificate lifecycle unless the first deployment
  requires it;
- general scheduling, placement, or autoscaling beyond the scale-set formula;
- a full clone of GitHub-hosted runner tooling; and
- sophisticated metrics, dashboards, admission policy, or automatic upgrades.

## Using this plan in future sessions

Start with the earliest phase whose exit evidence is not yet satisfied, choose
one small proof within it, and record the observed result. A phase need not be
completed in one session, and a later phase may be explored early when it can
answer a blocking question cheaply. Update this document only when the overall
scope, phase ordering, or evidence gate materially changes.

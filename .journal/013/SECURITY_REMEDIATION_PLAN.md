---
title: Security remediation plan
status: approved
date: 2026-07-18
session: 013
reviewed_commit: f493f93f6a11403ccd9af12e55c58e3b2caf7eaf
---

# Security remediation plan

## Purpose

This plan turns the targeted security review of `incus-gh-runner` into a set of
small, evidence-producing remediation slices. The goal is a runner platform
with a defensible security boundary for use inside a broader SLSA Build L3
pipeline, not a paper claim based only on ephemeral VMs or signed artifacts.

This is a working plan, not implementation approval or a fixed backlog. Each
slice should begin with its smallest useful proof, record what was learned, and
revise later work when Incus, GitHub, or Actions Runner behavior differs from
our assumptions. No release should claim Build L3 until the relevant controls
have been implemented and their resulting provenance and isolation properties
have been independently inspected.

## Security outcome

The remediated platform should establish that:

- one untrusted workflow cannot reach, influence, starve, or persist into
  another overlapping or subsequent build;
- runner workloads cannot reach the Incus API, controller host, provenance
  signing boundary, or unrelated trusted infrastructure;
- only explicitly authorized repositories and workflows can schedule jobs;
- controller, image, profile, source, and release identities are immutable and
  auditable at the point they are used;
- uncertain controller observations fail closed without deleting active work;
- release provenance accurately identifies the trusted builder and source;
- controller and VM artifacts are verified, inventoried, scanned, and updated;
  and
- deployment and incident-response guidance states the real trust boundary and
  does not rely on misleading secrecy, reproducibility, or sandbox claims.

The broader pipeline still owns workflow-level secret policy, dependency
selection, and verification of the provenance emitted for its own artifacts.

## Threat model and retained invariants

Treat workflow code as hostile and assume it gains full control of the runner
OS user and everything intentionally exposed to that one VM. Also account for
a compromised repository write credential, an unintended organization
repository, network interception, a substituted release artifact, a mutable
Incus alias/profile, and a compromised controller process.

Retain and regression-test the controls that are already sound:

- one fresh VM and one JIT registration per job, with no VM reuse;
- unprivileged job execution and root-only payload staging;
- payload completion before the ready marker and payload deletion before the
  runner starts;
- generated instance names, exact owner filtering, and foreign-instance
  preservation;
- bounded workers, operation timeouts, stale-inventory suppression, reconnect
  backoff, and systemd restart behavior;
- systemd credential delivery and secret-safe structured logging;
- SHA-pinned GitHub Actions and checksum-pinned Actions Runner download;
- minimal nonroot controller OCI packaging, signing, SBOM generation, and
  GitHub-hosted release execution; and
- explicit human approval before publishing the first release.

The owner marker remains a cleanup-scoping mechanism. It must not be described
or tested as authorization against another writer in the same Incus project.

## Finding register

The IDs below are coverage anchors, not an instruction to create one pull
request per row. A small slice should usually close several related IDs.

| ID | Severity | Concern | Planned slice |
|---|---|---|---|
| SEC-01 | High | Overlapping runners can share a network without enforced cross-runner isolation. | 2 |
| SEC-02 | High | Guidance does not block host, Incus API, metadata, control-plane, or trusted-network access. | 2 |
| SEC-03 | High | Port isolation and MAC/IP anti-spoofing are not required. | 2 |
| SEC-04 | High | The recommended Incus project is unrestricted. | 2 |
| SEC-05 | High | Instance and project CPU, memory, disk, VM-count, and network ceilings are absent. | 2 |
| SEC-06 | High | The controller's Unix-socket `incus-admin` access is root-equivalent and bypasses process sandbox containment. | 2 |
| SEC-07 | High | Ownership metadata is forgeable and is not an authorization boundary. | 2, 3 |
| SEC-08 | High | Organization scope falls back to GitHub's broad `default` runner group. | 1 |
| SEC-09 | High | Organization guidance omits selected-repository and selected-workflow restrictions. | 1, 6 |
| SEC-10 | High | `github.config_url` accepts plaintext HTTP while renewable credentials are used. | 1 |
| SEC-11 | High | Controller installation omits checksum and attestation verification. | 5 |
| SEC-12 | High | The attestation-only reusable workflow does not establish the advertised Build L3 builder topology. | 0, 5 |
| SEC-13 | High | Manual release dispatch can build a tag while provenance retains a different dispatch source context. | 0, 5 |
| SEC-14 | High | Live repository settings lack protected tags and immutable releases. | 0 |
| SEC-15 | High | Branch policy requires no independent approval, lacks sensitive-path ownership and last-push approval, and has broad bypass. | 0 |
| SEC-16 | High | Live required checks have drifted from the checked-in repository policy. | 0 |
| SEC-17 | High | All Actions are allowed and immutable SHA references are not enforced by repository policy. | 0 |
| SEC-18 | High | Release publication has no protected environment or approval boundary. | 0, 5 |
| SEC-19 | Medium | Non-not-found guest status errors can age an active runner into terminal deletion. | 3 |
| SEC-20 | Medium | One slow status read can consume the shared inventory deadline and poison later observations. | 3 |
| SEC-21 | Medium | The real backend never emits `RunnerIdle`, preventing reliable cancellation scale-down. | 3 |
| SEC-22 | Medium | The reference VM receives neither an SBOM nor release/recurring vulnerability gating. | 4 |
| SEC-23 | Medium | Ubuntu, kernel/QEMU exposure, and embedded Actions Runner updates have no explicit cadence or response policy. | 4, 6 |
| SEC-24 | Medium | The reference image is described as reproducible and offline despite live network inputs. | 0, 4 |
| SEC-25 | Medium | Preflight resolves mutable image/profile names, while create and metadata retain those mutable identities. | 3 |
| SEC-26 | Medium | Unknown or duplicate configuration fields can silently fall back to security-sensitive defaults. | 1 |
| SEC-27 | Medium | A job can inspect its current JIT/session material, while broader documentation implies it cannot. | 4 |
| SEC-28 | Medium | The final GHCR version tag is visible before smoke, signing, SBOM, and provenance gates complete. | 5 |
| SEC-29 | Medium | Console/status reads and persisted diagnostics lack byte and retention bounds. | 3, 4 |
| SEC-30 | Low | Pinned Go 1.26.4 contains GO-2026-5856/CVE-2026-42505. | 1 |
| SEC-31 | Low | Delete rechecks ownership once, then acts by reusable name without stable identity or conditional mutation. | 3 |
| SEC-32 | Assurance | The review did not run a live Incus security proof for the most important isolation and lifecycle claims. | 6 |

Operational completeness is also required even where the review did not tie a
gap to one source line:

- **OPS-01:** credential permissions, expiry, rotation, revocation, and
  compromise response for both GitHub App and PAT modes;
- **OPS-02:** an explicit policy for public repositories, fork-triggered
  workflows, and workflow/repository allowlisting;
- **OPS-03:** Incus, QEMU, kernel, host firewall, Incus API exposure, controller,
  runner-image, and dependency patch policy;
- **OPS-04:** centralized or tamper-resistant audit retention for GitHub,
  systemd, Incus, repository-setting changes, and release activity;
- **OPS-05:** host-compromise recovery, fleet draining, credential revocation,
  reimaging, evidence preservation, and affected build/provenance invalidation;
  and
- **OPS-06:** a maintained SLSA trust-boundary document identifying every
  person, service, configuration surface, and mutable input capable of
  influencing a build or its provenance.

## Slice overview

| Slice | Focus | Principal coverage | Exit evidence |
|---|---|---|---|
| 0 | Repository and release guardrails | SEC-14 through SEC-18 | Live settings match reviewed policy and a write credential cannot publish without independent review |
| 1 | Safe controller inputs and GitHub scheduling | SEC-08 through SEC-10, SEC-26, SEC-30 | Insecure URLs, implicit org groups, and malformed configuration fail before any external client is constructed |
| 2 | Incus isolation and controller authority | SEC-01 through SEC-07 | Two hostile runners cannot influence each other or reach trusted surfaces; controller authority is explicitly bounded |
| 3 | Fail-closed lifecycle and immutable runtime identity | SEC-19 through SEC-21, SEC-25, SEC-29, SEC-31 | Faults do not delete active work, canceled demand shrinks, and every mutation uses the verified identity |
| 4 | Guest and reference-image security | SEC-22 through SEC-24, SEC-27, SEC-29 | The shipped VM has an attested inventory, enforced scan policy, accurate claims, and a documented job-visible secret boundary |
| 5 | Trusted release and verified installation | SEC-11 through SEC-13, SEC-18, SEC-28 | A trusted reusable builder produces staged, verified, correctly bound release artifacts before tag promotion |
| 6 | Operational readiness and adversarial acceptance | SEC-09, SEC-23, SEC-32, OPS-01 through OPS-06 | A disposable-host security scenario and operator runbooks prove the complete boundary and recovery path |

Slices 1, 2, and 4 can advance independently after slice 0 establishes the
source/release guardrails. Slice 3 should reuse the disposable Incus environment
from slice 2. Slice 5 depends on the repository protections from slice 0 and
must land before installation documentation can make a meaningful provenance
verification promise. Slice 6 is the release-readiness gate, not a substitute
for the focused proofs inside earlier slices.

## Slice 0 — Establish repository and release guardrails

Make the control plane that can change the builder trustworthy before changing
the builder itself.

Start with a policy-only pull request:

- add ownership for workflows, release scripts, image definitions, controller
  security boundaries, deployment assets, and repository settings;
- require at least one independent approval, approval of the last push, review
  resolution, and the exact CI/release-rehearsal checks;
- narrow administrator bypass to a documented break-glass path;
- protect release-tag creation, update, and deletion;
- enable immutable releases;
- restrict usable Actions and enforce full commit SHA references;
- put publication behind a protected release environment; and
- make repository-setting reconciliation report and fail on drift;
- remove or qualify the current Build L3, offline-build, and reproducibility
  claims until evidence supports them; and
- remove or fail closed the production manual-release path until its source
  binding is corrected and tested.

Apply the reviewed settings as a distinct, observable operation after the pull
request lands and only with explicit maintainer approval. Do not mix that
external mutation with unrelated implementation. Confirm the actual GitHub
team or maintainer identity that will own security-sensitive paths before
landing `CODEOWNERS`, rather than inventing a placeholder owner.

Exit evidence:

- a captured live API snapshot matches the committed policy;
- a deliberately unpinned Action and an unauthorized tag operation are denied;
- a release attempt pauses at the protected environment;
- a security-sensitive change cannot merge without the configured independent
  owner/reviewer; and
- the expected emergency bypass is documented, narrowly held, and audited.

## Slice 1 — Fail closed on controller inputs and scheduling scope

This is the smallest code-hardening slice and should land before another
production deployment.

High-level work:

- require HTTPS for GitHub and GHES endpoints, with any loopback test exception
  explicit, disabled by default, and impossible for non-loopback hosts;
- make repository scope the safe documented default;
- require an explicit, non-`default` runner group for organization scope and
  document selected-repository/workflow restrictions;
- replace permissive configuration decoding with exact decoding that rejects
  unknown, duplicate, misspelled, and wrong-type fields;
- keep credential source exclusivity and secret redaction intact;
- update Go to at least 1.26.5 and regenerate all locked tool metadata; and
- emit actionable errors before constructing GitHub or Incus clients.

Exit evidence:

- table-driven tests reject HTTP, userinfo, malformed URLs, implicit org groups,
  typoed security keys, duplicate keys, and mixed credentials;
- a repository-scoped configuration still starts successfully;
- an organization configuration succeeds only with an explicit group;
- `govulncheck` no longer reports GO-2026-5856; and
- a negative GitHub test proves an unauthorized repository or workflow cannot
  schedule a runner.

## Slice 2 — Prove an isolated Incus deployment

Do not turn the controller into an Incus infrastructure manager. Instead, ship
a secure reference project/profile/network configuration, a validator, and
operator guidance that make the preconfigured boundary testable.

Begin with a disposable Incus 7 host and two minimal hostile VMs. Establish the
smallest configuration that proves:

- a dedicated or otherwise strongly isolated runner project;
- `restricted=true` with only the managed network, storage, profiles, and VM
  features the runner lifecycle actually requires;
- a dedicated bridge or OVN network with default-deny ingress and controlled
  egress;
- runner-to-runner port isolation plus MAC, IPv4, and IPv6 filtering;
- no path to the Incus API, controller/host addresses, metadata, signing
  services, or unrelated trusted networks;
- explicit allowlisting for GitHub/GHES and required dependency access, using a
  controlled proxy when that is the maintainable boundary;
- per-instance CPU, memory, root-disk, and bandwidth limits; and
- project VM-count, aggregate CPU, memory, and disk limits.

In parallel, run a bounded authority spike. Determine whether every required
operation can run through a project-restricted TLS identity: inventory, create,
start, guest-agent file transfer, console collection, stop, and delete. If the
proof succeeds, add TLS transport and systemd-delivered client credentials and
remove `incus-admin`. If Incus cannot delegate the complete lifecycle safely,
make a dedicated single-purpose host a mandatory v1 production requirement,
state that controller compromise is host compromise, and retain the restricted
identity work as a separately justified follow-up rather than inventing a large
broker design immediately.

Exit evidence:

- two concurrent hostile VMs cannot reach one another at L2/L3;
- spoofed addresses and forbidden network destinations fail;
- required GitHub and approved dependency traffic still works;
- resource exhaustion stops at the declared limits without degrading the
  other runner or host control plane;
- the controller identity cannot inspect unrelated projects, attach host
  paths/devices, or launch forbidden privileged configurations; and
- exported effective project, network, profile, storage, firewall, and identity
  configuration is retained with the proof.

## Slice 3 — Make lifecycle observation fail closed

Split this work into small controller pull requests so a lifecycle fix does not
hide an image-identity or cleanup change.

### 3A. Status and inventory errors

- tolerate only the precise not-found condition that means guest status has not
  yet appeared;
- propagate timeout, transport, permission, and decode failures so inventory
  becomes stale and no mutation is scheduled;
- use per-instance read budgets so one slow agent cannot consume the budget for
  later runners; and
- preserve active runners during uncertain state.

Evidence: fault-injection tests cover every error class, including one slow
runner followed by an active runner older than the bootstrap timeout. A live
agent outage must not stop or delete either active job.

### 3B. Authoritative idle state

Start with a short protocol spike to identify the smallest authoritative state
available from scale-set callbacks, GitHub inventory, and guest state. Do not
guess a durable mapping before observing a queue/cancel run, and do not trust a
job-writable hook as the authority. Before deleting a connected idle VM, prove
that its GitHub runner registration can be fenced or removed so a job cannot be
assigned during teardown. Ambiguous or restart-reconstructed state must fail
closed.

Evidence: queue enough work to scale to the maximum, cancel before assignment,
and observe convergence back to the minimum without terminating an active job.
Repeat across controller restart.

### 3C. Immutable image, profile, and instance identity

- resolve an image alias once to a fingerprint and create by that fingerprint;
- record the resolved fingerprint rather than the configured alias;
- capture and verify a digest or ETag for every effective profile before create,
  and fail closed on drift;
- distinguish the operator-friendly alias from the build-environment identity
  used for audit and provenance; and
- re-fetch owner plus stable instance identity immediately before stop and
  delete, using conditional mutation where Incus supports it.

Evidence: alias retargeting and profile-edit races cannot change the launched
environment, while delete/recreate and owner-marker race tests never mutate the
replacement instance.

### 3D. Bound observations and diagnostics

- cap status and console reads before allocation;
- truncate with an explicit marker;
- use safe file creation semantics and protected permissions;
- leave persistence disabled by default unless a documented policy enables it;
  and
- ship size- and age-based retention through `tmpfiles.d`, logrotate, or an
  equivalent maintained mechanism.

Evidence: oversized guest status/console data cannot OOM the controller or fill
the host, and retained files expire under the shipped policy.

## Slice 4 — Harden the guest and reference image

Treat the reference VM as the largest artifact in the builder's trusted
computing base.

High-level work:

- generate an SBOM from the final VM root filesystem and attest it to the exact
  released archive digest;
- scan the final filesystem during release and rescan supported released images
  on a schedule;
- cover every supported controller OCI architecture as well as the VM;
- report unfixed findings and define a narrow, time-bounded exception mechanism
  instead of silently ignoring them;
- automate stale Actions Runner detection and define Ubuntu, kernel, QEMU,
  runner, and image-rebuild response targets;
- correct `reproducible` and `offline-built` language immediately, or introduce
  snapshots and repeat-build evidence before restoring those claims;
- capture resolved Ubuntu packages, Actions Runner digest, build-tool versions,
  and other external materials in release evidence; and
- state clearly that an untrusted job can inspect its own same-UID Actions
  Runner/JIT session material, although it cannot obtain the controller's
  renewable credential or persist into another VM.

Run a small JIT-boundary spike before changing the guest architecture. If a
separate listener/worker identity can be added without maintaining a fragile
fork of Actions Runner, prove it. Otherwise document the bounded one-job
exposure and ensure no provenance signing secret or cross-job credential enters
the runner VM. If the captured JIT/session material can be reused beyond the
assigned job, treat that result as release-blocking rather than documentation
only.

Exit evidence:

- a released VM has a downloadable and attested SBOM tied to its archive digest;
- release and scheduled scans exercise the final root filesystem and publish
  actionable results;
- an intentionally vulnerable fixture fails the gate while an approved,
  time-bounded exception is visible;
- stale runner-version detection is tested;
- documentation accurately distinguishes networked, non-hermetic construction
  from reproducibility; and
- an adversarial job demonstrates the exact JIT material it can and cannot
  access, with no renewable controller or signing credential present.

## Slice 5 — Establish a trusted release path

Prototype the supported Build L3 construction for one artifact class before
generalizing it. The controller binary is the smallest proof; the reference VM
and OCI image can follow once the resulting predicate has been inspected.

High-level work:

- move build steps, subject computation, and attestation into a trusted reusable
  builder workflow whose identity is the asserted builder;
- inspect the generated predicate for source, workflow, parameters, materials,
  subject digest, hosted-runner identity, and signer isolation;
- remove manual tag input, or invoke the workflow from the exact protected tag
  and prove that event ref, checkout commit, version, and provenance source are
  identical;
- publish only to a run-specific staging reference until smoke, vulnerability,
  signature, SBOM, and provenance gates succeed;
- promote the final GHCR version tag only after all gates and protected
  environment approval;
- make staging cleanup and final promotion retry-safe after partial failure;
- make release rehearsals exercise the same trust-boundary logic without
  creating public release references;
- update binary, OCI, and VM installation guidance to verify checksum,
  repository, signer workflow, protected source ref, expected subject, and
  hosted-runner policy; and
- record the installed controller and imported VM digests for later incident
  response.

Exit evidence:

- verification accepts every intended artifact and rejects the wrong source,
  workflow, tag, subject, repository, or self-hosted signer;
- the predicate identifies the workflow that actually performed the build;
- a forced post-publish gate failure leaves no official version tag or
  immutable release artifact;
- the release environment records independent approval; and
- a clean host can follow the documented verification procedure before granting
  the controller Incus authority.

## Slice 6 — Prove operational security end to end

Turn the focused proofs into one disposable-host acceptance scenario and the
minimum runbooks needed to operate the builder safely.

The scenario should:

- deploy from verified controller and VM digests into the secure Incus
  reference configuration;
- prove an authorized private workflow can run while an unauthorized
  repository/workflow and prohibited fork path cannot schedule;
- overlap two adversarial jobs and test network, spoofing, resource, host/API,
  metadata, and cross-runner boundaries;
- interrupt one guest agent and the controller without deleting active work;
- queue and cancel demand and observe scale-down to the configured minimum;
- retarget an alias, mutate a profile, and race a foreign/replacement instance
  without changing or deleting the verified runner environment;
- exercise bounded diagnostics and retention;
- finish with zero owned VMs, an intact foreign sentinel, and no renewable
  credential in guest evidence; and
- preserve an evidence bundle containing effective configuration, artifact
  digests, attestations, SBOMs, scan results, GitHub policy snapshots, logs, and
  cleanup verification.

Complete concise operator procedures for credential lifecycle, public/fork
policy, patch response, audit retention, controller/host compromise, fleet
draining, artifact invalidation, and recovery. The SLSA trust-boundary document
should distinguish the GitHub control plane, repository policy, reusable
builder, Incus daemon, controller identity, image/profile/network configuration,
host operator, and untrusted workflow tenant.

Exit evidence: a reviewer can reproduce the security scenario from a clean
host, verify the evidence independently, map every SEC/OPS item to a passing
proof or explicit accepted risk, and determine exactly which builds and
artifacts must be invalidated after each documented compromise class.

## Decision spikes

Keep these questions experimental until evidence makes the durable choice
clear:

1. Can a project-restricted Incus TLS identity perform the full required guest
   lifecycle without recovering host-wide capabilities?
2. Which GitHub/scale-set signal can authoritatively distinguish connected idle
   runners from busy runners across restart?
3. Is profile digest revalidation sufficient, or should effective devices and
   configuration be materialized directly on each instance?
4. Which VM SBOM/rootfs scan path produces a stable subject binding without
   depending on privileged, runner-specific inspection tricks?
5. Can listener and job identities be separated without forking Actions Runner,
   and is the bounded current-job JIT exposure worth that complexity?
6. What is the smallest reusable-builder interface that works consistently for
   binaries, the reference VM, and the multi-architecture OCI image?

Resolve each spike with a small prototype, adversarial test, and journaled
decision. Do not let an inconclusive spike silently become a security claim.

## Completion policy

Before production use in the broader SLSA pipeline:

- every High finding must be closed with evidence;
- every Medium and Low finding must be fixed or recorded as an explicit,
  bounded accepted risk with an owner and review date;
- every operational item must have an executable procedure or an explicit
  external owner;
- no documentation may claim secrecy, reproducibility, isolation, or Build L3
  beyond what the retained evidence proves;
- live GitHub and Incus configuration must match reviewed, versioned policy;
- current CI, race tests, lint, vulnerability scanning, image validation,
  release verification, and the disposable-host scenario must pass on the exact
  reviewed commit; and
- publication remains a separate maintainer approval after the final evidence
  is reviewed.

## Expected planning artifacts

Execution should leave a small, durable set of review surfaces rather than a
large design archive:

- versioned secure Incus reference configuration and validator;
- updated threat/security model and production deployment guidance;
- focused controller and guest regression tests;
- VM SBOM, scan, and update automation;
- trusted reusable builder workflows plus verifier tests;
- reconciled repository policy and live-policy evidence;
- operational security and incident-response runbooks; and
- one checksummed disposable-host security evidence bundle.

## Using this plan

Start with the earliest unsatisfied exit evidence, choose one proof-sized
change, and stop for review when that proof changes the assumed architecture.
Update this plan only when the overall boundary, ordering, or completion policy
changes materially. Detailed task lists belong in the implementation session
that owns the current slice, not here.

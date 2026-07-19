---
id: 013
title: Plan SLSA security remediation
started: 2026-07-18
---

## 2026-07-18 20:15 — Kickoff
Goal for the session: Draft a reviewable planning document that addresses every issue identified by the targeted security review of the controller, reference runner image, release supply chain, repository controls, and recommended Incus deployment.
Current state of the world: `master` is clean at `f493f93f6a11403ccd9af12e55c58e3b2caf7eaf`; the review found strong VM-per-job and artifact-hardening foundations but identified release-blocking gaps in cross-build isolation, controller authority, GitHub access scoping, transport security, installation verification, provenance claims, and live repository governance, plus controller, image, and operational hardening work.
Plan: Consolidate the findings into small evidence-producing remediation slices, define dependencies and explicit proof gates, preserve the existing working controls, and stop with a standalone plan for human review before implementation begins.

## 2026-07-18 20:31 — Drafted security remediation plan
Created `SECURITY_REMEDIATION_PLAN.md` as a standalone review draft. It preserves the existing security invariants, maps SEC-01 through SEC-32 and OPS-01 through OPS-06, and organizes remediation into seven proof-sized slices: repository guardrails, safe inputs and GitHub scheduling, Incus isolation and authority, fail-closed lifecycle behavior, guest/image security, trusted release and installation, and adversarial operational acceptance.

The plan deliberately leaves six architecture choices as prototype-driven decision spikes instead of fixing speculative designs up front. Coverage validation confirmed every finding and operational ID appears in the document. No controller, image, workflow, repository setting, or deployment implementation was changed.

Next: pause for human review of the planning document. Implementation requires a later explicit request.

## 2026-07-18 21:36 — Slice 1 draft PR ready for review

The user approved the remediation plan and authorized Slice 1. Implemented the
slice on `feat/security-slice-1` at commit
`ce976af2c21b025691c7ccfdb081603ba2f43eda` and opened draft PR
[#27](https://github.com/meigma/incus-gh-runner/pull/27).

The controller now rejects non-HTTPS or ambiguous GitHub URLs, unsupported
enterprise scope, unsafe query-interpolated scheduling identifiers, repository
scope with a custom runner group, and organization scope using the default
group. GitHub responses must not conflict with the requested group or scale-set
identity and must expose the sole routing label plus disabled runner
self-update. Configuration files are decoded exactly from one read and reject unknown, duplicate,
misspelled, aliased, malformed, or wrong-type YAML without echoing field
values. Deployment guidance now makes private-repository scope the hardened
default and defines the selected-repository, commit-pinned workflow, credential,
and negative-scheduling requirements for organization scope. Go and the locked
mise metadata were updated to 1.26.5, and Moon is forced onto mise's verified
PATH.

Local evidence passed: `go test -race ./...`, `moon ci --summary minimal`,
`golangci-lint run ./...`, `go mod verify`, readonly module listing, documentation
build, release-config tests, image-contract tests, and three independent
read-only security/test/documentation reviews. `govulncheck` v1.1.4 with Go
1.26.5 and the 2026-07-08 database reported no reachable vulnerabilities and no
GO-2026-5856. This was one-off evidence from the ambient scanner, not a
mise-pinned gate. The local systemd task was skipped on macOS and remains covered
by hosted Linux CI.

The live negative GitHub scheduling proof remains open: the available credential
received 403 while reading organization runner groups, and no pre-existing
disposable unauthorized repository/workflow fixture was available. No runner
group settings, repository settings, or workflow dispatches were changed. Do not
claim SEC-09 or the Slice 1 acceptance gate complete until that test is run and
retained. Next: wait for exact-head PR CI and human review; do not merge.

## 2026-07-18 21:46 — Exact-head hosted checks green

All hosted checks completed successfully on PR #27 head
`ce976af2c21b025691c7ccfdb081603ba2f43eda`: CI, CodeQL, GitHub Pages, Kusari
Inspector, and the full reference-image build. The reference-image job completed
in 9m51s. Release-dry-run and Pages-deployment jobs were skipped by their normal
event conditions, with no failed or pending checks. The draft PR is merge-clean
and remains paused for human review; the live negative scheduling acceptance
gate documented above is still open.

## 2026-07-18 21:52 — Slice 1 merged and Slice 2 started

The user approved PR #27 with the documented live negative-scheduling gap still
open. Rechecked the exact reviewed head and required checks, marked the PR ready,
and squash-merged it through GitHub with head matching enforced. `master` now
contains `fix(security): fail closed on controller inputs` at
`f04f474510e2fbeff0ed23ed976e0a72e65a720b`; the local default worktree was
fast-forwarded to the same commit.

Started the next numerical remediation slice on isolated branch
`feat/security-slice-2`. The first proof-sized increment is intentionally
bounded to a secure reference Incus project/network/profile baseline, a
fail-closed drift validator, an explicitly disposable hostile-VM acceptance
harness, and a parallel authority spike for a project-restricted TLS identity.
No live Incus host, GitHub runner settings, paid infrastructure, or other
external system has been mutated. Slice 0 repository guardrails and Slice 1's
live negative scheduling proof remain separately open.

## 2026-07-18 22:38 — Slice 2 baseline and authority increment proven

Implemented the first Slice 2 increment on `feat/security-slice-2`. It now has
a declarative Incus 7 baseline, GET-only fail-closed validator and offline drift
matrix, hostile two-VM acceptance harness with destructive opt-in and
marker-checked cleanup, Moon/CI integration, and corrected production guidance.
The controller examples use only the validated runner profile, describe
`incus.owner` as a forgeable cleanup selector, require a single-purpose host,
and replace the old unrestricted Incus setup commands.

Live discovery on disposable Multipass VM
`incus-gh-runner-slice2-20260718` materially changed the design. Incus 7.0.1
rejected a project-local managed bridge, so the final baseline uses a
host-owned bridge and ACL in `default`, sets `features.networks=false` and
`limits.networks=0` in the runner project, and attaches the default-deny ACL at
both the host network and direct NIC layers. The final manifest matched the
effective API state with an existing ZFS pool. A live change from default
egress `reject` to `allow` was rejected as network drift, then the restored
state matched again.

A project-restricted, server-pinned TLS certificate passed the complete
container-compatible lifecycle and negative authority matrix: project-scoped
inventory, create/start, operation waits, guest-agent file push/pull, console,
stop/delete, foreign-project isolation, forbidden devices/config, container
blocking, and revocation. It did not prove KVM boot or VM guest-agent readiness.
More importantly, the identity can still mutate project-local profiles and
therefore weaken direct NIC ACL, anti-spoofing, or port isolation. The slice
keeps `incus-admin` plus the mandatory dedicated host instead of presenting
restricted TLS as a finished least-privilege boundary.

Checksummed effective configuration, negative output, authority results, and
the exact disposable scripts are indexed in `SLICE_2_EVIDENCE.md`. The
disposable VM was permanently purged after export. Local `moon ci --summary
minimal`, `go test -race ./...`, shell syntax, JSON parsing, documentation, and
whitespace gates passed after clearing a stale golangci-lint cache that still
pointed at the removed Slice 1 worktree.

Open Slice 2 exit gates: KVM-backed hostile VMs; IPv6 spoofing; resource-limit
exhaustion without host/control-plane degradation; and a controller identity
that cannot weaken every enforced isolation control. This increment must remain
draft/in-progress and must not be described as full Slice 2 completion. Slice 0
and the Slice 1 negative GitHub scheduling proof also remain open.

## 2026-07-18 22:48 — Slice 2 increment opened for review

Committed the documentation trust-boundary correction as `88185de` and the
baseline, validator, hostile-test harness, documentation, and CI increment as
`0e6d70cd0205517ae2272e552610624f94e560c9`. Pushed
`feat/security-slice-2` and opened draft PR #28:
https://github.com/meigma/incus-gh-runner/pull/28

GitHub reports the draft as mergeable at the exact expected head. Hosted CI,
CodeQL, GitHub Pages, Kusari Inspector, and the reference-image build started
and remain pending at this checkpoint. The PR body carries the four open Slice
2 gates explicitly; no completion or readiness claim has been made.

## 2026-07-18 22:56 — Slice 2 increment exact-head checks green

All hosted checks completed successfully on PR #28 head
`0e6d70cd0205517ae2272e552610624f94e560c9`: CI, both CodeQL analyses,
GitHub Pages, Kusari Inspector, and the full reference-image build. The image
job completed in 8m18s. Release-dry-run and Pages-deployment jobs were skipped
by their normal event conditions, with no failed or pending checks. The PR
remains a draft because the four Slice 2 acceptance gates above are still open.

## 2026-07-18 23:47 — CUE configuration proof added to Slice 2

Implemented the user-approved proof-sized CUE increment on
`feat/security-slice-2`. The dependency-free
`github.com/meigma/incus-gh-runner/config@v0` module exposes a closed operator
surface for dedicated Incus names, declared host capacity and reserve, runner
sizing, an IPv4 bridge, controlled DNS/proxy endpoints, and a ZFS source. It
derives the complete existing fail-closed baseline plus a partial controller
configuration that pins `incus.project`, the sole profile, and
`capacity.max_runners` to the same inputs. Security controls remain exact and
non-overridable; unknown fields, the `default` project, insufficient headroom,
invalid endpoints, and attempted weakening all fail tests.

CUE v0.16.1 is checksum-locked through mise for Linux/macOS on amd64/arm64.
The module carries matching Apache-2.0 and MIT license texts and is exercised by
format, tidy, concrete vet, golden-render, custom-sizing, controller-alignment,
and negative-case tests in the Incus isolation Moon gate. The JSON validator
now accepts one explicitly configured non-DNS TCP proxy port while retaining
exact DNS and three-rule ACL shape checks.

Local `mise exec -- moon ci --summary minimal`, `go test -race ./...`, shell
syntax, CUE contract, validator, hostile-harness, documentation, and whitespace
gates passed. Two independent security/correctness reviews found no remaining
actionable issue in this proof-sized increment. A local
`cue mod publish --out ... v0.0.1` rehearsal produced a valid OCI layout with
the module, examples/tests, README, and both license texts; nothing was pushed
to the CUE registry.

Committed as `1e8cea9e13c5f4c03cc746afbac9f658711744d2`, pushed the existing
branch, and updated draft PR #28 to describe the CUE increment and deferred
publication boundary. All exact-head hosted checks are green: CI, CodeQL,
GitHub Pages, Kusari Inspector, and the reference-image build (8m14s); five
event-inapplicable jobs skipped normally. The PR remains draft and the existing
Slice 2 KVM, IPv6 spoofing, resource-exhaustion, and least-authority gates remain
open. No live Incus host or registry was mutated.

## 2026-07-19 08:45 — CUE public API documentation completed

Addressed the user's IntelliSense review feedback on the existing CUE proof.
CUE exports every package identifier that does not start with `_`, including
nested definition fields, so the seven validation helpers are now hidden
`_#...` definitions and the intended main-package API is exactly `#Inputs` and
`#Deployment`. Added identifier-led documentation to every field in both
definitions, including the complete input surface, derived controller fragment,
rendered Incus manifest, quoted Incus keys, computed per-pool disk limit, and
all three anonymous ACL rule elements. The example exposes only documented
`baseline` and `controller` fields; all test fixtures and cases are hidden.

The consolidated `cue def` view preserves the comments. Golden rendering,
negative cases, the Incus validator and hostile-harness contracts, full local
Moon CI, `go test -race ./...`, whitespace checks, and an independent CUE API
review all passed without rendered baseline drift. Committed as
`110530d5f8f5a800f8d7ca23942775efa8ac01f2`, pushed the branch, and updated
draft PR #28. Exact-head CI, both CodeQL analyses, GitHub Pages, Kusari, and the
reference-image build (8m43s) are green; five event-inapplicable jobs skipped
normally. The four broader Slice 2 acceptance gates remain open and the PR
remains draft.

## 2026-07-19 10:12 — One-binary Go validator increment ready for review

Replaced the 375-line Bash drift validator and its generated fake-command test
harness with `incus-gh-runner validate <baseline>`. The existing no-subcommand
controller invocation remains compatible, while the validation subcommand has
an isolated startup path that does not load controller YAML, initialize GitHub
credentials, or start controller logic. It accepts exactly one rendered JSON
baseline and a local `--socket` override, defaulting to
`/var/lib/incus/unix.socket`.

The shipped CUE source now owns a hidden, closed `_#Baseline` runtime schema in
addition to the documented `#Inputs` and `#Deployment` API. The binary embeds
that same source and uses CUE Go v0.16.1 in process: schema unification,
concrete validation, and final subsumption reject conflicts, unknown fields,
unresolved values, and omitted fixed fields. Manifest reads require a regular
file and are capped at 1 MiB before CUE evaluation or Incus socket access.

The validation core strictly decodes the policy-approved JSON and compares
typed writable projections of the server, project, default-project network and
ACL, runner-project profile, and global storage pool. The Incus SDK adapter
contains only getter calls; a Unix-socket functional test records the exact GET
routes and project query scopes. ACL ordering is normalized across every
writable rule field, only `volatile.initial_source` is ignored for storage, and
drift errors no longer print expected or live configuration. Documentation,
Moon inputs, and native and OCI release smoke tests now use the single binary.

Local evidence passed: `mise exec -- moon run root:check`, focused and full Go
race tests, CUE render/schema tests, `go mod verify`, readonly module listing,
`go mod tidy -diff`, documentation, lint, formatting, and whitespace gates.
Govulncheck v1.1.4 reported no reachable vulnerabilities; the sole module-only
advisory is for the unimported `golang.org/x/crypto/openpgp` package. Two
independent security/correctness reviews found no actionable issues. Retained
limitations are bounded CUE evaluation without a deadline, sequential rather
than atomic Incus reads, and the inability to re-prove generation-time physical
host headroom from the rendered manifest.

The stripped release binary grows from 13.8 MB to 21.2 MB, a 7.4 MB (53.6%)
increase accepted for one downloadable binary with no runtime `cue`, `incus`,
or `jq` dependency. Committed as
`1030bde9ed869676bdffa26c5cf10045de4ce0f7`, pushed the existing branch, and
updated draft PR #28. Exact-head CI, CodeQL, GitHub Pages, Kusari Inspector, and
the reference-image build (7m22s) are green; event-inapplicable release and
Pages jobs skipped normally. The Go replacement was not exercised against a
newly provisioned live Incus host, and no paid host or registry was mutated.
The existing KVM, IPv6 spoofing, resource-exhaustion, and least-authority Slice
2 gates remain open, so the PR remains draft.

## 2026-07-19 12:10 — Combined KVM runtime gate opened as draft PR #29

Merged the reviewed baseline/CUE/Go-validator increment through PR #28; master
advanced to `10f4be261419f0c36761a0e99b6e5c84ebf1dbad`. Started the user-approved
follow-up on `feat/security-runtime-acceptance` and committed the locally proven
implementation as `3a5ca0bfd294d189790dc4773ed8d5b259f22a46`. Draft PR #29 is open at
https://github.com/meigma/incus-gh-runner/pull/29 while its exact-head hosted
checks and rebuilt reference image run.

## 2026-07-19 14:39 — Retained findings, removed the one-off framework

After reviewing the permanent maintenance cost, the user chose to treat the
5,339-line Go runtime-acceptance framework as a disposable proof spike. Removed
the source-only command, `internal/liveacceptance` packages, shell integration,
helper-specific contracts, and operator instructions in commit
`8454247d61f4f3db01ccede80a80306fe915504d`. The resulting PR changes only 13
existing files, with 198 additions and 37 deletions relative to `master`; it
adds no binary, package, recurring pressure framework, or dependency.

Retained the durable outcomes: exact CUE constraints for NIC-level IPv6 denial
and managed bridge names; matching CUE and embedded-policy negative tests;
reference-image partition/filesystem growth; the `br_netfilter` prerequisite;
and the live-discovered launch EOF, inetd socket handoff, noexec-safe listener,
and agent-responsive concurrency fixes. The CUE policy, rendered baseline, and
image recipe blobs are byte-identical to those exercised at paid-host head
`3d787dc1a0aac7a59e34b68e4ebc4f318ee7854f`.

Local `moon run root:check`, full Go race tests, focused CUE, image and shell
contracts, documentation build, and whitespace checks passed. Two independent
final reviews found no security or correctness regression after exact
bridge-name grammar and one stale proof reference were corrected. Retitled PR
#29 to `fix(security): harden Incus runner isolation` and rewrote its body to
separate retained implementation from historical one-time evidence. The PR
remains draft and its new exact-head hosted checks are pending.

The checksummed runtime bundle remains unchanged. It is historical evidence
for pre-trim head `3d787dc`, not a claim that final head `8454247` contains or
can rerun the discarded framework. No paid host was reprovisioned.

## 2026-07-19 14:47 — Trimmed exact-head checks green

All hosted checks completed successfully on final PR #29 head
`8454247d61f4f3db01ccede80a80306fe915504d`: CI, both CodeQL analyses,
GitHub Pages, Kusari Inspector, and the rebuilt reference image. The image job
completed in 7m29s; five event-inapplicable release and Pages jobs skipped
normally, with no failed or pending checks. The PR remains draft for human
review.

The hostile harness can now invoke a source-only Go helper that binds evidence
to an explicitly injected clean revision, the exact helper SHA-256, rendered
baseline digest, and immutable image fingerprint. On a disposable two-runner
host it requires the exact Incus-reported QEMU PID to hold `/dev/kvm`, reported
Secure Boot and an agent canary round trip, exact project-limit rejection of a
third VM, controlled self-assigned/spoofed/link-local IPv6 denial with positive
controls, and ten minutes of materially bound CPU/memory/synchronous-disk
pressure while independent API, peer, egress, host-memory, daemon, kernel, and
ZFS watchdogs remain healthy. Cleanup is marker-bound, run-scoped, and
fail-closed across ambiguous starts and cancellations; command output and
retained evidence are bounded.

The CUE baseline now fixes NIC `ipv6.address=none`. The reference image now
grows its 8 GiB root partition and ext4 filesystem to the configured Incus root
device so the default 20 GiB CUE input is effective inside the guest. The
acceptance artifact and documentation retain the root-equivalent local Incus
socket, unsigned-EFI negative-test, aggregate runtime throttling, and NIC/ZFS
throughput limitations rather than overstating closure.

Local `moon run root:check`, full and focused race tests, clean-cache lint,
CUE/policy/image/hostile-harness contracts, documentation, explicit-provenance
Linux cross-build, whitespace checks, and an independent false-pass/paid-host
review passed. The remaining action for this PR is one exact-commit,
exact-image paid KVM window, checksummed evidence retention, and verified host
destruction. Least-privilege Incus authorization deliberately remains a later
slice rather than being folded into this runtime proof.

## 2026-07-19 13:54 — Exact-head KVM runtime gate passed and host destroyed

Completed the paid bare-metal acceptance window for draft PR #29. Live use
materially improved the implementation before the final run: Incus rejected
the original overlong bridge name, VM IPv6 filtering required the host
`br_netfilter` module, Ubuntu mounts `/run` with `noexec`,
`systemd-socket-activate` needed `--inetd` for the synthetic HTTP responders,
and `incus launch` could wait for YAML when inheriting open non-TTY stdin. The
tracked fixes now constrain bridge names to 2–15 lowercase identifier
characters, require and document bridge netfilter, invoke `/run` scripts
through `/bin/sh`, stage the pressure helper under root-owned `/root`, use
inetd socket handoff, and close launch stdin. Local focused race tests, the
Incus/image contracts, and the complete `root:check` gate stayed green after
each correction; independent reviews found no remaining actionable issue.

The final source is
`3d787dc1a0aac7a59e34b68e4ebc4f318ee7854f`. The exact-head hosted reference
image workflow run `29702324164` passed in 9m22s, and its downloaded archive
verified as
`ae1e2b082d50b4f6daf6bdf35561f12b170fd20cacfc09ffcf2a4149c330db1a`.
That same fingerprint was imported and used by both final VMs. The
provenance-bound helper digest was
`93fa9da2ced9718f8a4f5fde171f3a091eaaea68cc14359e0aef1a9384930e60`,
and the rendered baseline digest remained
`c2aac4737d94483bf308fa356546c7c50499a0ec51c3aa261397a47126c438d2`.

Run `hostile-20260719203752-424722` passed the full parent and helper gates:
exact KVM QEMU PIDs and `/dev/kvm` FDs, reported Secure Boot, agent round trips,
guest-API absence, expanded root filesystems, two-VM admission with exact third
VM rejection, self-assigned/spoofed/link-local IPv6 denial with positive
controls, cross-runner L2/L3 denial, forbidden direct and proxy paths, approved
GitHub egress, and MAC/IPv4 spoof rejection with recovery. The ten-minute
pressure window produced 602 API and peer samples with zero failures; API
p95/max was 21.6/34.7 ms, peer max gap was 1.13 s, minimum host
`MemAvailable` was 24,386,736,128 bytes, and the daemon, kernel, and ZFS checks
remained healthy. Both checksum manifests passed, and marker-bound cleanup left
zero instances.

The final 3.9 MB non-image host archive and the 556 KB journal evidence subset
were copied and independently reverified before teardown. Latitude.sh accepted
destruction of exact server `sv_ZozMazxYBN7kw`; a later exact-ID lookup returned
`404 NotFound`, and the server was absent from the project list. Runtime was
about 1h23, approximately $0.72 at the quoted $0.52/hour before provider billing
granularity.

All exact-head hosted checks are green and PR #29 is merge-clean. It remains a
draft for human review. The KVM, IPv6, and bounded resource-survival gates are
now closed; least-privilege Incus authorization remains the sole open Slice 2
exit boundary because the restricted project identity can still weaken
project-local profile controls.

## 2026-07-19 14:58 — Focused isolation hardening merged

The user approved the trimmed PR #29. Reverified exact reviewed head
`8454247d61f4f3db01ccede80a80306fe915504d`, its clean merge state, and all
hosted checks, then marked the PR ready and squash-merged it through GitHub as
`99022572225f906c0bea56565b091a13cd9e12df` with title
`fix(security): harden Incus runner isolation`.

Fast-forwarded the clean local `master` to the same commit and confirmed its
tree exactly matches the reviewed feature tree. Worktrunk identified the
feature worktree as integrated by tree equality; removed that worktree and
local branch, then deleted the remote feature branch. PR #29 is merged and the
default checkout is clean and current. Session 013 remains open because the
least-authority Incus boundary is still a separate Slice 2 exit gate.

## 2026-07-19 16:18 — Slice 3A local proof ready for review infrastructure

Started Slice 3A from merged `master` commit
`99022572225f906c0bea56565b091a13cd9e12df` on
`feat/security-slice-3a`. The Incus inventory adapter now distinguishes the
precise tolerable condition—`status.json` is absent while the named instance
still exists—from an instance disappearing or any timeout, transport,
permission, decode, version, or state error. Every other uncertainty invalidates
the complete refresh, allowing the controller's existing stale-inventory
boundary to retain its previous observation and schedule no mutation.

Owned instances are inspected in deterministic name order. Each guest-status
read receives the smaller of a five-second individual cap and its fair share of
the remaining controller operation deadline. Inspection continues after a
runner-level failure so a slow early agent cannot consume the later active
runner's opportunity to report, but any accumulated error still suppresses the
partial snapshot.

Fault injection now covers disappeared instances, deadlines, transport and
permission errors, malformed JSON, unsupported versions, unknown states, a slow
runner followed by an active runner older than the bootstrap timeout, and the
controller retaining two active runners while inventory is uncertain. The
guest-contract reference documents the same fail-closed boundary.

Focused race tests passed. The timing-sensitive controller test passed 100
repetitions and the backend uncertainty/budget tests passed 50 repetitions.
After clearing stale golangci-lint cache entries for an already-removed
worktree, the complete `moon run root:check` gate passed: formatting, lint,
build, Go tests, docs, image contract, Incus isolation contract, release config,
and the platform-applicable systemd check. The live agent-outage acceptance
evidence remains to be run against the exact review commit; no live result is
claimed by this checkpoint.

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

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

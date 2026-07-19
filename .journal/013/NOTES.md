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

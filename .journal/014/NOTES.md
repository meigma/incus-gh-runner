---
id: 014
title: Review builder attestation architecture
started: 2026-07-20
---

## 2026-07-20 12:15 — Kickoff
Goal for the session: Review the proposed cross-repository attestation model for proving that builder images originate from authorized GitHub workflows on enrolled physical infrastructure through ephemeral incus-gh-runner VMs, then are admitted by simplestreams-s3.
Current state of the world: incus-gh-runner already owns ephemeral VM lifecycle, exact Incus ownership metadata, hardened host-side operation, and signed release artifacts; simplestreams-s3 already publishes immutable image artifacts and has supply-chain verification building blocks, but the proposed build-session protocol, host attester, admission policy, and verification-summary loop are architectural exploration outside the current security-review slice.
Plan: Check the handoff against the concrete repository architecture and trust boundaries, identify sound parts and missing security/protocol details, and recommend the smallest prototype that can disprove or validate the model before writing a full design.

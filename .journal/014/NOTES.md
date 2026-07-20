---
id: 014
title: Review builder attestation architecture
started: 2026-07-20
---

## 2026-07-20 12:15 — Kickoff
Goal for the session: Review the proposed cross-repository attestation model for proving that builder images originate from authorized GitHub workflows on enrolled physical infrastructure through ephemeral incus-gh-runner VMs, then are admitted by simplestreams-s3.
Current state of the world: incus-gh-runner already owns ephemeral VM lifecycle, exact Incus ownership metadata, hardened host-side operation, and signed release artifacts; simplestreams-s3 already publishes immutable image artifacts and has supply-chain verification building blocks, but the proposed build-session protocol, host attester, admission policy, and verification-summary loop are architectural exploration outside the current security-review slice.
Plan: Check the handoff against the concrete repository architecture and trust boundaries, identify sound parts and missing security/protocol details, and recommend the smallest prototype that can disprove or validate the model before writing a full design.

## 2026-07-20 12:20 — Architecture review
The proposed trust split is directionally correct: the guest may identify candidate bytes, while only the host control plane may assert the source image, VM and host identity, Incus policy state, GitHub assignment, and freshness. The non-circular A-to-B promotion sequence is also sound.

Repository inspection found useful existing seams. incus-gh-runner already resolves and pins the full predecessor fingerprint, hashes profile configuration/devices, materializes that snapshot directly onto each VM, records correlation and source-image metadata, and observes GitHub JobStarted events containing the assigned runner. Its broader read-only CUE validator covers project, network, ACL, profile, storage, listener, and server-policy state. simplestreams-s3 already inspects both split-image components, derives content identities, uploads immutable objects before one conditional index commit, and can therefore place admission immediately before catalog activation.

The handoff needs five corrections before it becomes an implementable security claim:

1. A hot-standby VM exists before a job is assigned, so its session cannot contain GitHub run identity at creation. Create an unbound infrastructure session, then bind it on JobStarted and cross-check the later OIDC token against the scale-set event (repository and immutable IDs, workflow ref/SHA, source SHA/ref, run ID/attempt, event, runner_environment, and exact job identifier). A live spike must determine the precise relationship between the scale-set JobID and OIDC check_run_id.
2. simplestreams-s3 is not an admission authority while the untrusted job retains raw write access to the active S3 prefix. The guest must have only quarantine/staging upload rights; a trusted admission service or worker must hold the sole permission to write immutable active objects and commit streams/v1/index.json.
3. A bearer build-session credential readable by the untrusted job is exfiltratable and does not strongly bind the request to the VM. Keep it in a trusted root-owned guest broker, use an instance-bound channel or host-initiated pull, and persist one-use/idempotent consumption across controller restarts. The signing key must remain host-side.
4. The current profile digest is not the whole Incus security environment. Sign a canonical runtime-manifest digest and the exact CUE policy/baseline digest, rerun the broader validator immediately before signing, and explicitly preserve its assurance boundary: API configuration is not measured runtime enforcement.
5. A root-owned file key proves possession of an enrolled software identity, not a particular physical machine. Treat this as an initial assurance tier; require a non-exportable enrolled TPM key for physical-host identity and separately add quote/PCR evidence if measured boot is claimed.

The provenance should use an in-toto Statement with the current SLSA provenance/v1 predicate: subjects for the metadata archive, QCOW2 disk, and conceptual Incus image fingerprint (SHA-256 over metadata bytes followed by disk bytes); predecessor A in resolvedDependencies; GitHub-controlled inputs in externalParameters; host/controller/runtime facts in trusted platform fields; and a documented custom buildType. A subsequent admission decision should bind the same subjects to an exact policy URI/digest and verifier identity. Do not claim a SLSA level until the complete builder trust base is assessed.

Recommended agile sequence: first capture one real JobStarted event plus OIDC token and prove the correlations; next generate a local host-signed DSSE statement without publishing; then prove quarantine-to-trusted-activation in simplestreams-s3; then issue and consume a signed admission summary by exact fingerprint; only after those seams work add TPM identity, measured boot, SBOM/vulnerability policy, revocation, and fleet enrollment.

## 2026-07-20 12:22 — Subject-binding correction
The handoff's final key rule is too permissive: an untrusted guest that merely reports a digest can obtain provenance for bytes the trusted platform never observed, then supply matching external bytes at publication. The guest may nominate outputs, but a trusted component must independently hash the exact metadata and disk bytes before the host signs, and the admission service must recompute the component digests plus the Incus combined fingerprint before activation. A root-owned helper in trusted predecessor image A, backed by an immutable staging copy or host pull, is sufficient for the first proof; direct trust in job-reported digests is not.

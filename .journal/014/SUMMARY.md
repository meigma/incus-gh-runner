---
id: 014
title: Review builder attestation architecture
date: 2026-07-20
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 006, 009]
---

## Goal

Evaluate the proposed cross-repository builder-attestation model, reduce it to
an operationally reasonable trust claim for small Incus fleets, and produce a
focused design and phased plan supporting both file-backed and TPM-bound keys
plus proof delivery to jobs.

## Outcome

The goal was met. The initial artifact-attestation proposal was narrowed to a
job-bound machine receipt: the enrolled controller records that it provisioned
one exact GitHub JIT runner registration into one owned Incus VM and that
GitHub later reported a job start for that same registration. The controller
does not inspect or sign job output.

Two reviewed handoff artifacts are the primary outputs of this session:

- `JOB_MACHINE_PROOF_DESIGN.md` defines the claim, trust boundary, signed
  payload, JIT-to-VM binding, guest delivery contract, verifier, failure model,
  file-backed deployment, TPM-bound systemd deployment, and exact dependencies.
- `JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md` turns that design into five gated
  phases, beginning with locally verifiable proof primitives and a thin
  host-to-VM experiment before live GitHub or TPM integration.

No implementation branch or pull request was created. The documents are a
reviewed proposal and approval-gated plan for a future implementation session.

## Key Decisions

- Attest job placement, not artifact bytes -> this preserves the useful machine
  provenance claim without making the controller an artifact observer,
  publisher, or admission service.
- Bind `JobStarted` to GitHub's exact JIT runner reference -> the controller
  retains the JIT runner ID, name, and scale-set ID, saves them conditionally on
  the stopped VM with an Incus ETag, and verifies them before signing.
- Use host-push delivery instead of a guest request protocol -> the controller
  writes a DSSE envelope and ready marker through the Incus agent; no guest
  credential, OIDC exchange, or host signing endpoint is needed.
- Trust the approved guest bootstrap not to relay the bearer JIT configuration
  -> GitHub does not independently observe the VM or physical host, so this is
  an explicit placement assumption rather than a hidden guarantee.
- Use one Ed25519/DSSE proof format in both storage modes -> file-backed
  `LoadCredential=` and TPM-bound `LoadCredentialEncrypted=` expose the same
  PKCS#8 key file to unchanged controller code.
- Call the first TPM tier TPM-bound storage -> systemd binds encrypted key
  material to the TPM, but the plaintext software key exists at runtime and the
  receipt does not prove TPM use, PCR state, or measured boot to a verifier.
- Ship a policy-neutral verifier -> `incus-gh-runner proof verify` checks the
  enrolled public key and expected host ID, while repository, workflow, image,
  and freshness policy remains outside the controller.
- Learn through five review gates -> prove the format and simple guest channel
  before investing in GitHub correlation, live operation, or TPM provisioning.

## Changes

- `.journal/014/JOB_MACHINE_PROOF_DESIGN.md` - reviewed normative design and
  primary architectural handoff for job-bound machine provenance.
- `.journal/014/JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md` - five-phase,
  success-criteria-driven implementation plan; future work starts here only
  after explicit approval.
- `.journal/014/NOTES.md` - architecture exploration, threat-model narrowing,
  TPM tier analysis, research results, and review corrections.
- `.journal/TECH_NOTES.md` - durable pointer and scope guard for future proof
  implementation work.

## Open Threads

- Implementation has not started. A future session should begin with phase 1
  of `JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md` and pause at every review gate.
- Version 1 intentionally accepts an availability gap: `actions/scaleset`
  acknowledges before `HandleJobStarted`, so a controller crash can leave a
  proof-required job to fail by helper timeout.
- GitHub job-container access to the VM's proof path is deferred.
- Artifact provenance/admission, guest OIDC, TPM-native non-exportable signing,
  measured boot, fleet PKI, and guaranteed durable event delivery remain out of
  scope unless a later requirement justifies their operational cost.

## Lessons

- Persisting the validated JIT reference before starting the VM is the simplest
  direct job-to-machine correlation; terminal cleanup must fence the GitHub
  registration before deleting the VM to cover provisioning crashes.
- TPM instructions need testing against the minimum systemd version: systemd
  250 supports TPM-bound credentials, but older supported hosts need explicit
  credential-directory creation and name-aware cross-host decrypt tests.
- A committed proof file must be immutable after its ready marker; duplicate
  events compare the signed job/runner/UUID tuple instead of rewriting it.

## References

- `.journal/014/JOB_MACHINE_PROOF_DESIGN.md` - primary design artifact.
- `.journal/014/JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md` - primary execution
  handoff and phase gates.
- Journal design checkpoint `62b355c` (`docs(journal): design job machine proofs`).
- `.journal/001/V1_IMPLEMENTATION_PLAN.md` - original controller roadmap and
  architectural context.
- `.journal/006/SUMMARY.md` - real GitHub scale-set/JIT lifecycle context.
- `.journal/009/SUMMARY.md` - hardened systemd and credential boundary context.
- [DSSE protocol](https://github.com/secure-systems-lab/dsse/blob/master/protocol.md)
- [systemd credentials](https://systemd.io/CREDENTIALS/)
- [`actions/scaleset` v0.4.0 message types](https://github.com/actions/scaleset/blob/v0.4.0/types.go)

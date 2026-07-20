# Job-bound machine provenance proof implementation plan

Status: reviewed proposal
Date: 2026-07-20
Design: `JOB_MACHINE_PROOF_DESIGN.md`

## Delivery approach

Build this in five reviewable phases. Each phase is one logical chunk, leaves observable evidence, and pauses for review. The first end-to-end experiment deliberately pushes a test envelope into a VM before GitHub event handling or TPM work is added. If that simple channel is poor, revise or discard it early.

This document is a plan, not approval to begin implementation.

## Fixed technical choices

- Envelope: DSSE with payload type `application/vnd.meigma.incus-gh-runner.job-provenance.v1+json`.
- Signature: one Ed25519 signature over a versioned JSON payload.
- New direct module: `github.com/secure-systems-lab/go-securesystemslib v0.11.0`, using `dsse.NewEnvelopeSigner`, `SignPayload`, `NewEnvelopeVerifier`, and `VerifyAndDecode`.
- Existing modules retained: `github.com/actions/scaleset v0.4.0` and `github.com/lxc/incus/v7 v7.2.0`.
- Standard library: Go 1.26.5 `crypto/ed25519`, `crypto/x509`, `encoding/pem`, `crypto/sha256`, and `encoding/json`.
- Key encoding: PKCS#8 PEM private key and SubjectPublicKeyInfo PEM public key.
- Key ID: `sha256:<lowercase hex>` over public-key DER, calculated locally rather than with `dsse.SHA256KeyID`.
- GitHub identity: authenticated `JobStarted` plus the validated JIT runner reference; no OIDC or REST lookup.
- Machine identity: host-side Incus API, pinned runtime identity, and controller-owned instance metadata.
- Guest channel: Incus agent file push to a fixed proof path, followed by a ready marker.
- Verifier: shipped `incus-gh-runner proof verify --public-key <path> --expected-host-id <id> <proof>` command that prints only verified payload JSON.
- TPM: systemd 250+ `LoadCredentialEncrypted=` and `systemd-creds encrypt --with-key=tpm2`; no Go TPM module and no PCR binding.

The pins and APIs above were verified current on 2026-07-20. At implementation start, run `go list -m -json <module>@latest` for the three Go modules. If anything changed, review the release notes and deliberately retain or update the pin; never float a dependency within a phase.

## Phase 1 — Proof format, signer, and verifier

### Purpose

Create and independently verify a complete receipt without touching GitHub, Incus, or the runner image.

### Work

- Add `internal/provenance` version 1 payload types, strict field validation, 64 KiB envelope/payload limits, and DSSE handling.
- Add the fixed launch-input type and golden JSON/digest vector described by the design.
- Preserve the existing profile-digest byte format and add its own golden vector.
- Adapt parsed Ed25519 keys to the DSSE signer and verifier interfaces.
- Add optional `job_proof.host_id` and `job_proof.signing_key_file` settings. Both empty disables proofs; only one set is invalid.
- At startup, accept only a regular file no larger than 16 KiB containing exactly one PKCS#8 Ed25519 key. Read it once and use secret-safe errors.
- Ship `incus-gh-runner proof verify`. It must run without controller configuration, verify the enrolled public key and expected host ID, and emit only the verified payload.
- Document OpenSSL key generation, public-key enrollment, and overlap-based rotation. Do not introduce a key-management service.

### Success criteria

- A fixed event and machine snapshot produce a valid DSSE envelope with the exact payload type, claim, and key ID.
- A fixed key-ID vector independently hashes SubjectPublicKeyInfo DER and prevents accidental use of the library's differently formatted helper.
- The verifier rejects a changed payload or signature, wrong key, wrong host ID, wrong payload type, extra signature, oversized input, invalid IDs, and unknown payload fields.
- Missing, malformed, multi-key, and non-Ed25519 private inputs fail without printing key bytes; source ownership and mode remain deployment checks.
- The verifier needs only its explicit proof, public-key, and host-ID arguments and never loads the controller signing key.
- Existing configurations behave exactly as before when `job_proof` is absent.
- Unit tests, lint, and dependency/security checks pass with only `go-securesystemslib v0.11.0` added directly.

### Review gate

Review the readable payload, exact claim, verifier interface, dependency change, and disabled-by-default behavior.

## Phase 2 — Thin host-to-VM delivery slice

### Purpose

Prove the guest path and permission model with a prebuilt test envelope before adding event correlation or concurrency.

### Work

- Add `/run/incus-gh-runner-proof` and the wait/copy helper to the reference image without changing the root-only JIT directory.
- Add a narrow Incus proof sink that receives an expected instance name and UUID plus an already signed envelope.
- Recheck owner, running state, and UUID before the proof write and before the ready marker.
- Write the proof and marker as root-owned mode `0444`, with the marker as the commit point.
- If a marker exists, verify the committed proof and make an identical job/runner/UUID tuple a no-op; reject every other overwrite.
- Add one functional-test harness that injects a phase 1 envelope into a reference VM and retrieves it through the helper. Do not expose this test hook as a production API.

### Success criteria

- An unprivileged runner user retrieves the test proof through `incus-gh-runner-proof --output <path>` and the copied file is mode `0600`.
- The existing JIT payload and status paths retain their current root-only permissions.
- Tests prove proof-before-marker ordering, both identity rechecks, no marker after a failed proof write, and refusal of a replacement instance.
- A duplicate after readiness does not rewrite either committed file; a different tuple is refused.
- Timeout, malformed committed proof, helper interruption, and a VM disappearing between writes all fail deterministically.
- A real Incus functional test records the guest-agent delivery time and completes within the existing bounded adapter timeout.

### Review gate

Decide whether the fixed path, marker protocol, helper, and permissions are simple and reliable enough to keep. Revise or discard this slice before building more around it.

## Phase 3 — Bind a GitHub job to its VM

### Purpose

Connect one authenticated job-start event to the exact JIT registration and VM, then use the proven delivery slice.

### Work

- Change the JIT payload port to preserve the validated `RunnerScaleSetJitRunnerConfig.Runner` ID, name, and scale-set ID instead of discarding it.
- Put the launch digest in the initial stopped-VM request. After obtaining the JIT response, re-fetch the owned VM and conditionally save the reference with `UpdateInstance(name, api.InstancePut, etag)`. Wait for completion and re-fetch the same UUID and values before VM start or JIT delivery.
- Refuse JIT delivery if the response does not match the requested name and resolved scale set. On any later provisioning failure, invoke the existing scale-set fence for the unused registration before normal VM cleanup.
- Change terminal owned-runner cleanup to fence the runner name before Incus deletion. Treat an absent registration as success so restart recovery closes the JIT-allocation-to-delivery crash window without a new database.
- Implement the exact version 1 launch digest. Reject reserved profile keys and re-create the digest from the current instance before signing.
- Copy the required `JobStarted` fields into an internal event, attach only the controller's resolved scale-set context, and enforce positive-ID and length checks in the callback. Perform exact-registration and VM checks later in the coordinator.
- Keep the existing busy-runner update. Try a nonblocking enqueue into a queue of `max(1, capacity.max_runners)`; on overflow, log the non-secret IDs, drop only the proof event, and return success to the already-acknowledged listener.
- Add a supervised coordinator for snapshot verification, signing, and the phase 2 sink. Generalize the application supervisor for the third component rather than starting an unowned goroutine.
- Treat a committed duplicate as a no-op. Abort if instance deletion or terminal state wins before marker commit; do not add a lifecycle lease or event database.

### Success criteria

- Tests prove a valid JIT runner reference is conditionally saved and re-read before VM start or guest delivery. An empty, mismatched, wrong-scale-set, stale-ETag, replaced-instance, or failed-update result prevents both.
- A failure after JIT allocation fences the unused registration; a fencing failure is reported alongside the provisioning error and never causes the opaque JIT payload to be logged or delivered.
- Crash-point tests after JIT allocation, metadata persistence, VM start, and partial payload delivery prove the next process fences the runner registration before deleting the recovered terminal VM.
- A valid event signs only after runner ID/name, scale-set context, owner, running state, UUID, image, profile list, and launch digest all match.
- A golden vector fixes launch-input bytes and digest; mutation of any included config or device blocks signing, while server `volatile.*` and controller audit keys cannot create circular input.
- The synchronous callback performs no signing or Incus I/O and never blocks when the proof queue is full.
- Queue saturation emits the named structured error, leaves runner demand tracking working, and causes the proof consumer to time out rather than receive invented evidence.
- Tests cover malformed events, unknown or replaced instances, committed duplicates, a second job on one VM, delivery failure, teardown race, shutdown, and secret-safe logs.
- Existing provisioning, standby, job execution, diagnostics, and cleanup tests remain green with proofs disabled.

### Review gate

Review field authority, JIT-to-VM correlation, concurrency, idempotency, and failure behavior. No job-controlled value may choose a signed machine field or filesystem path.

## Phase 4 — Live file-backed proof

### Purpose

Test the complete claim against GitHub and Incus before introducing TPM operations.

### Work

- Install the file-backed proof-key drop-in alongside one existing GitHub credential drop-in; verify the source key is `root:root` mode `0600`.
- Enroll the public key and host ID outside the runner.
- Add a disposable workflow whose first proof-dependent step runs the helper and preserves the receipt as an ordinary workflow artifact.
- Verify it outside the VM with the shipped command, then compare the payload with the expected repository, workflow, run, JIT runner ID/name, scale set, Incus UUID, image fingerprint, profiles, and launch digest.
- Run one job on a hot standby and one on a demand-created runner. Measure `JobStarted` receipt to marker visibility.
- Exercise one controlled delivery failure and confirm a proof-required job stops before publication.

### Success criteria

- Both jobs receive valid receipts whose direct JIT-registration claim matches GitHub and the live Incus instance.
- The review explicitly records the remaining placement assumption: the trusted guest bootstrap could otherwise relay the bearer JIT registration.
- Tampering or verification with another host key or host ID fails.
- A job that does not consume the proof runs without workflow changes; a proof-required job fails clearly when it is unavailable.
- No private key or GitHub credential appears in logs, diagnostics, artifacts, or the guest receipt, and final owned Incus inventory returns to zero.
- Observed timing supports the 60-second helper default or provides evidence for a small adjustment.

### Review gate

Decide from live evidence whether the host-push model is operationally adequate. If not, revise the channel rather than adding TPM or persistence around it.

## Phase 5 — TPM-bound systemd credential

### Purpose

Protect the same software signing key at rest with the host TPM without changing the proof protocol or controller sandbox.

### Work

- Package separate proof-key drop-ins for `LoadCredential=` and `LoadCredentialEncrypted=`. Both expose the same credential name and environment variable and coexist with both GitHub credential choices.
- Extend `deploy/systemd/verify.sh` to cover every relevant drop-in combination, file-backed source ownership/mode, encrypted credential presence, and continued `PrivateDevices=yes`.
- Document explicit creation of `/etc/credstore.encrypted` for older supported systemd releases, a root-only temporary plaintext source, public-key enrollment, the exact encryption command and empty PCR set, source removal or deliberate escrow, service start, rotation, and replacement recovery.
- Use the encryption attempt itself as the portable TPM/systemd capability test. Do not require `systemd-creds has-tpm2`, which is unavailable across the full systemd 250+ range.
- On an enrolled TPM 2.0 host, encrypt the key, start the unchanged controller, reboot normally, and repeat the phase 4 proof and external verification.
- Rotate by trusting the new public key before restarting with a new credential. If a second TPM host exists, first prove origin-host decryption yields the enrolled public key, then run the same `systemd-creds decrypt --name=machine-provenance-key` check on the copied credential there.

### Success criteria

- The controller starts after a normal reboot, signs a receipt, and retains `PrivateDevices=yes` without direct `/dev/tpmrm0` access.
- File-backed and TPM-bound modes produce the same schema, key ID rules, verifier behavior, and workflow experience.
- Rotation keeps old receipts verifiable with the retained old public key.
- Documentation says that TPM clear or motherboard replacement requires a new key and enrollment unless an offline private-key escrow exists, and explains that escrow weakens the TPM-only storage assurance.
- Cross-host decryption fails after the same name-aware command succeeds on the origin host; otherwise cross-host binding remains a clearly recorded evidence gap, not a claimed test result.
- File-backed deployment remains tested and supported.

### Review gate

Confirm the operator instructions call this TPM-bound key storage and do not claim TPM-native signing, a TPM quote, physical-host measurement, or measured boot.

## Final acceptance

The feature is complete only after all five gates pass and a human can answer these questions from operator documentation without reading source code:

- What does the receipt directly claim, and what placement assumption remains?
- How does a workflow obtain a proof and fail when it is missing?
- How does a consumer verify a proof and apply repository or workflow policy?
- How are the host public key and host ID enrolled and rotated?
- How do file-backed and TPM-bound deployments differ?
- What happens across controller restart, queue overflow, TPM replacement, and normal host updates?

The final scope review must confirm that artifact signing, a guest request API, direct TPM access, measured boot, a durable event database, job-container support, and cross-repository admission policy did not enter the implementation.

## Known limitation carried intentionally

`actions/scaleset v0.4.0` acknowledges a message before invoking `HandleJobStarted`. A crash before marker delivery can therefore leave a job without a proof. Version 1 detects that only through the workflow helper timeout and fails proof-dependent work closed. Guaranteed delivery would require acknowledgement control and durable event storage; it is an explicit future decision, not hidden work in these phases.

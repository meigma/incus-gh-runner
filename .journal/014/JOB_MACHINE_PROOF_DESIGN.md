# Job-bound machine provenance proof

Status: reviewed proposal
Date: 2026-07-20

## Outcome

When enabled, `incus-gh-runner` gives each started GitHub Actions job a signed machine receipt. The receipt directly proves that an enrolled controller provisioned one exact GitHub JIT runner registration into one owned Incus VM and that GitHub later reported a job start for that same registration.

A workflow that needs machine provenance can copy the receipt from a fixed path in the VM and pass it to its publication or verification step. The controller does not inspect or sign build output. The receipt says how a job was placed; it says nothing about the bytes the job produced or whether the job succeeded.

The same controller code supports two key-storage choices:

- a normal private-key file loaded with systemd `LoadCredential=`; or
- that private key encrypted to the host TPM and loaded with `LoadCredentialEncrypted=`.

In both cases systemd presents a protected runtime file to the controller. TPM support therefore strengthens storage without adding TPM protocol code or direct TPM access to `incus-gh-runner`.

## Exact claim

The signed claim is:

> The enrolled host controller provisioned this GitHub JIT runner registration into this owned Incus VM using the recorded image and launch inputs, and GitHub later reported that this job started for the same registration on the controller's authenticated scale-set session.

Treating that as evidence that the job actually ran in the VM also relies on the approved guest image and bootstrap code not copying the bearer JIT registration to another machine. GitHub does not independently observe the Incus VM or physical host.

The proof does not claim:

- that the job completed or succeeded;
- that an artifact came from the VM;
- that the workflow or its dependencies were harmless;
- that the job read or preserved the proof;
- that the host boot state was measured;
- that a file-backed key is physically tied to one host; or
- that a verifier can tell which storage mode was used from the signature.

GitHub job authorization remains an operator responsibility. Repository scope and runner-group controls determine which workflows may reach the scale set. The controller will not duplicate those controls with another workflow allowlist in version 1. A consumer can require an exact repository and workflow reference from the verified payload.

## Trust boundary

The trusted side is the physical host, systemd, the controller process, the local Incus daemon, the approved guest image and JIT bootstrap, and the operator's GitHub runner-access policy. The workflow may delete or corrupt its copy of a proof, but it cannot forge a proof that passes external signature verification.

Signed fields have these authorities:

| Fields | Trusted source |
|---|---|
| Host ID | Local controller configuration and enrolled public-key record |
| Controller version and commit | Running controller binary |
| Issued time | Controller host clock at signing |
| Repository, workflow, run, job, request, runner, and event | Authenticated GitHub `JobStarted` message |
| Scale-set ID and name | Controller's already resolved scale-set session, not `JobStarted` |
| JIT runner ID, name, and scale-set ID | Validated GitHub JIT configuration response saved before guest delivery |
| Incus project and requested instance name | Controller configuration and generated allocation, confirmed through Incus |
| VM UUID, state, image, profiles, and launch inputs | Host-side Incus API plus controller-owned instance metadata |

With a file-backed key, enrollment identifies the installation holding that key; copying the private key copies its identity. A TPM-bound systemd credential makes an offline copy of the encrypted credential unusable on another TPM. The plaintext key still exists in systemd's credential store, the controller's address space, and controller memory while the service runs. This is TPM-bound key storage, not a TPM-resident signing key or remote TPM attestation.

## Lifecycle

1. Preflight resolves the full image fingerprint and materializes the approved Incus profile configuration. The controller calculates the version 1 launch digest from those pre-metadata inputs.
2. The controller creates an owned, stopped VM with the launch digest in its initial audit metadata. It then asks GitHub for a JIT configuration and retains the returned runner reference rather than keeping only the opaque JIT payload.
3. The controller requires a positive runner ID, the requested runner name, and the resolved scale-set ID. It re-fetches the same owned VM and uses the Incus ETag to conditionally add that JIT reference to its metadata. It waits for the update and re-fetches the same UUID and values before starting the VM.
4. Only then does the controller start the VM and send the opaque JIT payload. The approved guest bootstrap consumes it and connects the runner. GitHub assigns a job and sends `JobStarted` on the authenticated scale-set session.
5. The synchronous callback keeps the existing busy-runner update, validates field shape and bounds, and tries to copy the event into a bounded queue. It performs no signing or Incus I/O.
6. A proof coordinator re-fetches the instance, compares the event's runner ID and name with the saved JIT reference, and verifies its exact owner marker, running state, server-generated `volatile.uuid`, image, profiles, and launch digest before signing.
7. The Incus adapter rechecks the same owner and UUID while delivering `job-proof.dsse.json`, then writes `job-proof.ready` last.
8. A workflow that requires provenance runs the guest helper. The helper waits for readiness with a bounded timeout and copies the envelope to a caller-selected path.

Proof delivery is attempted for every valid started job while the feature is enabled. Opt-in means consuming the proof; there is no guest-to-host request API.

The Incus adapter adds the existing SDK's conditional `UpdateInstance(name, api.InstancePut, etag)` call and waits for its operation. An ETag conflict, identity mismatch, or failed re-read prevents VM start and JIT delivery. Any ordinary failure after GitHub creates the JIT reference also invokes the existing scale-set fence for that runner name. To cover a controller crash in this window, every terminal owned-VM deletion fences the same runner name before deleting the VM; an already absent registration is success. Fencing and VM cleanup errors are reported without exposing the opaque JIT data.

## Proof payload

The payload is a small, versioned JSON object. Field names below are normative for version 1; values are illustrative.

```json
{
  "version": 1,
  "claim": "github_job_started_for_jit_runner_provisioned_to_incus_vm",
  "issued_at": "2026-07-20T20:15:32.123456Z",
  "host": {
    "id": "builder-host-01",
    "controller_version": "1.1.0",
    "controller_commit": "0123456789abcdef"
  },
  "github": {
    "owner": "meigma",
    "repository": "builder-images",
    "workflow_ref": "meigma/builder-images/.github/workflows/build.yml@refs/heads/main",
    "workflow_run_id": 123456789,
    "job_id": "01234567-89ab-cdef-0123-456789abcdef",
    "runner_request_id": 123456,
    "runner_id": 7890,
    "runner_name": "incus-gh-runner-01234567-89ab-cdef-0123-456789abcdef",
    "event_name": "workflow_dispatch",
    "scale_set_id": 42,
    "scale_set_name": "incus-linux-x64"
  },
  "machine": {
    "incus_project": "github-runners",
    "instance_name": "incus-gh-runner-01234567-89ab-cdef-0123-456789abcdef",
    "instance_uuid": "fedcba98-7654-3210-fedc-ba9876543210",
    "image_fingerprint": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "launch_configuration_sha256": "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
    "profiles": [
      {
        "name": "github-runner",
        "sha256": "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0"
      }
    ]
  }
}
```

Mandatory string identity fields must be non-empty and no longer than 4 KiB. Workflow run, runner request, runner, and scale-set IDs must be positive. The event runner name must equal the instance name and the saved JIT runner name; its runner ID must equal the saved JIT runner ID. The saved JIT scale-set ID and the callback's scale-set context must both equal the controller's resolved scale set. An invalid or inconsistent event is logged and receives no proof.

`job_id` and `workflow_ref` remain opaque GitHub values. Version 1 does not assume that `job_id` equals an OIDC `check_run_id`, and it does not add a GitHub REST lookup merely to enrich the receipt.

There is no expiry because the proof is durable evidence, not bearer authorization. Consumers may impose freshness using `issued_at`, but that timestamp trusts the enrolled host's clock; version 1 has no external timestamp authority. A replay still names only its original run, job, registration, and VM, so a consumer must compare those fields with the operation it is evaluating.

## Exact launch digest

`launch_configuration_sha256` is the lowercase SHA-256 hex digest of one `encoding/json.Marshal` result with these fields in this order:

```json
{
  "version": 1,
  "instance_type": "virtual-machine",
  "image_fingerprint": "<full SHA-256 fingerprint>",
  "profiles": [{"name": "<name>", "sha256": "<digest>"}],
  "config": {"<key>": "<value>"},
  "devices": {"<device>": {"<key>": "<value>"}}
}
```

The ordered profile list, config, and devices come from the pinned runtime identity before controller audit metadata is added to the instance request. Config and device maps are always initialized, so empty values encode as `{}` rather than `null`. Go's standard JSON encoder provides the byte encoding: no indentation or trailing newline, with map keys sorted as documented by `encoding/json`. A golden test vector fixes the exact bytes and digest.

Each `profiles[].sha256` retains the repository's existing profile digest: lowercase SHA-256 hex over `encoding/json.Marshal` of a two-field struct in the order `{"config": <profile config>, "devices": <profile devices>}`. There is no indentation or trailing newline; map keys are sorted. No extra normalization occurs, so a nil map encodes as `null` and an initialized empty map as `{}`. A separate golden vector fixes this existing format.

Preflight rejects profile config keys beginning with `volatile.` or `user.incus-gh-runner.` because those namespaces are reserved for Incus and controller state. Before signing, the coordinator reconstructs the same input from the current instance, excluding server-added `volatile.*` and controller-owned `user.incus-gh-runner.*` keys, and compares the result with the saved digest and pinned identity. The digest field itself is added only afterward and can never be part of its own input.

This digest describes the VM launch inputs. It does not describe host-wide Incus server settings, networking, storage, firmware, or a CUE deployment baseline.

## Signature and verification

The JSON payload is wrapped in a DSSE envelope with payload type:

```text
application/vnd.meigma.incus-gh-runner.job-provenance.v1+json
```

DSSE signs both the payload type and the exact payload bytes, so no custom signature format or JSON canonicalizer is needed. Version 1 permits exactly one Ed25519 signature.

The key ID is `sha256:<lowercase hex>` over the DER SubjectPublicKeyInfo form of the Ed25519 public key. A local adapter calculates this value; it must not use `dsse.SHA256KeyID`, whose OpenSSH-style representation is different. A fixed test vector protects the format.

The shipped policy-neutral verifier is:

```text
incus-gh-runner proof verify --public-key <path> --expected-host-id <id> <proof>
```

It loads no controller configuration or signing credential. On success it writes only the verified payload JSON to standard output. Repository, workflow, image, and freshness policy remain the caller's responsibility.

The verifier must:

- limit the envelope and decoded payload to 64 KiB each;
- require the exact payload type and exactly one signature;
- derive the selected public key's key ID and require the envelope hint to match;
- verify the DSSE signature before parsing or trusting payload fields;
- decode the verified payload once with unknown fields rejected;
- require the expected schema version, claim, and host ID; and
- fail non-zero without printing an unverified payload.

Artifact digests are intentionally absent. A trusted workflow may publish this proof beside an artifact or include it in separate artifact provenance, but the controller makes no cryptographic claim about that artifact.

## Key enrollment and rotation

The private key is PKCS#8 PEM containing exactly one Ed25519 key. The public key is SubjectPublicKeyInfo PEM. A simple generation flow is:

```sh
umask 077
openssl genpkey -algorithm Ed25519 -out machine-provenance-key.pem
openssl pkey -in machine-provenance-key.pem -pubout -out machine-provenance-key.pub.pem
```

Consumer enrollment is an operator-managed record containing the stable host ID, public key, and derived key ID. The key ID is a lookup hint, not an identity by itself. A verifier uses the enrolled public key and requires the signed host ID to match its enrollment record.

Rotation first distributes the new public key and host mapping, then restarts the controller with the new private credential. Consumers retain the old public key for as long as old proofs must remain verifiable. Automatic key distribution and fleet PKI are outside version 1.

## Controller configuration and systemd

The controller adds one optional configuration section:

```yaml
job_proof:
  host_id: builder-host-01
  signing_key_file: /run/credentials/incus-gh-runner.service/machine-provenance-key
```

Both empty disables the feature; supplying only one is a startup error. Avoiding a separate `enabled` Boolean keeps configuration unambiguous and fits the current exact YAML validator.

The controller accepts a bounded regular file containing the one valid Ed25519 key, reads it once at startup, and never logs it. Source-file ownership and mode are deployment checks because systemd creates the runtime credential file. Key reload is not supported.

File-backed deployment uses a proof-key drop-in that can coexist with either GitHub credential drop-in:

```ini
[Service]
LoadCredential=machine-provenance-key:/etc/incus-gh-runner/machine-provenance-key.pem
Environment=INCUS_GH_RUNNER_JOB_PROOF_SIGNING_KEY_FILE=%d/machine-provenance-key
```

Install the source private key as `root:root` mode `0600`.

TPM-bound deployment changes only the loader:

```ini
[Service]
LoadCredentialEncrypted=machine-provenance-key:/etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred
Environment=INCUS_GH_RUNNER_JOB_PROOF_SIGNING_KEY_FILE=%d/machine-provenance-key
```

On the target host, systemd 250 or newer encrypts the same software key to that host's TPM:

```sh
sudo install -d -o root -g root -m 0700 /etc/credstore.encrypted
sudo systemd-creds encrypt \
  --name=machine-provenance-key \
  --with-key=tpm2 \
  --tpm2-device=auto \
  --tpm2-pcrs= \
  /run/incus-gh-runner-machine-provenance-key.pem \
  /etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred
sudo chmod 0600 /etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred
```

Creating `/etc/credstore.encrypted` explicitly is required on older supported systems; systemd did not ship its automatic tmpfiles entry until later. The plaintext input should be created on a root-only temporary filesystem, used to derive and enroll the public key, then removed after successful encryption unless the operator deliberately keeps an offline recovery escrow. The encryption command itself is the portable capability check across the supported systemd range; instructions must not depend on a `has-tpm2` verb that is absent from systemd 250.

The explicit empty PCR set avoids routine firmware, kernel, or bootloader updates locking out the service. TPM clearing or motherboard replacement makes the encrypted credential unusable. Without an escrowed private key, recovery means generating a new key and re-enrolling the host. With escrow, the same key can be sealed again, but TPM-only storage becomes an operator assurance; neither public-key enrollment nor the receipt can prove that no recovery copy exists.

`PrivateDevices=yes` remains enabled. systemd's service-launch credential helper decrypts the credential before applying the service's device namespace, then exposes only the protected plaintext credential file to the dynamic service user. The controller itself never opens a TPM device.

A cross-host check must first decrypt successfully on the origin host and confirm the derived public key matches enrollment, then fail on the second host. Both manual decrypt commands must include `--name=machine-provenance-key`; without it, the deliberately different ciphertext filename causes a name mismatch that says nothing about TPM binding. Temporary plaintext test outputs stay root-only and are removed immediately.

## Guest delivery contract

The reference image creates `/run/incus-gh-runner-proof` as `root:root` mode `0755`. This is separate from the existing root-only `/run/incus-gh-runner`, so JIT payload and status permissions remain unchanged.

The controller writes these root-owned, read-only files:

- `/run/incus-gh-runner-proof/job-proof.dsse.json`, mode `0444`;
- `/run/incus-gh-runner-proof/job-proof.ready`, mode `0444`.

The sink receives the expected instance UUID. It verifies owner, running state, and UUID immediately before the proof write and again before the marker write. The marker is the commit point.

If the marker already exists, the sink reads and verifies the committed proof. An exact match on `job_id`, `runner_id`, and `instance_uuid` is a no-op. Any other value is refused. A committed proof is never rewritten. If the marker is absent, an earlier uncommitted proof may be replaced before the marker is created.

The reference image supplies:

```text
incus-gh-runner-proof --output <path> [--timeout 60s]
```

It waits for the marker, checks that the proof is a non-empty DSSE-shaped JSON document, and copies it to the requested path with mode `0600`. It does not verify the signature; external verification uses the enrolled host key. Timeout or a missing proof exits non-zero, so a proof-required workflow fails closed. Guest root can cause denial of service by deleting the files but cannot forge a verifiable envelope.

Custom images must implement the same directory and helper contract. GitHub job containers cannot see the VM's `/run` path in version 1; container mounts or runner hooks are separate work.

## Failure behavior

- A missing or invalid signing key fails startup only when proofs are configured.
- A failed conditional JIT-metadata update never starts the VM or exposes the JIT payload. Normal failure and restart cleanup both fence the unused GitHub registration before deleting the VM.
- The queue capacity is `max(1, capacity.max_runners)`. Enqueue never blocks the scale-set callback.
- On a full queue, the callback writes a structured error with the non-secret job and runner IDs, drops that proof event, and returns success so the already-acknowledged listener session and runner lifecycle continue. A proof-required job then fails by helper timeout.
- Malformed events, an unknown registration, ownership or UUID mismatch, a non-running VM, launch drift, or inconsistent metadata produce no proof.
- Delivery is bounded. If the VM exits or deletion wins the race before the marker commit, delivery stops and the helper times out; version 1 adds no lifecycle lease.
- Proof errors do not stop unrelated runner cleanup and never expose private keys or JIT content in logs.

The pinned `actions/scaleset` listener deletes a GitHub queue message before calling `HandleJobStarted`. A controller crash in that interval, or before marker delivery, can leave a running job without a proof and the event may not replay. Version 1 treats this as an availability failure: the helper times out and proof-dependent publication must stop. The controller never reconstructs a proof from ambiguous state after restart.

Guaranteed delivery would require control of acknowledgement order and durable event storage. That is intentionally deferred until live use proves the added operational complexity is justified.

## Code boundaries

- `internal/provenance` owns the payload, launch-input schema, strict validation, DSSE signer/verifier, coordinator, and narrow machine-source and proof-sink ports.
- `internal/adapters/github` preserves validated JIT runner references and copies valid `JobStarted` fields without signing or Incus I/O.
- `internal/adapters/incus` conditionally stores controller metadata with an ETag, verifies the current owned instance, and delivers proof then marker through the existing Incus agent file API.
- `internal/runtime` loads the key and wires the optional coordinator.
- `internal/app` supervises the coordinator alongside the existing demand source and lifecycle controller.
- The reference image owns only the public proof directory and wait/copy helper.

This keeps proof construction separate from GitHub, Incus, filesystem, and systemd adapters. Consumer admission policy does not enter the controller.

## Resolved dependencies

Versions and named APIs were checked against current module metadata and primary documentation on 2026-07-20.

| Dependency | Pinned version | Exact use |
|---|---:|---|
| [`github.com/secure-systems-lab/go-securesystemslib`](https://pkg.go.dev/github.com/secure-systems-lab/go-securesystemslib/dsse) | `v0.11.0` | New direct dependency. Use `dsse.NewEnvelopeSigner`, `SignPayload`, `NewEnvelopeVerifier`, and `VerifyAndDecode`. A local Ed25519 adapter implements the signer and verifier interfaces and the specified key ID. |
| [`github.com/actions/scaleset`](https://github.com/actions/scaleset/blob/v0.4.0/types.go) | `v0.4.0` | Existing dependency. Preserve `RunnerScaleSetJitRunnerConfig.Runner`; consume `JobStarted`; retain this exact pin. |
| [`github.com/lxc/incus/v7`](https://pkg.go.dev/github.com/lxc/incus/v7/client) | `v7.2.0` | Existing dependency. Use conditional `UpdateInstance` with the fetched ETag for JIT metadata and `CreateInstanceFile` for ordered proof and marker delivery; retain this exact pin. |
| Go standard library | Go `1.26.5` | `crypto/ed25519`, `crypto/x509`, `encoding/pem`, `crypto/sha256`, `encoding/json`, and bounded file handling. |
| [systemd credentials](https://systemd.io/CREDENTIALS/) | systemd `250+` for TPM mode | `systemd-creds encrypt`, `LoadCredential=`, and `LoadCredentialEncrypted=` provide both storage modes. TPM encryption flags were added in systemd 250. |

No OIDC library is needed because the job does not request a proof and the authenticated `JobStarted` message supplies the job identity in this claim. No Go TPM library is needed because systemd performs TPM binding and decryption. In-toto, Sigstore/Cosign, JWS, and canonical-JSON libraries are also excluded: the receipt has no artifact subject, and DSSE authenticates the exact bytes and payload type.

Primary references:

- [DSSE protocol](https://github.com/secure-systems-lab/dsse/blob/master/protocol.md)
- [GitHub scale-set message and JIT fields](https://github.com/actions/scaleset/blob/v0.4.0/types.go)
- [`actions/scaleset` acknowledgement order](https://github.com/actions/scaleset/blob/v0.4.0/listener/listener.go#L207-L225)
- [systemd credential overview](https://systemd.io/CREDENTIALS/)
- [`systemd-creds` TPM options](https://man7.org/linux/man-pages/man1/systemd-creds.1.html)
- [`LoadCredentialEncrypted=`](https://man7.org/linux/man-pages/man5/systemd.exec.5.html)

## Deferred work

Version 1 deliberately excludes:

- artifact digests or SLSA provenance;
- guest OIDC tokens or a guest-to-host request service;
- guaranteed delivery across a controller crash;
- a lifecycle lease solely for proof delivery;
- job-completion receipts;
- GitHub job-container delivery;
- TPM-resident, non-exportable signing keys;
- TPM endorsement or attestation-key enrollment;
- PCR quotes, measured boot, transparency logs, or fleet PKI; and
- automatic public-key distribution or cross-repository admission policy.

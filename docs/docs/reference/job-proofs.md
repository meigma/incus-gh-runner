# Job Proofs Reference

A job machine proof is a signed receipt binding one authenticated GitHub Actions job-start event to the Incus VM provisioned for it. This page describes the version 1 proof wire format: the DSSE envelope, the payload schema, the fixed constants and size limits, the key-ID rule, and the host-key enrollment facts a verifier depends on.

Guest-side delivery — the staging directory, ready marker, and the `incus-gh-runner-proof` helper — is described in the [guest contract reference](guest-contract.md#filesystem-contract). The `job_proof` configuration keys and the `proof verify` command are described in the [configuration reference](configuration.md#job_proof). The procedure for generating, enrolling, installing, and rotating the proof key is in [Deploy to production](../how-to/deploy.md#6-enable-job-proofs-optional).

## Constants

| Constant | Value |
|---|---|
| Schema version | `1` |
| Claim | `github_job_started_for_jit_runner_provisioned_to_incus_vm` |
| DSSE payload type | `application/vnd.meigma.incus-gh-runner.job-provenance.v1+json` |
| Instance type | `virtual-machine` — the fixed Incus instance type covered by a version 1 proof |

## Size limits

| Limit | Value |
|---|---|
| Encoded DSSE envelope | 64 KiB (65536 bytes) |
| Decoded payload | 64 KiB |
| Each mandatory string identity field | 4 KiB |

## DSSE envelope

The proof is one JSON [DSSE](https://github.com/secure-systems-lab/dsse) envelope with the standard `payloadType`, base64-encoded `payload`, and `signatures` fields.

| Field | Constraint |
|---|---|
| `payloadType` | Must equal the fixed DSSE payload type above |
| `payload` | Base64-encoded version 1 payload JSON, decoded size within the payload limit |
| `signatures` | Exactly one signature |
| `signatures[0].keyid` | Must equal the enrolled [key ID](#key-id) |
| `signatures[0].sig` | Raw Ed25519 signature over the DSSE pre-authentication encoding (PAE) of the payload |

The `proof verify` command authenticates an envelope against an enrolled public key and expected host identity; see the [configuration reference](configuration.md#proof-verify-proof).

## Payload schema (version 1)

The payload is a single JSON object. No field is optional: every field listed below is present in every version 1 proof.

```json
{
  "version": 1,
  "claim": "github_job_started_for_jit_runner_provisioned_to_incus_vm",
  "issued_at": "2026-07-21T17:31:04.418306Z",
  "host": {
    "id": "builder-host-01",
    "controller_version": "0.1.0",
    "controller_commit": "0123456789abcdef0123456789abcdef01234567"
  },
  "github": {
    "owner": "OWNER",
    "repository": "REPOSITORY",
    "workflow_ref": "OWNER/REPOSITORY/.github/workflows/build.yml@refs/heads/main",
    "workflow_run_id": 987654321,
    "job_id": "8c1f3a52-6a0e-4d5f-9d2b-1f0e6b7c8d9e",
    "runner_request_id": 1234,
    "runner_id": 42,
    "runner_name": "incus-gh-runner-0f47a1b2c3d4e5f6",
    "event_name": "push",
    "scale_set_id": 7,
    "scale_set_name": "incus-gh-runner-prod"
  },
  "machine": {
    "incus_project": "github-runners",
    "instance_name": "incus-gh-runner-0f47a1b2c3d4e5f6",
    "instance_uuid": "1c9f9f0a-2f3b-4c5d-8e7f-6a5b4c3d2e1f",
    "image_fingerprint": "aa11bb22cc33dd44ee55ff6600112233445566778899aabbccddeeff00112233",
    "launch_configuration_sha256": "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
    "profiles": [
      {
        "name": "github-runner",
        "sha256": "ffeeddccbbaa99887766554433221100ffeeddccbbaa99887766554433221100"
      }
    ]
  }
}
```

All values above are illustrative.

| Field | Type | Constraint |
|---|---|---|
| `version` | integer | Must equal `1` |
| `claim` | string | Must equal the fixed claim constant |
| `issued_at` | string | RFC3339 UTC timestamp; the controller host time at signing; non-zero |
| `host` | object | See [`host`](#host) |
| `github` | object | See [`github`](#github) |
| `machine` | object | See [`machine`](#machine) |

### `host`

Identifies the enrolled controller installation.

| Field | Type | Constraint |
|---|---|---|
| `id` | string | Non-empty; the operator-enrolled stable host identity (`job_proof.host_id`) |
| `controller_version` | string | Non-empty; the running controller release version |
| `controller_commit` | string | Non-empty; the running controller source revision |

### `github`

Identifies the authenticated job-start event and JIT runner registration.

| Field | Type | Constraint |
|---|---|---|
| `owner` | string | Non-empty; repository owner reported by GitHub |
| `repository` | string | Non-empty; repository name reported by GitHub |
| `workflow_ref` | string | Non-empty; opaque workflow reference reported by GitHub |
| `workflow_run_id` | integer | Greater than `0` |
| `job_id` | string | Non-empty; opaque job identifier reported by GitHub |
| `runner_request_id` | integer | `0` or greater; GitHub may report `0` for `JobStarted` messages in live scale-set sessions |
| `runner_id` | integer | Greater than `0`; the JIT runner registration identifier |
| `runner_name` | string | Non-empty; the JIT runner name assigned to the VM; equals `machine.instance_name` |
| `event_name` | string | Non-empty; the workflow event reported by GitHub |
| `scale_set_id` | integer | Greater than `0`; the resolved runner scale-set identifier |
| `scale_set_name` | string | Non-empty; the resolved runner scale-set name |

### `machine`

Identifies the owned Incus VM and its pinned launch inputs.

| Field | Type | Constraint |
|---|---|---|
| `incus_project` | string | Non-empty; the configured Incus project containing the VM |
| `instance_name` | string | Non-empty; the requested and observed Incus instance name; equals `github.runner_name` |
| `instance_uuid` | string | Non-empty; the server-generated Incus instance UUID |
| `image_fingerprint` | string | 64 lowercase hexadecimal characters; the full pinned image SHA-256 fingerprint |
| `launch_configuration_sha256` | string | 64 lowercase hexadecimal characters; identifies the exact version 1 launch input bytes |
| `profiles` | array | Ordered pinned profile identities; may be empty; each entry is `{"name", "sha256"}` with a non-empty name and a 64-lowercase-hex SHA-256 |

## Key ID

The enrolled key ID is `sha256:<hex>`, where `<hex>` is the lowercase hexadecimal SHA-256 digest of the public key's DER-encoded SubjectPublicKeyInfo (SPKI) form. The envelope's single signature carries this value as its `keyid`.

The key ID is a lookup hint over the public key's DER encoding, not a host identity by itself. A verifier selects the enrolled public key by key ID and separately requires the matching host ID.

## Host key enrollment

The signing key is one PKCS#8 PEM Ed25519 private key; its public half is enrolled as a SubjectPublicKeyInfo PEM. Each proof consumer is enrolled with three values:

- the stable `job_proof.host_id`;
- the SubjectPublicKeyInfo public key; and
- its `sha256:<hex>` [key ID](#key-id).

Two storage modes exist for the private key on the controller host — a file-backed systemd credential and a TPM-bound encrypted systemd credential. Both expose the same protected runtime file to the controller, and the proof format, key-ID rule, verifier behavior, and workflow are identical across modes; a receipt cannot attest which storage mode produced it. TPM binding protects the encrypted key at rest against offline use on another host. It is not TPM-native signing, measured boot, or remote attestation: the plaintext key exists in systemd's runtime credential store and in controller memory while the service runs.

Rotation is overlapping: a consumer that trusts both the old and new public keys can verify proofs issued before and after a key replacement. Existing proofs remain verifiable for as long as the consumer retains the old public key. Automatic key distribution and fleet PKI are not provided. The generation, installation, encryption, and rotation procedures are in [Deploy to production](../how-to/deploy.md#6-enable-job-proofs-optional).

## See also

- [Guest contract reference](guest-contract.md) — proof staging directory, ready marker, and the `incus-gh-runner-proof` helper
- [Configuration reference](configuration.md) — `job_proof` keys, credential drop-ins, and the `proof verify` command
- [Deploy to production](../how-to/deploy.md#6-enable-job-proofs-optional) — generate, enroll, install, and rotate the proof key
- [How incus-gh-runner works](../explanation/how-it-works.md#security-model) — where job proofs sit in the security model

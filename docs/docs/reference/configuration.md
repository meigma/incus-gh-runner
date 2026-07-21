# Configuration Reference

Complete listing of configuration sources, YAML keys, environment variables, CLI flags, and the systemd unit's configuration-related facts for `incus-gh-runner`.

## Sources and precedence

Controller-mode configuration is assembled from four sources. Precedence,
highest first:

1. CLI flags
2. Environment variables (`INCUS_GH_RUNNER_` prefix)
3. YAML configuration file
4. Built-in defaults

Each key is resolved independently, so different keys in the same run may come from different sources.

The default configuration file path is `/etc/incus-gh-runner/config.yaml`. This path is optional: if no file exists there, startup continues using environment variables, flags, and defaults. An explicit `--config` path is not optional — if the file does not exist or cannot be read, the process exits with an error before startup completes.

Configuration files are decoded exactly before source precedence is applied. Unknown or duplicate keys, misspellings, wrong YAML scalar or container types, aliases, merge keys, and multiple YAML documents are rejected. Field-level errors identify the field; validation errors do not include field values.

## YAML configuration keys

Duration values use Go duration syntax (for example `30s`, `5m`).

### `github`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `github.config_url` | string | — | Required. Absolute HTTPS GitHub or GHES organization or repository URL. GitHub-hosted domains may not specify a port; GHES ports must be valid. Enterprise URLs, trailing DNS dots, userinfo, query strings, fragments, escaped paths, and other path shapes are rejected. |
| `github.scale_set` | string | — | Required. Non-empty and exactly representable by the GitHub client without query decoding. Also used as the sole runner label. A pre-existing scale set must have that exact label and runner self-update disabled. |
| `github.runner_group` | string | `default` | Controller validation requires a non-empty, exactly representable value, `default` at repository scope, and an explicit non-default name at organization scope. Operators must separately restrict that organization group to selected repositories and commit-pinned workflows. |
| `github.token_file` | string | — | PAT file read once at startup. Mutually exclusive with `github.app` and `INCUS_GH_RUNNER_GITHUB_TOKEN`. |
| `github.app.client_id` | string | — | Required if GitHub App credentials are configured. |
| `github.app.installation_id` | int64 | — | Required if GitHub App credentials are configured. Must be greater than `0`. |
| `github.app.private_key_file` | string | — | Required if GitHub App credentials are configured. Path to a PEM file, read once at startup. |

### `incus`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `incus.socket` | string | `""` | Optional. Non-default local Incus Unix socket path. |
| `incus.project` | string | — | Required. Non-empty. Must already exist. |
| `incus.image` | string | — | Required. Non-empty. Existing local image alias or fingerprint, resolved to a full fingerprint during preflight. |
| `incus.profiles` | list of strings | `[]` | Optional. No empty entries. Profiles must already exist. Their effective configuration and devices are pinned during preflight and materialized directly onto each VM. When omitted, Incus image/default profile selection is reproduced before pinning. |
| `incus.owner` | string | — | Required. Non-empty. Exact cleanup selector written to every instance this process manages; not an authorization boundary. |
| `incus.bootstrap_timeout` | duration | `5m` | Must be greater than `0`. |
| `incus.diagnostics_dir` | string | `""` | Optional. Directory for terminal-runner serial console diagnostics. Persistence is disabled when empty. |

!!! warning "Sensitive console diagnostics"
    Content written to `incus.diagnostics_dir` can include workload console output. Restrict access to this directory accordingly.

Each capture is limited to 1 MiB and the directory sink retains at most 256 capture files. The packaged tmpfiles policy expires files older than 30 days from `/var/log/incus-gh-runner/diagnostics`; deployments using another directory must update that policy path.

### `job_proof`

Job machine proofs are disabled when this section is absent. The two settings
must be configured together; setting only one fails controller startup.

When enabled, the controller attempts one signed proof for every valid GitHub
`JobStarted` event on its resolved scale set. Events enter a non-blocking queue
whose capacity is `max(1, capacity.max_runners)`; malformed events or a full
queue produce no proof and do not interrupt runner demand tracking. The guest
helper therefore remains the fail-closed boundary for proof-dependent work: it
times out when no committed proof arrives.

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `job_proof.host_id` | string | `""` | Stable host identity recorded in every proof and enrolled beside the public key. Required when `job_proof.signing_key_file` is set. |
| `job_proof.signing_key_file` | string | `""` | Regular file no larger than 16 KiB containing exactly one PKCS#8 PEM Ed25519 private key. Read once during startup. Required when `job_proof.host_id` is set. |

The upstream listener acknowledges a queue message before invoking the job
callback. A controller crash in that interval or before proof delivery can
leave a running job without a proof; version 1 does not persist or reconstruct
events after restart. Proof-required workflows must treat helper timeout as a
hard failure.

### `capacity`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `capacity.min_runners` | int | `0` | Must be `>= 0`. |
| `capacity.max_runners` | int | `1` | Must be `>= capacity.min_runners`. |

### `concurrency`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `concurrency.incus_operations` | int | `2` | Must be `>= 1`. |

### `timeouts`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `timeouts.incus_operation` | duration | `5m` | Must be greater than `0`. |
| `timeouts.shutdown` | duration | `30s` | Must be greater than `0`. |

### `retry`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `retry.initial` | duration | `1s` | Must be greater than `0`. |
| `retry.maximum` | duration | `30s` | Must be `>= retry.initial`. |

### Top-level

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `reconcile_interval` | duration | `1s` | Must be greater than `0`. |

## Environment variables

Every scalar YAML key binds to an environment variable named
`INCUS_GH_RUNNER_` followed by the key path, uppercased, with `.` and `_` both
rendered as `_`. The list-valued `incus.profiles` key is YAML-only.

Examples:

| YAML key | Environment variable |
|---|---|
| `github.config_url` | `INCUS_GH_RUNNER_GITHUB_CONFIG_URL` |
| `incus.project` | `INCUS_GH_RUNNER_INCUS_PROJECT` |
| `capacity.min_runners` | `INCUS_GH_RUNNER_CAPACITY_MIN_RUNNERS` |
| `timeouts.shutdown` | `INCUS_GH_RUNNER_TIMEOUTS_SHUTDOWN` |
| `github.token_file` | `INCUS_GH_RUNNER_GITHUB_TOKEN_FILE` |
| `github.app.private_key_file` | `INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE` |
| `job_proof.host_id` | `INCUS_GH_RUNNER_JOB_PROOF_HOST_ID` |
| `job_proof.signing_key_file` | `INCUS_GH_RUNNER_JOB_PROOF_SIGNING_KEY_FILE` |

### `INCUS_GH_RUNNER_GITHUB_TOKEN`

Environment-only GitHub personal access token. This variable:

- Is never read from the YAML file (there is no `github.token` YAML key).
- Has no corresponding CLI flag.
- Is trimmed of surrounding whitespace before use.

For production systemd deployments, prefer `github.token_file` through the packaged PAT credential drop-in. A raw environment value is useful for local or externally supervised execution where the environment is already the credential boundary.

### `github.token_file`

Path to a file containing a GitHub personal access token. The controller reads and trims the file once during startup; a missing, unreadable, or empty file fails startup. The packaged PAT drop-in sets `INCUS_GH_RUNNER_GITHUB_TOKEN_FILE` to systemd's protected runtime credential copy, so the path should be absent from `config.yaml` in that deployment.

## Credential rule

Exactly one credential source must be configured:

- **GitHub App** — all three of `github.app.client_id`, `github.app.installation_id`, and `github.app.private_key_file` must be set.
- **Personal access token file** — `github.token_file` or `INCUS_GH_RUNNER_GITHUB_TOKEN_FILE` must identify the protected token file.
- **Personal access token value** — `INCUS_GH_RUNNER_GITHUB_TOKEN` must be set.

Configuring more than one method is an error, including setting both PAT sources. Configuring no credential is also an error.

## Job proof key enrollment and rotation

Generate the Ed25519 signing key and its SubjectPublicKeyInfo public key without
loosening the process umask:

```sh
umask 077
openssl genpkey -algorithm Ed25519 -out machine-provenance-key.pem
openssl pkey \
  -in machine-provenance-key.pem \
  -pubout \
  -out machine-provenance-key.pub.pem
```

Install the private key through a protected runtime credential path and set
`job_proof.signing_key_file` to that path. The two shipped proof-key drop-ins
both expose the credential as `%d/machine-provenance-key` and are installed
alongside, not instead of, the selected GitHub credential drop-in:

- `deploy/systemd/credentials-job-proof-file.conf` uses `LoadCredential=` with
  `/etc/incus-gh-runner/machine-provenance-key.pem`; install that source as
  `root:root` mode `0600`.
- `deploy/systemd/credentials-job-proof-tpm.conf` uses
  `LoadCredentialEncrypted=` with
  `/etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred`; create
  the directory as `root:root` mode `0700` and the encrypted credential as
  `root:root` mode `0600`.

The TPM-bound option requires systemd 250 or newer, a TPM 2.0 device, and the
distribution's TSS2 runtime libraries. Minimal Ubuntu 24.04 installations may
need `tpm2-tools` to supply those dynamically loaded libraries even though
`systemd --version` reports `+TPM2`; the encryption attempt remains the
authoritative capability check. The mode encrypts the same software key to the
target TPM with an empty PCR set. It changes storage only:
the plaintext still exists in systemd's runtime credential store and the
controller's memory, and the controller retains `PrivateDevices=yes` without
opening a TPM device. It is not TPM-native signing, measured boot, or remote TPM
attestation. See [Deploy](../how-to/deploy.md#option-b-tpm-bound-proof-key) for
the exact encryption and origin-host validation procedure.

Enroll three values with each consumer:

- the stable `job_proof.host_id`;
- `machine-provenance-key.pub.pem`; and
- its `sha256:<hex>` key ID over the public key's DER form.

Derive the enrolled key ID with OpenSSL:

```sh
key_hex="$(
  openssl pkey -pubin -in machine-provenance-key.pub.pem -outform DER |
    openssl dgst -sha256 -r |
    awk '{print $1}'
)"
printf 'sha256:%s\n' "$key_hex"
```

The key ID is a lookup hint over the public key's DER encoding, not a host
identity by itself. A consumer must select the enrolled public key and require
the matching host ID.

Rotate with overlap: distribute and trust the new public key first, replace the
file-backed source or encrypt and install a new TPM-bound credential, then
restart the controller. Retain the old public key for as long as existing
proofs must remain verifiable. The proof format and workflow do not change
between storage modes.

TPM clearing or motherboard replacement makes the encrypted credential
unusable. With an offline escrow, seal the same private key to the replacement
TPM; without one, generate a new key and enroll its public key and key ID before
restarting. Keeping an escrow is an explicit availability tradeoff that weakens
the assurance that the TPM-bound blob is the only recoverable private-key copy.
Automatic key distribution and fleet PKI are not provided.

!!! warning "Root-equivalent socket access"
    The controller's Incus client uses the account's `incus-admin` group membership, which grants root-equivalent control over the host. This applies regardless of which GitHub credential type is configured. The `incus.owner` value limits the controller's intended cleanup scope but is forgeable by another project writer; it is not authorization. Run the current production deployment only on a dedicated, single-purpose Incus host.

The packaged systemd deployment supplies either `github.app.private_key_file` or `github.token_file` through one selected credential drop-in. Secret values do not belong in `config.yaml`. See [systemd unit facts](#systemd-unit-facts).

## CLI

Running `incus-gh-runner` without a subcommand starts the controller. The
controller configuration sources and flags documented above apply to that
mode. The `validate` and `proof verify` subcommands are independent of controller
configuration and GitHub credentials.

### `--config`

| Flag | Type | Default |
|---|---|---|
| `--config` | string | `""` |

Configuration file path. Empty selects the default path (`/etc/incus-gh-runner/config.yaml`, optional). A non-empty value must point to an existing, readable file.

### Configuration flags

Each flag overrides its corresponding YAML key and environment variable.

| Flag | Type | Default | Configuration key |
|---|---|---|---|
| `--min-runners` | int | `0` | `capacity.min_runners` |
| `--max-runners` | int | `1` | `capacity.max_runners` |
| `--incus-operations` | int | `2` | `concurrency.incus_operations` |
| `--reconcile-interval` | duration | `1s` | `reconcile_interval` |
| `--operation-timeout` | duration | `5m` | `timeouts.incus_operation` |
| `--shutdown-timeout` | duration | `30s` | `timeouts.shutdown` |

### `--version`

Prints build metadata and exits:

```
incus-gh-runner <version> (<commit>) built <date>
```

In a release build, `<version>`, `<commit>`, and `<date>` are populated at build time. In a development build, these render as `dev`, `none`, and `unknown` respectively.

### `validate <baseline>`

Validates exactly one rendered JSON baseline against the embedded CUE policy,
then compares it with effective state read from a local Incus Unix socket. The
command performs read operations only; it never creates, changes, or deletes
Incus resources and does not invoke external `cue`, `incus`, or `jq`
executables.

| Flag | Type | Default |
|---|---|---|
| `--socket` | string | `/var/lib/incus/unix.socket` |

`--socket` selects a local Incus Unix socket. The command does not load
`/etc/incus-gh-runner/config.yaml`, controller flags or environment variables,
or GitHub credentials. A successful validation prints one human-readable line
to stdout; compatibility notices are written to stderr.

The live comparison confirms effective resource ceilings but cannot re-measure
or re-prove the physical-host capacity and reserved headroom used to generate
them. Re-render and review the baseline after those generation-time inputs
change.

### `proof verify <proof>`

Authenticates one DSSE job machine proof against an enrolled Ed25519 public key
and expected host identity:

```sh
incus-gh-runner proof verify \
  --public-key machine-provenance-key.pub.pem \
  --expected-host-id builder-host-01 \
  job-proof.dsse.json > verified-payload.json
```

| Flag | Type | Default |
|---|---|---|
| `--public-key` | string | required |
| `--expected-host-id` | string | required |

The command reads no controller configuration and never loads the signing key.
It limits the envelope and decoded payload to 64 KiB, requires the exact version
1 payload type and one signature, rejects unknown payload fields, and prints
only verified payload JSON to stdout. Repository, workflow, image, and freshness
policy are deliberately left to the caller. Any parse, signature, key, schema,
or host mismatch exits non-zero without printing unverified payload bytes.

### Exit behavior

Controller mode exits `0` on clean shutdown. It exits `1` on configuration
load failure, configuration validation failure, or runtime failure, printing
the error to stderr. There is no flag to control controller log level or log
format; controller logs are always structured JSON on stdout. `validate` exits
`0` only when policy and live-state checks pass and exits non-zero with an error
on stderr otherwise.

## systemd unit facts

The packaged base unit is `deploy/systemd/incus-gh-runner.service`. It deliberately selects no GitHub credential method. Install exactly one packaged credential drop-in as `/etc/systemd/system/incus-gh-runner.service.d/credentials.conf`:

- `credentials-github-app.conf` loads `/etc/incus-gh-runner/github-app-private-key.pem` and sets `INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE`.
- `credentials-personal-access-token.conf` loads `/etc/incus-gh-runner/github-token` and sets `INCUS_GH_RUNNER_GITHUB_TOKEN_FILE`.

When job proofs are enabled, install exactly one independent proof-key drop-in
as `job-proof.conf`:

- `credentials-job-proof-file.conf` loads the root-only plaintext source with
  `LoadCredential=`.
- `credentials-job-proof-tpm.conf` decrypts the TPM-bound source with
  `LoadCredentialEncrypted=`.

Both set `INCUS_GH_RUNNER_JOB_PROOF_SIGNING_KEY_FILE` to the same protected
runtime file, so they compose with either GitHub credential method.

| Directive | Value |
|---|---|
| `ExecStart` | `/usr/bin/incus-gh-runner --config /etc/incus-gh-runner/config.yaml` |
| GitHub credential | Selected by one systemd drop-in; absent from the base unit |
| `ConfigurationDirectory` | `incus-gh-runner` (mode `0755`), resolves to `/etc/incus-gh-runner` |
| `LogsDirectory` | `incus-gh-runner` (mode `0700`), resolves to `/var/log/incus-gh-runner` |
| `DynamicUser` | `yes` |
| `SupplementaryGroups` | `incus-admin` |
| `UMask` | `0077` |
| `Restart` | `on-failure` |
| `RestartSec` | `5s` |
| `StartLimitIntervalSec` / `StartLimitBurst` | `60s` / `5` |
| `KillSignal` | `SIGTERM` |
| `TimeoutStopSec` | `70s` |

`TimeoutStopSec` must exceed the application's internal shutdown budget of `2 × timeouts.shutdown`. At the default `timeouts.shutdown` of `30s`, that budget is `60s`, under the unit's `70s` stop timeout. Raising `timeouts.shutdown` requires raising `TimeoutStopSec` to keep `TimeoutStopSec > 2 × timeouts.shutdown`. See [How incus-gh-runner works](../explanation/how-it-works.md) for the shutdown model these settings drive.

### Hardening directives

The unit sets the following sandboxing directives: `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome`, `PrivateTmp`, `PrivateDevices`, `ProtectKernelTunables`, `ProtectKernelModules`, `ProtectKernelLogs`, `ProtectControlGroups`, `ProtectClock`, `ProtectHostname`, `ProtectProc=invisible`, `RestrictNamespaces`, `RestrictRealtime`, `RestrictSUIDSGID`, `LockPersonality`, `MemoryDenyWriteExecute`, `SystemCallArchitectures=native`, empty `CapabilityBoundingSet`, empty `AmbientCapabilities`, and `RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6`.

## See also

- [How incus-gh-runner works](../explanation/how-it-works.md) — capacity formula, lifecycle states, and the shutdown model these settings drive.
- [Deploy](../how-to/deploy.md) — end-to-end production deployment using this configuration surface.
- [Operate](../how-to/operate.md) — day-2 operational use of these settings.
- [Guest contract](guest-contract.md) — schemas and metadata keys not covered by this page.

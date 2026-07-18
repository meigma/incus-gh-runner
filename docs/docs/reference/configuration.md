# Configuration Reference

Complete listing of configuration sources, YAML keys, environment variables, CLI flags, and the systemd unit's configuration-related facts for `incus-gh-runner`.

## Sources and precedence

Configuration is assembled from four sources. Precedence, highest first:

1. CLI flags
2. Environment variables (`INCUS_GH_RUNNER_` prefix)
3. YAML configuration file
4. Built-in defaults

Each key is resolved independently, so different keys in the same run may come from different sources.

The default configuration file path is `/etc/incus-gh-runner/config.yaml`. This path is optional: if no file exists there, startup continues using environment variables, flags, and defaults. An explicit `--config` path is not optional — if the file does not exist or cannot be read, the process exits with an error before startup completes.

## YAML configuration keys

Duration values use Go duration syntax (for example `30s`, `5m`).

### `github`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `github.config_url` | string | — | Required. Absolute `http` or `https` URL (organization or repository). |
| `github.scale_set` | string | — | Required. Non-empty. Also used as the default runner label. |
| `github.runner_group` | string | `default` | Non-empty. |
| `github.app.client_id` | string | — | Required if GitHub App credentials are configured. |
| `github.app.installation_id` | int64 | — | Required if GitHub App credentials are configured. Must be greater than `0`. |
| `github.app.private_key_file` | string | — | Required if GitHub App credentials are configured. Path to a PEM file, read once at startup. |

### `incus`

| Key | Type | Default | Required / validation |
|---|---|---|---|
| `incus.socket` | string | `""` | Optional. Non-default local Incus Unix socket path. |
| `incus.project` | string | — | Required. Non-empty. Must already exist. |
| `incus.image` | string | — | Required. Non-empty. Existing local image alias or fingerprint. |
| `incus.profiles` | list of strings | `[]` | Optional. No empty entries. Profiles must already exist. |
| `incus.owner` | string | — | Required. Non-empty. Exact ownership marker written to every instance this process manages. |
| `incus.bootstrap_timeout` | duration | `5m` | Must be greater than `0`. |
| `incus.diagnostics_dir` | string | `""` | Optional. Directory for terminal-runner serial console diagnostics. |

!!! warning "Sensitive console diagnostics"
    Content written to `incus.diagnostics_dir` can include workload console output. Restrict access to this directory accordingly.

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

Every YAML key binds to an environment variable named `INCUS_GH_RUNNER_` followed by the key path, uppercased, with `.` and `_` both rendered as `_`.

Examples:

| YAML key | Environment variable |
|---|---|
| `github.config_url` | `INCUS_GH_RUNNER_GITHUB_CONFIG_URL` |
| `incus.project` | `INCUS_GH_RUNNER_INCUS_PROJECT` |
| `capacity.min_runners` | `INCUS_GH_RUNNER_CAPACITY_MIN_RUNNERS` |
| `timeouts.shutdown` | `INCUS_GH_RUNNER_TIMEOUTS_SHUTDOWN` |
| `github.app.private_key_file` | `INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE` |

### `INCUS_GH_RUNNER_GITHUB_TOKEN`

Environment-only GitHub personal access token, used for local testing. This variable:

- Is never read from the YAML file (there is no `github.token` YAML key).
- Has no corresponding CLI flag.
- Is trimmed of surrounding whitespace before use.

## Credential rule

Exactly one credential type must be configured:

- **GitHub App** — all three of `github.app.client_id`, `github.app.installation_id`, and `github.app.private_key_file` must be set.
- **Personal access token** — `INCUS_GH_RUNNER_GITHUB_TOKEN` must be set.

Configuring any GitHub App field together with the token environment variable is an error. Configuring neither is an error.

!!! warning "Root-equivalent socket access"
    The controller's Incus client uses the account's `incus-admin` group membership, which grants root-equivalent control over the host. This applies regardless of which credential type is configured.

In the packaged systemd unit, `github.app.private_key_file` is supplied through a systemd credential rather than the YAML file, and must be absent from `config.yaml` in that deployment. See [systemd unit facts](#systemd-unit-facts).

## CLI

`incus-gh-runner` is a single command with no subcommands.

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

### Exit behavior

The process exits `0` on clean shutdown. It exits `1` on configuration load failure, configuration validation failure, or runtime failure, printing the error to stderr. There is no flag to control log level or log format; logs are always structured JSON on stdout.

## systemd unit facts

The packaged unit is `deploy/systemd/incus-gh-runner.service`.

| Directive | Value |
|---|---|
| `ExecStart` | `/usr/bin/incus-gh-runner --config /etc/incus-gh-runner/config.yaml` |
| `LoadCredential` | `github-app-private-key:/etc/incus-gh-runner/github-app-private-key.pem` |
| `Environment` | `INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE=%d/github-app-private-key` (points at the loaded credential) |
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

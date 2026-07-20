# Guest Contract Reference

The controller pushes a runtime payload into a booted VM over the Incus agent; the guest runs exactly one GitHub Actions job, reports its status through a well-known file and the serial console, and powers itself off. This page documents that interface — the filesystem, JSON schemas, console output, and instance metadata that anything auditing the reference image, or replacing it with a custom one, must reproduce.

## Filesystem contract

`/run/incus-gh-runner/` is a directory on the guest's `/run` tmpfs, created by `systemd-tmpfiles` with mode `0700`, owner `root:root`.

The controller writes two files into this directory through the Incus agent, in order:

| Order | File | Mode | Content |
|---|---|---|---|
| 1 | `payload.json` | `0600` | The [payload](#payload-schema-payloadjson), JSON-encoded |
| 2 | `payload.ready` | `0600` | Empty; its presence is the only signal that matters |

A `path` unit watches for `payload.ready` and starts the one-shot guest service once it exists. The guest service requires both files to be present, parses and validates `payload.json`, and deletes both files before the Actions Runner process starts. If either file is missing when the service starts, the guest treats this as a fatal startup error, writes an error line to the serial console, and exits.

## Payload schema (`payload.json`)

```json
{
  "version": 1,
  "jit_config": "<opaque string>"
}
```

| Field | Type | Constraint |
|---|---|---|
| `version` | integer | Must equal `1` |
| `jit_config` | string | Non-empty; the opaque GitHub Actions just-in-time runner registration configuration |

The object must contain exactly these two keys — no more, no fewer. Any other shape, a `version` other than `1`, or an empty or missing `jit_config` fails guest-side validation.

## Status schema (`status.json`)

```json
{
  "version": 1,
  "state": "exited",
  "exit_code": 0
}
```

| Field | Type | Constraint |
|---|---|---|
| `version` | integer | Always `1` |
| `state` | string | One of `starting`, `running`, `exited`, `failed` |
| `exit_code` | integer | Present once the runner process has exited; the process's exit status |

The reference guest writes this file to a temporary path in the same directory and renames it into place, with mode `0600`, so a reader never observes a partially written file. It progresses through `starting` → `running` → `exited` over the lifetime of one job. It never emits `state: failed`; a non-zero `exit_code` on the `exited` state is how a failed job is represented. `failed` is a valid, controller-recognized value for a custom guest that distinguishes runner-process failure from a clean exit.

## Controller state mapping

The controller combines Incus and `status.json` with authoritative GitHub
message-session events. Guest state alone never proves that a connected runner
is idle:

| Signal | Runner state |
|---|---|
| Incus instance status is `stopped` or `error` | `terminal` |
| Incus instance status is `running` and `status.json` state is `running` | `ready`; job occupancy is resolved by the controller |
| Exact `JobStarted` event in the current GitHub message session | `busy` and ineligible for scale-down |
| Incus instance status is `running` and `status.json` state is `exited` or `failed` | `terminal` |
| Instance not yet `running`, or a running instance with an absent `status.json` or state `starting` | `provisioning`, until the instance's age exceeds `incus.bootstrap_timeout`, then `terminal` |

Instance age is measured from the `user.incus-gh-runner.created-at` metadata value (falling back to the Incus-reported creation time if that key is absent). See [Configuration Reference](configuration.md) for `incus.bootstrap_timeout`, and [How incus-gh-runner works](../explanation/how-it-works.md) for the lifecycle states themselves.

Only a guest-file not-found response while the instance still exists means
`status.json` has not appeared yet. A disappeared instance, timeout, transport
or permission failure, malformed document, unsupported version, or unknown
state invalidates the complete inventory refresh. The controller then retains
its last observation and schedules no create or delete mutation until a fresh
inventory succeeds. Each runner status read receives an independent bounded
share of the overall Incus operation deadline, so one slow guest agent cannot
consume the observation budget for later runners.

Only a ready runner created under the current GitHub message-session generation
can be considered for idle scale-down. Before changing its Incus state, the
controller removes and confirms absence of the runner's exact GitHub
registration. The fenced VM remains alive until the guest runner process exits
and reports a terminal state. A controller restart or GitHub message-session
reconnect invalidates process-local idle knowledge; reconstructed ready runners
remain capacity but are not scale-down candidates.

## Serial console contract

The guest's serial console is `ttyS0`. It carries secret-free lifecycle lines only:

| Line | Emitted when |
|---|---|
| `incus-gh-runner-guest state=<state>` | Each `status.json` transition (`starting`, `running`, `exited`) |
| `incus-gh-runner-guest error=missing-ready-marker` | `payload.ready` is absent at guest service start |
| `incus-gh-runner-guest error=missing-payload` | `payload.json` is absent at guest service start |
| `incus-gh-runner-guest action=poweroff exit_code=<exit_code> grace_seconds=30` | Immediately before shutdown, on every code path |

After the `action=poweroff` line, the guest sleeps 30 seconds — a fixed diagnostic grace period — before calling `systemctl poweroff`. This grace period runs on every exit path, including startup validation failures.

!!! warning "JIT configuration never reaches the console or guest journal"
    `jit_config` is passed to the Actions Runner process only as a command-line argument. The guest never writes it to `/dev/ttyS0` or to its own systemd journal, on any code path, including error paths.

## Instance metadata

The controller sets these keys on every instance it creates:

| Key | Value | Purpose |
|---|---|---|
| `user.incus-gh-runner.owner` | The configured `incus.owner` value | Exact-match cleanup selector; instances without a matching value are excluded from listing and refused on delete, but another project writer can forge it |
| `user.incus-gh-runner.correlation-id` | Generated UUID | Unique per-instance identifier, also used to derive the instance name |
| `user.incus-gh-runner.created-at` | RFC3339Nano timestamp, UTC | Anchor for the `incus.bootstrap_timeout` calculation |
| `user.incus-gh-runner.image` | Resolved full image fingerprint | Immutable image identity used to create the instance |
| `user.incus-gh-runner.image-reference` | Configured `incus.image` value | Operator-friendly alias or fingerprint supplied to the controller |
| `user.incus-gh-runner.profiles` | JSON array of profile names and SHA-256 digests | Audit identity for the effective profile configuration and devices materialized at create time |

Image aliases are resolved once during controller preflight. The controller
creates each VM by the resulting fingerprint. Effective profile configuration
and devices are also captured during preflight, revalidated before each create,
and written directly onto the instance with an explicit empty attached-profile
list. Later alias retargeting or profile edits therefore cannot change an
already approved runner environment; profile drift blocks new capacity until
the approved profile state is restored or the controller is deliberately
restarted and preflight succeeds again.

## Diagnostics capture

When `incus.diagnostics_dir` is configured, the controller captures the instance's serial console log during deletion, after ensuring the instance is stopped and before removing it, and writes it to `<diagnostics_dir>/<runnerID>.console.log`, mode `0600`, inside a directory created with mode `0700`. When `incus.diagnostics_dir` is empty, no diagnostics file is written; captured console output is discarded rather than persisted.

!!! warning "Console diagnostics may contain sensitive output"
    Captured console content may include sensitive workload output. Diagnostics files must be handled with the same care as other job-adjacent artifacts.

## Reference image

| Property | Value |
|---|---|
| Base distribution | Ubuntu 24.04 LTS (`noble`), built with debootstrap |
| Architecture | x86_64 |
| Image format | Incus unified VM tarball |
| Virtual disk | 8 GiB (8589934592 bytes), ext4 |
| Actions Runner version | 2.335.1, SHA-256 pinned at build time |
| Actions Runner install path | `/opt/actions-runner` |
| Runner OS user | `actions-runner`, system account, `nologin` shell |
| `/etc/machine-id` | Set to the literal string `uninitialized` at build time |
| Kernel console | GRUB configures `console=tty1 console=ttyS0` |
| Guest bootstrap trigger | `incus-gh-runner-guest.path`, enabled at build time, watching for `payload.ready` |

For obtaining, verifying, importing, or building this image, see [Runner images](../how-to/runner-images.md).

## See also

- [Configuration Reference](configuration.md) — `incus.owner`, `incus.image`, `incus.bootstrap_timeout`, `incus.diagnostics_dir`
- [Runner images](../how-to/runner-images.md) — obtaining, verifying, building, and validating images against this contract
- [How incus-gh-runner works](../explanation/how-it-works.md) — runner lifecycle states and the cleanup boundary

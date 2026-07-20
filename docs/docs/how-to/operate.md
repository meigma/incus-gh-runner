# How to operate and troubleshoot incus-gh-runner

Read logs, collect VM diagnostics, change capacity and configuration safely, upgrade the binary and reference image, and diagnose common failures on a running deployment.

## Prerequisites

- `incus-gh-runner` deployed as the `incus-gh-runner.service` systemd unit (see [Deploy incus-gh-runner](./deploy.md)).
- `journalctl` access to the unit (root, or membership in a group granted access).
- Shell access to edit `/etc/incus-gh-runner/config.yaml` and restart the unit.

## Read the logs

The process writes structured JSON logs (Go `slog`) to stdout. Under systemd this lands in the journal for the unit:

```console
journalctl -u incus-gh-runner -f
```

Add `-o json-pretty` to inspect individual fields, or `-o cat` to print the raw JSON line emitted by the process:

```console
journalctl -u incus-gh-runner -o cat -n 200
```

There is no log-level or verbosity flag. The process always logs at its built-in level; you cannot quiet or increase it via configuration.

The following events carry the fields you need for day-2 monitoring. All other fields on a line (timestamp, logger group, message) are structural `slog` output.

| Event | Fields | Meaning |
|---|---|---|
| `runner demand updated` | `assigned_jobs`, `target`, `generation` | Controller recomputed capacity target and current GitHub message-session authority. |
| `runner operation scheduled` | `operation` (`create`/`fence`/`delete`/`list`), `operation_id`, `runner_id` | An external lifecycle operation was handed to a worker. `fence` removes GitHub registration before idle scale-down. |
| `runner operation completed` | `operation`, `operation_id`, `runner_id` | The operation succeeded. |
| `runner operation failed` | `operation`, `operation_id`, `runner_id`, `retry_after`, `error` | The operation failed and entered cooldown; see [Troubleshooting](#troubleshooting) below. |
| `GitHub Actions job started` | `runner_name`, `job_id` | A queued job was assigned to a runner. |
| `GitHub Actions job completed` | `runner_name`, `job_id`, `result` | The job finished on that runner. |
| `GitHub message session disconnected; reconnecting` | `error`, `retry_after` | The GitHub Actions long-poll session dropped; see [Troubleshooting](#troubleshooting) below. |
| `owned Incus runner started` | `runner_id`, `correlation_id` | A VM the controller owns was created, started, and handed its job payload; it is provisioning until the guest reports in. |
| `owned Incus runner deleted` | `runner_id` | A VM the controller owns was deleted. |

Credentials (GitHub App private key material, PAT values, JIT runner configuration) are never logged, so logs are safe to attach to a ticket.

For what these events mean in terms of runner lifecycle and capacity, see [How incus-gh-runner works](../explanation/how-it-works.md).

## Collect VM diagnostics

When a runner VM is deleted, the controller captures its serial console log before tearing it down. To keep these captures, point `incus.diagnostics_dir` at a directory under the unit's `LogsDirectory`:

```yaml
incus:
  diagnostics_dir: /var/log/incus-gh-runner/diagnostics
```

1. Create the directory (or let the unit's `LogsDirectory` machinery own it) so it is writable by the service's `DynamicUser`.
2. Restart the unit for the new value to take effect:
   ```console
   systemctl restart incus-gh-runner
   ```
3. After a runner VM is deleted, find its capture at `<diagnostics_dir>/<runnerID>.console.log`. See the [guest contract reference](../reference/guest-contract.md) for the diagnostics file naming and permissions.

If `incus.diagnostics_dir` is unset, console captures are discarded and nothing is written to disk.

The controller caps each capture at 1 MiB, marks truncated output, and retains at most 256 capture files by deleting the oldest before a new file is created. The packaged `incus-gh-runner.tmpfiles.conf` additionally removes files older than 30 days from the recommended directory. Install it as `/usr/lib/tmpfiles.d/incus-gh-runner.conf`; if you choose a different diagnostics directory, copy the policy and change its path to match. Ensure `systemd-tmpfiles-clean.timer` is enabled so expiration also runs while the controller is idle.

!!! warning "Console output may contain sensitive workload content"
    The serial console log is the guest's raw console output, including anything a running Actions job printed to it before exit. Treat diagnostics files as sensitive and restrict directory access to operators who are cleared to see job output. Shorten the shipped 30-day expiration when your workload sensitivity requires it.

For the full cleanup and metadata model behind runner VMs, see [How incus-gh-runner works](../explanation/how-it-works.md). For every configuration key, see [Configuration reference](../reference/configuration.md).

## Change capacity or configuration safely

To change capacity limits, timeouts, or any other setting:

1. Edit `/etc/incus-gh-runner/config.yaml` (or the relevant environment override).
2. Restart the unit:
   ```console
   systemctl restart incus-gh-runner
   ```

`systemctl restart` sends `SIGTERM`. The controller does not delete busy VMs on shutdown — jobs already running on a runner continue to completion and are reconciled against the new configuration once the process comes back up. Idle and provisioning runners are unaffected by the restart itself; they are re-evaluated against the new target on the next reconcile.

### Shutdown budget

Each shutdown phase (graceful, then forced) waits up to `timeouts.shutdown`; the total wait budget across both phases is `2 * timeouts.shutdown`. The unit's `TimeoutStopSec` must exceed this budget, or systemd kills the process mid-shutdown before it finishes waiting out active operations. The packaged unit ships `TimeoutStopSec=70s`, which covers the default `timeouts.shutdown: 30s` (budget 60s) with headroom.

If you raise `timeouts.shutdown` above 35s, raise `TimeoutStopSec` in the unit to stay above `2 * timeouts.shutdown`, run `systemctl daemon-reload`, then restart the unit.

For the full set of timeout and capacity keys, see [Configuration reference](../reference/configuration.md).

## Upgrade

### Upgrade the binary

1. Replace `/usr/bin/incus-gh-runner` with the new binary.
2. Restart the unit:
   ```console
   systemctl restart incus-gh-runner
   ```

The same busy-VM survival and shutdown-budget behavior described above applies during a binary upgrade.

### Upgrade the reference image

1. Import the new image into Incus under a new alias or fingerprint.
2. Update `incus.image` in `config.yaml` to point at it.
3. Restart the unit.

Existing runner VMs built from the old image are left running until they finish their job and are recycled; new VMs are created from the image now referenced by `incus.image`. See [Obtain, build, and validate runner images](./runner-images.md) for how to fetch, verify, and validate a released image before importing it.

## Troubleshooting

| Symptom | Likely cause | Action |
|---|---|---|
| Unit exits immediately at start | Invalid config, a bad credential, or the startup preflight failed to resolve the configured Incus image or a profile | Read the error on stderr / `journalctl -u incus-gh-runner`; the process fails fast and reports the specific validation or preflight error. |
| Repeated `GitHub message session disconnected; reconnecting` | A GitHub-side outage, or the App/PAT credential was revoked mid-run | Backoff is capped and automatic; no restart is needed for a transient outage. If it persists, verify the credential is still valid. |
| Runner stays `provisioning`, then goes `terminal` after a while | Image doesn't implement the guest contract, the wrong image is configured, or the VM has no network reachability to GitHub | Check `incus.image` and `incus.bootstrap_timeout`; validate the image with [`image/validate-incus.sh`](./runner-images.md); confirm the VM's network path. |
| `runner operation failed` repeats with growing `retry_after` | An Incus-side failure (API, storage, hypervisor) put that operation into cooldown | Creates share one cooldown; each runner's delete has its own. A fresh successful inventory list (`operation: list`) must land before further mutation is attempted — check Incus itself for the underlying error reported in the `error` field. |
| A runner VM refuses deletion | The controller's cleanup selector on the instance doesn't match `incus.owner` | Expected behavior — the controller refuses deletion outside its exact selector. Confirm the VM's `user.incus-gh-runner.owner` metadata and your configured `incus.owner`, and delete manually via Incus if it is genuinely orphaned. The selector prevents accidental cleanup; another project writer can forge it. |

## Related

- [Deploy incus-gh-runner](./deploy.md)
- [Obtain, build, and validate runner images](./runner-images.md)
- [Configuration reference](../reference/configuration.md)
- [Guest contract reference](../reference/guest-contract.md)
- [How incus-gh-runner works](../explanation/how-it-works.md)

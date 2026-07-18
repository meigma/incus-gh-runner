# incus-gh-runner

`incus-gh-runner` is an early-stage controller for running ephemeral GitHub
Actions jobs in Incus virtual machines. The controller will consume GitHub
runner scale-set demand, maintain a bounded pool of hot standby runners, and
delete each VM after its one assigned job.

The phase 1 controller core reconciles coalesced demand through a bounded worker
pool using explicit demand-source and runner-backend ports. Phase 2 adds a
checksummed, offline-built reference VM image and a versioned one-shot guest
contract. Phase 3 adds periodic ownership inventory and the real Incus backend
for create, start, payload injection, observation, diagnostics, and deletion.
Phase 4 wires persistent GitHub scale-set resolution, message polling, demand
statistics, and fresh per-VM JIT configuration into that lifecycle.
The phase 4 hardware proof ran one genuine job on Incus 7.2 and returned the
owned inventory to zero. Phase 5 proved hot-standby replacement and restart
reconciliation for the primary live path. Phase 6 hardens unattended operation
with GitHub session recovery, bounded shutdown escalation, and a protected
systemd deployment.

## v1 boundaries

- One GitHub runner scale set per controller process.
- One preconfigured Incus environment and runner image.
- Fully connected hot standby runners with a configurable minimum and maximum.
- One JIT configuration and at most one job per VM.
- Ownership-scoped reconciliation without a controller database.
- A foreground process intended to run under systemd.

The controller will not create or manage Incus projects, networks, storage,
profiles, clustering, or host security.

## Development

[mise](https://mise.jdx.dev) provides the locked toolchain, and
[Moon](https://moonrepo.dev) is the task runner and CI entrypoint:

```sh
mise install
moon run root:check
```

Useful focused commands are:

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
go run ./cmd/incus-gh-runner --version
```

The smallest real single-runner configuration is:

```yaml
github:
  config_url: https://github.com/meigma/incus-gh-runner
  scale_set: incus-gh-runner-phase4
  runner_group: default
incus:
  project: runner-test
  image: incus-gh-runner:test
  profiles: [default]
  owner: incus-gh-runner-phase4
  bootstrap_timeout: 5m
  diagnostics_dir: /var/log/incus-gh-runner/runners
capacity:
  min_runners: 0
  max_runners: 1
concurrency:
  incus_operations: 1
reconcile_interval: 1s
timeouts:
  incus_operation: 5m
  shutdown: 30s
retry:
  initial: 1s
  maximum: 30s
```

Configuration precedence is flags, environment variables, the selected YAML
file, then defaults. `--config` selects a required file; otherwise
`/etc/incus-gh-runner/config.yaml` is optional. Environment variables use the
`INCUS_GH_RUNNER` prefix, such as
`INCUS_GH_RUNNER_CAPACITY_MAX_RUNNERS`.

Production authentication uses a GitHub App configured with `client_id`,
`installation_id`, and a protected `private_key_file`. The initial development
path accepts a PAT only through `INCUS_GH_RUNNER_GITHUB_TOKEN`; the token is not
decoded from YAML and has no CLI flag. The process resolves or creates the
configured scale set and leaves that persistent scale set in place across
controller restarts.

Startup still fails fast when the initial GitHub message session cannot be
opened. After startup, a listener or session failure closes the old session and
creates a fresh one using capped exponential backoff. Successful GitHub polling,
including a healthy long-poll expiry with no message, resets the delay to
`retry.initial`; `retry.maximum` prevents a prolonged outage from producing an
unbounded retry interval. The controller applies the same bounds to failed Incus
inventory, create, and delete operations. An inventory failure pauses mutations
until a fresh owned-runner snapshot succeeds; create cooldown is shared, while
delete cooldown is isolated to the exact runner so unrelated cleanup can proceed.

On SIGINT or SIGTERM, the application cancels both long-lived components and
waits through the controller's graceful and forced-cancellation shutdown
windows. If either component still has not returned after twice
`timeouts.shutdown`, the process exits with an error so its supervisor can
restart it instead of leaving a wedged service indefinitely.

The checked-in hardened unit, example configuration, credential boundary, and
installation procedure are documented in the
[systemd deployment guide](https://meigma.github.io/incus-gh-runner/deployment/).

CI runs the same aggregate gate with `moon ci --summary minimal`.

## Reference runner image

The `image/` tree defines an Ubuntu 24.04 x86_64 Incus VM with a pinned Actions
Runner and a one-shot systemd guest service. The hosted `Reference Image`
workflow proves that distrobuilder can assemble the unified VM artifact without
starting a VM or requiring KVM acceleration. Real boot validation remains a
separate Incus-capable functional gate.

See the [reference image and guest contract](https://meigma.github.io/incus-gh-runner/reference-image/)
for the payload, readiness, diagnostic, cleanup, and poweroff behavior.

The integration seams pin
[`actions/scaleset`](https://github.com/actions/scaleset) and the
[`incus/v7` Go client](https://github.com/lxc/incus). Third-party client types
stay in `internal/adapters`; controller-owned ports live with the orchestration
core. Scale-set callbacks only publish into the coalescing mailbox; JIT and
Incus calls remain in the bounded runner-operation path.

## Functional test environments

Unit tests must not require Incus or GitHub access. Functional lifecycle work
uses explicitly disposable infrastructure:

- Incus testing targets a dedicated project on a local or otherwise isolated
  daemon. The project, image, profiles, network, and storage must already exist;
  tests must never point cleanup logic at an unrelated or shared project.
- Live VM diagnostics require Incus 7.0 or newer. Incus 6 releases are not
  supported.
- GitHub scale-set testing targets a dedicated private repository or
  organization and a uniquely named test scale set. Do not use production
  repositories, runner groups, or credentials for development experiments.

The opt-in GitHub preflight resolves the persistent scale set and opens and
closes one real message session without creating a runner:

```sh
INCUS_GH_RUNNER_TEST_GITHUB_CONFIG_URL=https://github.com/meigma/incus-gh-runner \
INCUS_GH_RUNNER_TEST_GITHUB_SCALE_SET=incus-gh-runner-phase4 \
INCUS_GH_RUNNER_GITHUB_TOKEN="$(gh auth token)" \
go test ./internal/adapters/github -run TestScaleSetSessionFunctional -count=1 -v
```

The Incus lifecycle test is opt-in and destructive only for instances carrying
its unique ownership marker. It refuses the default project and expects the
phase 2 reference image to be imported already:

```sh
INCUS_GH_RUNNER_TEST_PROJECT=runner-test \
INCUS_GH_RUNNER_TEST_IMAGE=incus-gh-runner:test \
INCUS_GH_RUNNER_TEST_PROFILES=default,github-runner \
go test ./internal/adapters/incus -run TestIncusLifecycleFunctional -count=1 -v
```

`INCUS_GH_RUNNER_TEST_PROFILES` and `INCUS_GH_RUNNER_TEST_SOCKET` are optional.
The proof drives one unit of fake demand through the controller, injects a
credential-free probe payload, captures the live terminal serial history during
the guest's bounded diagnostic grace window, deletes the exact owned VM, and
verifies that the owned inventory returns to zero.

The manual `Runner Functional Proof` workflow accepts an exact runner label and
executes a minimal unprivileged Linux job on that scale set. Prepare all costly
live-test inputs before allocating hardware:

```sh
scripts/live/phase4-prepare.sh
```

That command cross-compiles the controller and phase 3 functional test, starts
and downloads a fresh hosted reference-image build, verifies its checksum, and
assembles the transfer bundle under `build/live-phase4`. On a fresh Ubuntu
24.04 Incus-capable host, `phase4-host-prepare.sh` installs the native Incus and
QEMU packages, creates a non-default project, and runs the phase 2 and phase 3
live gates before the controller is started for the genuine phase 4 job.

The phase 5 hot-standby harness starts the controller, waits for GitHub to
report exactly one connected idle runner, dispatches a held one-shot job,
observes that runner become busy and a different idle replacement connect,
then verifies job success and exact deletion. It restarts around the idle
replacement and its cleanup job before returning the scale set and Incus
inventory to zero. Use a dedicated repository scale set with
`min_runners: 1`, `max_runners: 2`, and at least two Incus operations:

```sh
INCUS_GH_RUNNER_GITHUB_TOKEN="$(gh auth token)" \
scripts/live/phase5-hot-standby.sh \
  meigma/incus-gh-runner \
  incus-gh-runner-phase5 \
  runner-test \
  phase5-live-20260718 \
  /path/to/incus-gh-runner \
  /path/to/config.yaml \
  /var/log/incus-gh-runner/phase5-hot-standby
```

The token must be able to run and inspect repository workflows. Readiness is
proven from the exact-owner Incus inventory and the guest runner journal;
workflow-job state proves which runner accepted each job without requiring
organization-wide runner administration permission. The harness preserves
controller logs from each process lifetime, runner snapshots, workflow output,
and a correlation manifest in the evidence directory.

## Packaging

Release Please prepares version bumps, tags, and a draft GitHub release. The
tag-triggered release workflow then builds and attests four GoReleaser binaries,
publishes and signs the multi-architecture controller OCI image, and attaches a
versioned Incus reference-image archive plus checksum to that draft. The draft
remains a deliberate human publication decision after the workflow inspection
summary has been reviewed.

Release PRs rehearse the controller binary, OCI image, and reference VM build
paths without uploading GitHub release assets. See the
[reference-image release instructions](https://meigma.github.io/incus-gh-runner/reference-image/#release-artifact)
for download, checksum, provenance, and Incus boot verification commands. The
first public v1 release remains pending until the complete phase 7 acceptance
scenario succeeds.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and
[SECURITY.md](SECURITY.md) for private vulnerability reporting.

The repository does not yet include a project license.

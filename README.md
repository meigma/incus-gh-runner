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
credential-free probe payload, captures the terminal serial console, deletes
the exact owned VM, and verifies that the owned inventory returns to zero.

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

## Packaging

The repository retains its GoReleaser binary path and melange/apko container
path. These assets are renamed for `incus-gh-runner`, but they are not considered
release-proven until the v1 packaging phase exercises them.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and
[SECURITY.md](SECURITY.md) for private vulnerability reporting.

The repository does not yet include a project license.

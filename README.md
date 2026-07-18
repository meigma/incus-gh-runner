# incus-gh-runner

`incus-gh-runner` is an early-stage controller for running ephemeral GitHub
Actions jobs in Incus virtual machines. The controller will consume GitHub
runner scale-set demand, maintain a bounded pool of hot standby runners, and
delete each VM after its one assigned job.

The phase 1 controller core reconciles coalesced demand through a bounded worker
pool using explicit demand-source and runner-backend ports. Phase 2 adds a
checksummed, offline-built reference VM image and a versioned one-shot guest
contract. Real GitHub and Incus lifecycle adapters are intentionally not wired
into the executable yet.

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

The controller configuration boundary currently covers the settings needed by
the fake-demand proof:

```yaml
capacity:
  min_runners: 0
  max_runners: 4
concurrency:
  incus_operations: 2
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
core. The deterministic fake application test is the current functional proof
until the real lifecycle slices begin.

## Functional test environments

Unit tests must not require Incus or GitHub access. Functional lifecycle work
uses explicitly disposable infrastructure:

- Incus testing targets a dedicated project on a local or otherwise isolated
  daemon. The project, image, profiles, network, and storage must already exist;
  tests must never point cleanup logic at an unrelated or shared project.
- GitHub scale-set testing targets a dedicated private repository or
  organization and a uniquely named test scale set. Do not use production
  repositories, runner groups, or credentials for development experiments.

Live functional tests and their credential interface will be added alongside
the first real Incus and GitHub lifecycle slices rather than specified ahead of
the working implementation.

## Packaging

The repository retains its GoReleaser binary path and melange/apko container
path. These assets are renamed for `incus-gh-runner`, but they are not considered
release-proven until the v1 packaging phase exercises them.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and
[SECURITY.md](SECURITY.md) for private vulnerability reporting.

The repository does not yet include a project license.

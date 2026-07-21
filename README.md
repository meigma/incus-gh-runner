# incus-gh-runner

`incus-gh-runner` is a controller that runs ephemeral GitHub Actions runners in
Incus virtual machines. It registers a runner scale set with GitHub, provisions
a fresh VM with a just-in-time runner registration for each assigned job, and
deletes the VM when its one job finishes.

## Features

- One job per VM: every job runs in a fresh virtual machine that is deleted
  afterward, so no state leaks between jobs.
- Hot standby pool: a configurable minimum of connected idle runners absorbs
  bursts, bounded by a configurable maximum.
- Cleanup-scoped reconciliation: the controller only counts and deletes VMs
  carrying its exact cleanup marker, and keeps no database of its own. The
  marker prevents accidental cross-controller cleanup; it is not an Incus
  authorization boundary.
- Unattended operation: GitHub session recovery with capped backoff, bounded
  shutdown escalation, and a hardened systemd unit with credential isolation.
- Bring your own image: any VM image that implements the documented guest
  contract works. The repository ships the guest-side components and a
  hardening guide for building one.

## Requirements

- A dedicated, single-purpose Linux host running Incus 7.0 or newer with QEMU
  VM support. Incus 6 is not supported.
- Membership in the `incus-admin` group for the controller process. This is
  root-equivalent on the host. Do not deploy the current controller on an Incus
  host shared with unrelated trusted workloads.
- A GitHub App or personal access token authorized to manage runner scale sets
  at the configured repository or organization.

## Installation

Each GitHub release provides `incus-gh-runner_<version>_<os>_<arch>` binaries
for Linux and macOS (amd64 and arm64) and a multi-architecture controller OCI
image, all with checksums and build attestations.

Download a binary from the [releases page](https://github.com/meigma/incus-gh-runner/releases)
and install it:

```sh
install -m 0755 incus-gh-runner_<version>_linux_amd64 /usr/bin/incus-gh-runner
```

To build from source instead:

```sh
go build ./cmd/incus-gh-runner
```

See [Build runner images](docs/docs/how-to/build-runner-images.md) for
building the guest VM image the controller boots for each job.

## Usage

The controller is a single foreground command driven by one configuration
file:

```sh
incus-gh-runner --config config.yaml
```

The smallest working configuration:

```yaml
github:
  config_url: https://github.com/OWNER/REPOSITORY
  scale_set: incus-gh-runner-example
  runner_group: default
  app:
    client_id: Iv1.xxxxxxxxxxxxxxxx
    installation_id: 12345678
    private_key_file: /path/to/private-key.pem
incus:
  project: github-runners
  image: incus-gh-runner-v1
  profiles: [github-runner]
  owner: incus-gh-runner-example
```

For a PAT, omit the `app` block and set `github.token_file` to a protected token
file instead. Private-repository scope is the hardened starting point and pairs
the scale set with only that repository. Public repositories require a separate
threat review that prevents untrusted fork code from targeting the runner.
Organization scope is supported only with a dedicated non-default runner group
restricted to the selected repositories and commit-pinned workflows that may
submit jobs. Set `github.runner_group` to that group's exact name. The controller
rejects the `default` group for organization scope; enterprise URLs are outside
the supported contract.

The referenced Incus project, image, and profiles must already exist; the
controller creates the GitHub scale set automatically if it is absent. Jobs
target the scale set by its name:

```yaml
jobs:
  example:
    runs-on: incus-gh-runner-example
```

For production, run the controller under the hardened systemd unit in
[`deploy/systemd/`](deploy/systemd/), selecting the GitHub App or PAT credential
drop-in. Apply and validate the restricted project, network, profile, storage,
resource limits, and controlled-egress baseline with the CUE policy and
read-only drift tooling in
[`deploy/incus/`](deploy/incus/) first. Follow the
[deployment guide](docs/docs/how-to/deploy.md) for the end-to-end path.

## Documentation

- [Deploy to production](docs/docs/how-to/deploy.md) — host preparation,
  restricted Incus preparation, repository or organization scope, GitHub App
  or PAT setup, and the systemd installation.
- [Operate and troubleshoot](docs/docs/how-to/operate.md) — logs, VM
  diagnostics, safe configuration changes, and upgrades.
- [Build runner images](docs/docs/how-to/build-runner-images.md) — building
  and boot-testing a hardened, contract-conforming runner image.
- [Configuration reference](docs/docs/reference/configuration.md) — every
  configuration key, environment variable, and CLI flag.
- [Guest contract reference](docs/docs/reference/guest-contract.md) — the
  controller-guest interface every runner image must implement.
- [How incus-gh-runner works](docs/docs/explanation/how-it-works.md) — the
  capacity model, runner lifecycle, cleanup boundary, and security model.

## Development

[mise](https://mise.jdx.dev) provides the locked toolchain and
[Moon](https://moonrepo.dev) is the task runner; CI runs the same aggregate
gate through `mise exec -- moon ci`:

```sh
mise install
mise exec -- moon run root:check
```

Business logic stays isolated from the GitHub and Incus client adapters:
third-party types live in `internal/adapters`, controller-owned ports live
with the orchestration core, and scale-set callbacks only publish into a
coalescing mailbox while Incus work runs in a bounded worker pool.

Unit tests run without Incus or GitHub access. Opt-in functional tests
(`INCUS_GH_RUNNER_TEST_*` environment variables) exercise the real GitHub
scale-set session and the destructive Incus VM lifecycle against explicitly
disposable projects. See the comments in those test files for the required
inputs.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and
[SECURITY.md](SECURITY.md) for private vulnerability reporting.

## License

Licensed under either of

- Apache License, Version 2.0 ([LICENSE-APACHE](LICENSE-APACHE))
- MIT License ([LICENSE-MIT](LICENSE-MIT))

at your option.

Unless you explicitly state otherwise, any contribution intentionally
submitted for inclusion in this project by you, as defined in the Apache-2.0
license, shall be dual licensed as above, without any additional terms or
conditions.

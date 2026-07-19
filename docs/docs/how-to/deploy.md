# How to deploy incus-gh-runner to production

Deploy the `incus-gh-runner` controller as a hardened systemd unit and connect it to a live GitHub scale set.

## Prerequisites

- Incus 7.0 or newer, initialized with QEMU VM support, on a host reachable at the target Incus socket. Incus 6 is not supported.
- The `incus-admin` group exists on the host (`getent group incus-admin`). Membership in this group grants root-equivalent access to the Incus socket.

    !!! warning "Root-equivalent socket access"
        `SupplementaryGroups=incus-admin` in the unit file gives the controller the same host access as a user in `incus-admin`. Run the controller on a host dedicated to this workload, not one shared with unrelated services.

- A systemd version supporting `LoadCredential=` and the `%d` credentials-directory specifier the unit relies on, along with `DynamicUser=` and the unit's other sandboxing directives. Ubuntu 24.04 is the validated reference host.
- Administrative access to a GitHub organization or repository, to create a GitHub App and its installation.
- The `incus-gh-runner` binary for your platform, and a checked-out or downloaded copy of `deploy/systemd/incus-gh-runner.service` from the repository.

## 1. Prepare Incus

Create a dedicated project for runner VMs, and profiles that give those VMs network and disk access:

```sh
incus project create github-runners
incus profile create github-runner --project github-runners
incus profile device add github-runner eth0 nic network=<your-network> --project github-runners
incus profile device add github-runner root disk pool=<your-storage-pool> path=/ --project github-runners
```

Substitute your own network and storage pool names. The project's `default` profile and the new `github-runner` profile together form the `profiles` list you will reference in the controller config.

Import a runner image into the project. See [Obtain, build, and validate runner images](./runner-images.md) for downloading a released image, building one locally, and importing it into Incus.

The controller does not create projects, networks, storage, or profiles itself — everything in this step must exist before the controller starts.

## 2. Create the GitHub App

The controller authenticates to GitHub as a GitHub App in production. Create an App with permissions sufficient to manage self-hosted runners and runner scale sets for the organization or repository at your intended `github.config_url`, then install it on that org or repository.

1. In the target organization or repository's settings, create a new GitHub App.
2. Grant it the permissions GitHub requires to manage self-hosted runner scale sets for that org or repository. The exact permission names are not fixed by this project — consult GitHub's current Actions Runner Controller / runner scale set documentation for the required scopes.
3. Install the App on the organization or repository referenced by `github.config_url`.
4. Record the App's **Client ID**.
5. Record the **Installation ID** of the installation you created in step 3.
6. Generate and download a **private key** (PEM file) for the App.

You need all three values — client ID, installation ID, and the PEM file — for the install step below.

## 3. Install the controller

Install the binary:

```sh
sudo install -m 0755 incus-gh-runner /usr/bin/incus-gh-runner
```

Install the unit file:

```sh
sudo install -m 0644 deploy/systemd/incus-gh-runner.service /etc/systemd/system/incus-gh-runner.service
```

Create the configuration directory and place the config and App private key:

```sh
sudo mkdir -p /etc/incus-gh-runner
sudo install -m 0644 config.yaml /etc/incus-gh-runner/config.yaml
sudo install -m 0600 github-app-private-key.pem /etc/incus-gh-runner/github-app-private-key.pem
```

The unit runs under `DynamicUser=yes`, so `config.yaml` must remain readable by the dynamically allocated service user (mode `0644` as shown). The private key does not need to be — the unit loads it through `LoadCredential=`, which systemd reads as root before the service starts, independent of the runtime user.

!!! warning "Do not set `private_key_file` in YAML"
    The unit's `LoadCredential=github-app-private-key:/etc/incus-gh-runner/github-app-private-key.pem` line injects the key's runtime location into the service as `INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE`. Leave `github.app.private_key_file` unset in `config.yaml` — setting it there points the controller at a path outside the credential mechanism and defeats the unit's key handling.

## 4. Write the configuration

Write `/etc/incus-gh-runner/config.yaml`. This minimal example covers the required keys plus a capacity range; see [Configuration reference](../reference/configuration.md) for every key, its default, its environment variable, and the full credential precedence rules:

```yaml
github:
  config_url: https://github.com/your-org
  scale_set: incus-gh-runner-prod
  app:
    client_id: Iv1.xxxxxxxxxxxxxxxx
    installation_id: 12345678
incus:
  project: github-runners
  image: incus-gh-runner:v0.1.0
  profiles: [default, github-runner]
  owner: incus-gh-runner-production
capacity:
  min_runners: 1
  max_runners: 4
```

- `github.config_url` is the org or repo URL the App is installed on.
- `github.scale_set` names the runner scale set; the controller creates it automatically on first start if it does not already exist.
- `incus.image` is the alias or fingerprint of the image you imported in step 1.
- `incus.owner` is an arbitrary ownership marker exclusive to this deployment — do not reuse it across independent controller instances pointed at the same Incus project. See [How incus-gh-runner works](../explanation/how-it-works.md) for how this marker gates instance deletion.

## 5. Validate the unit (optional)

From a checkout of the repository, run the bundled sandboxed check before deploying to catch unit syntax errors and hardening regressions:

```sh
deploy/systemd/verify.sh
```

This requires Linux and `systemd-analyze`; it verifies the unit definition and checks its systemd security exposure score against a fixed threshold, without touching your live system.

## 6. Start and enable the service

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now incus-gh-runner.service
```

## Verification

Confirm the controller started and connected successfully:

```sh
sudo journalctl -u incus-gh-runner -n 50 --no-pager
```

Look for a JSON log line with `msg="incus-gh-runner started"`, carrying `scale_set`, `scale_set_id`, and `incus_project` fields. Its absence, or a repeating restart loop, means startup failed — check the preceding log lines for the specific error before continuing.

Validate the deployment end to end by running a real workflow job against the scale set. In a workflow in the repository or an organization repository covered by `github.config_url`, target the scale set by name:

```yaml
jobs:
  example:
    runs-on: incus-gh-runner-prod
    steps:
      - run: echo "hello from incus-gh-runner"
```

Dispatch the job and confirm a VM is created, the job completes, and the VM is deleted afterward.

## Related

- [Operate and troubleshoot incus-gh-runner](./operate.md) — running the deployed controller, log fields, and recovering from failures.
- [Obtain, build, and validate runner images](./runner-images.md) — image acquisition, verification, and validation.
- [Configuration reference](../reference/configuration.md) — every config key, environment variable, CLI flag, and the credential precedence rules.
- [How incus-gh-runner works](../explanation/how-it-works.md) — capacity model, runner lifecycle, and the security model behind the credential and ownership rules used above.

# How to deploy incus-gh-runner to production

Deploy the `incus-gh-runner` controller as a hardened systemd unit and connect it to a live GitHub scale set using either a GitHub App or a personal access token (PAT).

## Prerequisites

- Incus 7.0 or newer, initialized with QEMU VM support, on a host reachable at the target Incus socket. Incus 6 is not supported.
- The `incus-admin` group exists on the host (`getent group incus-admin`). Membership in this group grants root-equivalent access to the Incus socket.

    !!! warning "Root-equivalent socket access"
        `SupplementaryGroups=incus-admin` in the unit file gives the controller the same host access as a user in `incus-admin`. Run the controller on a host dedicated to this workload, not one shared with unrelated services.

- A systemd version supporting `LoadCredential=` and the `%d` credentials-directory specifier the unit relies on, along with `DynamicUser=` and the unit's other sandboxing directives. Ubuntu 24.04 is the validated reference host.
- Administrative access to the target GitHub organization or repository.
- The `incus-gh-runner` binary for your platform, and a checked-out or downloaded copy of `deploy/systemd/` from the repository.

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

## 2. Choose the GitHub scope and credential

Private-repository scope is the recommended hardened starting point. Set
`github.config_url` to the exact HTTPS destination for the scale set:

- One repository: `https://github.com/OWNER/REPOSITORY`
- An organization: `https://github.com/ORGANIZATION`

A private repository URL restricts the scale set to that repository and must
use the `default` runner group. Do not expose this self-hosted runner to an
untrusted public-repository fork workflow; public repositories require a
separate threat review and workflow/approval policy. Enterprise URLs are
outside the supported controller contract. Before using organization scope:

1. Create a dedicated non-default runner group, such as `incus-gh-runner-prod`.
2. Set its repository access to **Selected repositories** and disable access to
   public repositories.
3. Enable selected-workflow access and allow only fully qualified workflow
   paths. For a SLSA builder, pin each allowed workflow as
   `OWNER/REPOSITORY/.github/workflows/build.yml@<full-commit-SHA>`.
4. Confirm the group contains only the intended scale set, then use the exact
   group name as `github.runner_group`.

GitHub documents these controls in
[Managing access to self-hosted runners using groups](https://docs.github.com/en/actions/how-tos/manage-runners/self-hosted-runners/manage-access).
The [runner-group REST fields](https://docs.github.com/en/rest/actions/self-hosted-runner-groups?apiVersion=2022-11-28)
expose the same repository, public-repository, and selected-workflow policy for
automation and verification.
The controller rejects the broad `default` group at organization scope. If the
selected-workflow restriction is unavailable, use repository scope; the
hardened organization profile depends on that control. GitHub App installation
scope controls API authority, but it is not a substitute for the runner group's
scheduling policy.

The time-limited token shown on GitHub's **New self-hosted runner** page cannot operate this controller: it registers one runner once, while `incus-gh-runner` must continuously create fresh JIT configurations for replacement VMs.

Choose one of the following renewable credential methods. GitHub Apps are preferred for independent lifecycle and rotation; a repository-scoped fine-grained PAT is the simpler option when installing an App is undesirable. GitHub maintains the current permission requirements in [Authenticating Actions Runner Controller to the GitHub API](https://docs.github.com/en/actions/how-tos/manage-runners/use-actions-runner-controller/authenticate-to-the-api).

### Option A: GitHub App

1. Create a GitHub App owned by the target organization.
2. Grant only the permissions for the selected scope:
   - Repository scope: repository **Administration: read and write** and **Metadata: read-only**.
   - Organization scope: repository **Metadata: read-only** and organization **Self-hosted runners: read and write**.
3. Install the App for the target organization or selected repository.
4. Record the App's client ID and installation ID.
5. Generate and download a private key PEM file.

### Option B: personal access token

For a single repository, create a fine-grained PAT that can access only that
repository and grant repository **Administration: read and write**. For an
organization-scoped scale set, grant organization **Administration: read** and
**Self-hosted runners: read and write**. Do not add unrelated permissions.

A classic PAT also works, but requires the broader `repo` scope for repository runners or `admin:org` for organization runners. Prefer a fine-grained PAT when GitHub makes it available for the target.

## 3. Install the controller

Install the binary and base unit:

```sh
sudo install -m 0755 incus-gh-runner /usr/bin/incus-gh-runner
sudo install -m 0644 deploy/systemd/incus-gh-runner.service /etc/systemd/system/incus-gh-runner.service
sudo install -d -m 0755 /etc/incus-gh-runner
sudo install -m 0644 config.yaml /etc/incus-gh-runner/config.yaml
```

The unit runs under `DynamicUser=yes`, so `config.yaml` must remain readable by the dynamically allocated service user. Credential files remain root-only and are exposed to the service through systemd's protected credential directory.

## 4. Write the configuration

Write `/etc/incus-gh-runner/config.yaml`. This common configuration works with either credential method:

```yaml
github:
  config_url: https://github.com/OWNER/REPOSITORY
  scale_set: incus-gh-runner-prod
  runner_group: default
incus:
  project: github-runners
  image: incus-gh-runner:v0.1.0
  profiles: [default, github-runner]
  owner: incus-gh-runner-production
capacity:
  min_runners: 1
  max_runners: 4
```

For organization scope, replace the three GitHub scheduling fields with a
dedicated group whose selected-repository and selected-workflow policy was
configured in step 2:

```yaml
github:
  config_url: https://github.com/ORGANIZATION
  scale_set: incus-gh-runner-prod
  runner_group: incus-gh-runner-prod
```

When using a GitHub App, add its non-secret identifiers beneath the existing
`github` mapping. Do not add a second `github` key; exact configuration decoding
rejects duplicate keys.

```yaml
github:
  app:
    client_id: Iv1.xxxxxxxxxxxxxxxx
    installation_id: 12345678
```

When using a PAT, do not add the `app` block. The selected systemd drop-in supplies the remaining credential path.

- `github.scale_set` names the runner scale set; the controller creates it automatically on first start if it does not already exist.
- `incus.image` is the alias or fingerprint of the image you imported in step 1.
- `incus.owner` is an arbitrary ownership marker exclusive to this deployment — do not reuse it across independent controller instances pointed at the same Incus project.

See [Configuration reference](../reference/configuration.md) for every key, default, environment variable, and credential validation rule.

## 5. Install one credential drop-in

Install exactly one credential file and its matching drop-in as `credentials.conf`.

For a GitHub App:

```sh
sudo install -m 0600 github-app-private-key.pem /etc/incus-gh-runner/github-app-private-key.pem
sudo install -d -m 0755 /etc/systemd/system/incus-gh-runner.service.d
sudo install -m 0644 deploy/systemd/credentials-github-app.conf \
  /etc/systemd/system/incus-gh-runner.service.d/credentials.conf
```

For a PAT stored in a local file named `github-token`:

```sh
sudo install -m 0600 github-token /etc/incus-gh-runner/github-token
sudo install -d -m 0755 /etc/systemd/system/incus-gh-runner.service.d
sudo install -m 0644 deploy/systemd/credentials-personal-access-token.conf \
  /etc/systemd/system/incus-gh-runner.service.d/credentials.conf
```

Do not place the App private key or PAT value in `config.yaml`. The drop-ins load the root-owned source file and point the controller at the protected runtime copy. To change methods, replace `credentials.conf`, add or remove the `github.app` identifiers in `config.yaml`, then reload and restart the service.

## 6. Validate the unit (optional)

From a checkout of the repository, run the bundled sandboxed check before deploying:

```sh
deploy/systemd/verify.sh
```

This requires Linux and `systemd-analyze`; it verifies the base unit and both credential variants, then checks the systemd security exposure score against a fixed threshold without touching your live system.

## 7. Start and enable the service

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

Validate a repository-scoped deployment end to end by running a real workflow
job in the configured repository:

```yaml
jobs:
  example:
    runs-on: incus-gh-runner-prod
    steps:
      - run: echo "hello from incus-gh-runner"
```

Dispatch the job and confirm a VM is created, the job completes, and the VM is deleted afterward.

At organization scope, select both the approved group and the scale-set label:

```yaml
jobs:
  example:
    runs-on:
      group: incus-gh-runner-prod
      labels: incus-gh-runner-prod
    steps:
      - run: echo "hello from incus-gh-runner"
```

Then inspect the runner group and
confirm that it is non-default, selected-repository only, public repository
access is disabled, and its selected workflows are commit-pinned as intended.
Before production use, dispatch the scale-set label from a disposable
unauthorized repository and from an unauthorized workflow. Neither dispatch
may create controller demand or a runner VM. If selected-workflow restriction
cannot be configured and verified, deploy at repository scope instead.

## Related

- [Operate and troubleshoot incus-gh-runner](./operate.md) — running the deployed controller, log fields, and recovering from failures.
- [Obtain, build, and validate runner images](./runner-images.md) — image acquisition, verification, and validation.
- [Configuration reference](../reference/configuration.md) — every config key, environment variable, CLI flag, and credential rule.
- [How incus-gh-runner works](../explanation/how-it-works.md) — capacity model, runner lifecycle, and the security model behind the credential and ownership rules used above.

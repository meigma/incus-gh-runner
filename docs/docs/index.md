# incus-gh-runner

incus-gh-runner is a controller that runs one-job GitHub Actions runners in ephemeral Incus virtual machines. It presents them to GitHub as a runner scale set, provisioning and tearing down a fresh VM for each job, and it deploys as a systemd service on a Linux host.

## Requirements

- Incus 7.0 or newer.
- A dedicated Linux host. The controller's identity needs `incus-admin` group membership, which is root-equivalent on that host.
- A GitHub App or personal access token authorized for the configured repository or organization.

See [Deploy to production](how-to/deploy.md) for the full host and GitHub prerequisites.

## Where to go

**Deploy it**
[Deploy to production](how-to/deploy.md) walks through the end-to-end production deployment: host prerequisites, GitHub App or PAT setup, configuration, and installing the systemd unit.

**Operate it**
[Operate and troubleshoot](how-to/operate.md) covers day-2 operations — checking runner state, reading logs, restarting the service, and troubleshooting.

**Runner images**
[Runner images](how-to/runner-images.md) covers obtaining, verifying, and importing a released reference image, plus building and validating one locally.

**Understand it**
[How incus-gh-runner works](explanation/how-it-works.md) explains the capacity model, runner lifecycle, the controller's cleanup boundary over Incus resources, its failure-handling philosophy, and its security model.

**Look up facts**

- [Configuration reference](reference/configuration.md) — every config key, environment variable, CLI flag, and how they take precedence over each other.
- [Guest contract reference](reference/guest-contract.md) — the controller-guest interface: payload and status file schemas, serial console lines, and instance metadata keys.

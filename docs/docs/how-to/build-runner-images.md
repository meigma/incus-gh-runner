# How to build a hardened runner image

Build your own Incus VM image for `incus-gh-runner`. Any image that implements
the [guest contract](../reference/guest-contract.md) works with the controller,
regardless of the tool that built it — distrobuilder, Packer, or a hand-rolled
debootstrap pipeline. This page gives the required wiring and the hardening
baseline the project settled on while maintaining its own Ubuntu 24.04 image,
so you do not have to rediscover it.

## Prerequisites

- An image build tool that can produce an Incus VM image (a unified tarball
  with `metadata.yaml` plus a `rootfs.img`, or any format `incus image import`
  accepts)
- The [guest contract reference](../reference/guest-contract.md) for the exact
  schemas your image must implement
- An Incus 7.0+ host with an existing, disposable, non-`default` project for
  boot testing

## 1. Start from a minimal server base

Install only what the boot path, the Actions Runner, and the guest components
need. The project's Ubuntu 24.04 baseline was:

- Boot and platform: `linux-virtual`, `grub-efi-amd64-signed`, `shim-signed`,
  `acpid`, `netplan.io`, `systemd-resolved`, `cloud-initramfs-growroot`
- Actions Runner runtime dependencies: `libicu74`, `libkrb5-3`,
  `liblttng-ust1t64`, `libssl3t64`, `zlib1g`, `ca-certificates`, `git`
- Guest component dependencies: `curl`, `jq` (the shipped guest entrypoint
  validates the payload with `jq`)

Do not install an SSH server, cloud-init, snapd, or anything else that opens
an inbound path or mutates the system after boot. Each VM lives for one job;
there is nothing to administer inside it.

## 2. Make the Incus agent start in the guest

The controller delivers the runtime payload through the Incus agent, so the
image must start it. Incus exposes the agent binary and its unit files to the
VM over a 9p share at boot; the image needs the mount and service wiring that
runs it (distrobuilder's `incus-agent` generator, or the equivalent
systemd units documented by Incus). A guest without a running agent never
receives a payload and is recycled as `terminal` after `incus.bootstrap_timeout`.

## 3. Install the Actions Runner under a dedicated user

Create a system account with no shell and give it the runner tree:

```sh
install -d -m 0755 /opt/actions-runner
useradd --system --home-dir /opt/actions-runner --shell /usr/sbin/nologin actions-runner
```

Download the exact Actions Runner release you intend to run and verify its
checksum before unpacking — never unpack an unverified archive into the image:

```sh
runner_version=2.335.1
curl --fail --location --silent --show-error \
  "https://github.com/actions/runner/releases/download/v${runner_version}/actions-runner-linux-x64-${runner_version}.tar.gz" \
  --output runner.tar.gz
echo "<pinned-sha256>  runner.tar.gz" | sha256sum --check --strict
tar --extract --gzip --file runner.tar.gz --directory /opt/actions-runner
chown -R actions-runner:actions-runner /opt/actions-runner
```

The runner process itself runs as `actions-runner`; only the guest supervisor
runs as root. Keep it that way — job code must not start with root privileges.

## 4. Install the guest components

The repository ships the guest-side implementation of the contract under
[`guest/`](https://github.com/meigma/incus-gh-runner/tree/master/guest).
Install the files at these paths:

| Source | Install path | Mode |
|---|---|---|
| `guest/incus-gh-runner-guest` | `/usr/local/libexec/incus-gh-runner-guest` | `0755` |
| `guest/incus-gh-runner-proof` | `/usr/local/bin/incus-gh-runner-proof` | `0755` |
| `guest/incus-gh-runner-guest.service` | `/usr/lib/systemd/system/incus-gh-runner-guest.service` | `0644` |
| `guest/incus-gh-runner-guest.path` | `/usr/lib/systemd/system/incus-gh-runner-guest.path` | `0644` |
| `guest/incus-gh-runner.conf` | `/usr/lib/tmpfiles.d/incus-gh-runner.conf` | `0644` |

Enable the path unit so the guest service starts when the payload commit
marker appears:

```sh
systemctl enable incus-gh-runner-guest.path
```

If you write your own guest implementation instead, reproduce the shipped
behavior exactly: root-only `0700` payload directory, payload validation,
deletion of `payload.json` and `payload.ready` before the runner process
starts, secret-free status file and console lines, the 30-second diagnostic
grace period, and terminal poweroff. The service unit's hardening also
carries lessons worth keeping: `UMask=0077`,
`ACTIONS_RUNNER_DISABLEUPDATE=1` (the VM is deleted after one job, so a
runner self-update is wasted work and an unpinned code path), and
`TimeoutStartSec=infinity` (job duration is bounded by GitHub and the
controller, not by systemd).

## 5. Wire the serial console

Route the kernel and guest lifecycle output to the first serial port:

```text
GRUB_CMDLINE_LINUX_DEFAULT="... console=tty1 console=ttyS0"
GRUB_TERMINAL=console
```

The controller captures the serial console as the VM's diagnostics record and
watches it for the guest lifecycle lines. An image without a working `ttyS0`
console is undebuggable in production — this is load-bearing, not cosmetic.

## 6. Reset machine identity

Every VM cloned from the image must derive a fresh identity on first boot:

```sh
echo uninitialized > /etc/machine-id
rm -f /var/lib/dbus/machine-id
```

## 7. Provide fail-closed root-disk growth

If the image's baked virtual disk is smaller than the Incus root device you
deploy with, the filesystem must grow on first boot or jobs run out of disk
mid-workload. The baseline pairs `cloud-initramfs-growroot` (partition) with
an `x-systemd.growfs` root mount option (filesystem). Whatever mechanism you
choose, boot a disposable VM with the intended root-device size and verify
the resulting filesystem size before deploying the image.

## 8. Sign the boot chain

Use the distribution's signed shim and GRUB (`shim-signed`,
`grub-efi-amd64-signed`) and install GRUB with `--uefi-secure-boot` so the
image can boot with Secure Boot enforced. The controller does not require
Secure Boot, but an image built this way costs nothing extra and keeps the
option open.

## 9. Pin and record what the build fetched

A networked image build is not reproducible: package repositories and
download URLs move underneath it. Treat integrity as a recording problem:

- Pin every downloaded artifact (the Actions Runner above, anything else you
  fetch) to an exact version and SHA-256.
- Checksum the finished image archive and keep the checksum with the archive.
- If you build in CI, attest the build (for example with GitHub artifact
  attestations) so consumers can bind the archive to the workflow that
  produced it.

Promote an image by moving the archive, its checksum, and its provenance
record together as one set.

## 10. Boot-test against the guest contract

Import the image into a disposable project and confirm the wiring before
production use:

```sh
incus --project <disposable-project> image import <archive> --alias smoke-test
incus --project <disposable-project> launch smoke-test smoke-vm --vm
```

Then verify, in order:

1. `incus --project <disposable-project> exec smoke-vm -- systemctl is-active incus-gh-runner-guest.path`
   reports `active` — the agent is up and the payload watcher is armed.
2. Write a syntactically valid payload with a bogus `jit_config` through the
   agent (see the [payload schema](../reference/guest-contract.md#payload-schema-payloadjson)),
   then create the `payload.ready` marker. The guest must transition its
   status file through `starting` to `running`, delete both payload files,
   emit the matching `incus-gh-runner-guest state=...` console lines, and —
   after the runner fails to register with the bogus credential — write a
   terminal status and power the VM off on its own.
3. Nothing secret-like appears on the serial console
   (`incus --project <disposable-project> console smoke-vm --show-log`).

Delete the instance, alias, and imported image when done. A stalled status
transition or a missing console lifecycle line means the image does not
implement the contract — see the
[guest contract reference](../reference/guest-contract.md).

!!! warning "Console diagnostics can carry sensitive output"
    The shipped guest entrypoint never writes job secrets to the serial
    console. A custom guest implementation is responsible for the same
    guarantee: everything on `ttyS0` ends up in the controller's diagnostics
    captures.

## Related

- [Guest contract reference](../reference/guest-contract.md) — the payload,
  status, console-line, and metadata schemas an image must implement
- [Configuration reference](../reference/configuration.md) — `incus.image`,
  `incus.bootstrap_timeout`, and related keys
- [How to deploy](./deploy.md) — end-to-end production deployment
- [How to operate](./operate.md) — day-2 operations, including image upgrades

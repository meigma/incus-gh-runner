# How to obtain, build, and validate runner images

Get a unified Incus VM image ready for `incus-gh-runner`: download and verify a released reference image, build one locally, or validate any image against the guest contract before deploying it.

## Prerequisites

- `gh` (GitHub CLI), authenticated against GitHub
- `incus` client access to the target Incus project
- Incus 7.0 or newer
- For local builds: a Linux host with passwordless `sudo`
- For validation: `jq`, plus an existing, disposable, non-`default` Incus project

## Use a released reference image

Set the release tag and download the image archive and its checksum:

```bash
tag=v0.1.0
version=${tag#v}
asset="incus-gh-runner-reference-image_${version}_ubuntu-24.04_x86_64.tar.xz"

gh release download "$tag" \
  --repo meigma/incus-gh-runner \
  --pattern "$asset" \
  --pattern "${asset}.sha256"
```

Verify the checksum:

```bash
sha256sum --check "${asset}.sha256"
```

Verify the release attestation:

```bash
gh attestation verify "$asset" \
  --repo meigma/incus-gh-runner \
  --signer-workflow meigma/incus-gh-runner/.github/workflows/attest.yml \
  --source-ref "refs/tags/$tag" \
  --deny-self-hosted-runners
```

If either check fails, discard the file and re-download it. Do not import an image that fails checksum or attestation verification.

Import the image into the runner project, using an alias that matches the `incus.image` value in your controller configuration:

```bash
incus --project <incus-project> image import "$asset" --alias <alias>
```

Confirm the import:

```bash
incus --project <incus-project> image list
```

The alias (or its fingerprint) appears in the listing. Set `incus.image` in the controller configuration to this value — see [Configuration reference](../reference/configuration.md).

## Build a reference image locally

Requires a Linux host, passwordless `sudo`, and this repository's mise-pinned toolchain (`distrobuilder` 3.3.1, checksum-pinned in `mise.lock`). Install the pinned toolchain first if you have not already:

```bash
mise install
```

Build the image into an empty output directory:

```bash
mise exec -- image/build.sh <output-dir>
```

`<output-dir>` is created if it does not exist; the build refuses to write into a directory that already has contents. The build does not boot a VM and needs no KVM. It produces:

- `<output-dir>/incus-gh-runner-ubuntu-24.04-x86_64.tar.xz` — the unified VM image archive
- `<output-dir>/incus-gh-runner-ubuntu-24.04-x86_64.tar.xz.sha256` — its checksum

Verify the checksum before using the archive:

```bash
cd <output-dir>
sha256sum --check incus-gh-runner-ubuntu-24.04-x86_64.tar.xz.sha256
```

Import the resulting archive the same way as a released image (see above), or validate it first.

### Build integrity and reproducibility

The reference-image build is networked and non-hermetic. It resolves Ubuntu
packages from the configured live `noble` repositories and downloads the
Actions Runner archive while the build is running. The repository pins the
Actions Runner version and archive checksum, and mise pins `distrobuilder`, but
the Ubuntu repository snapshot and complete resolved package set are not yet
pinned or recorded. Building without network access is not supported.

Consequently, two builds from the same source commit may contain different
Ubuntu package revisions and are not expected to produce byte-identical image
archives. A release checksum and GitHub build attestation bind an operator to
the exact archive produced by the release workflow; they do not establish a
reproducible or offline build. Retain that checksum, attestation, and archive as
one set when promoting an image.

### Root disk growth

The reference archive starts with an 8 GiB virtual disk. On first boot,
`cloud-initramfs-growroot` expands its root partition to the Incus root-device
size, and the `x-systemd.growfs` mount option expands the ext4 filesystem. This
makes the CUE module's `inputs.runners.rootDiskGiB` setting effective inside
the guest instead of exposing only a larger virtual block device.

Custom images must provide an equivalent, fail-closed partition and filesystem
growth path when their baked disk is smaller than the configured Incus root
device. Boot a disposable VM with the intended root-device size and verify the
resulting filesystem before deploying a custom image.

## Validate any image

Run the guest-contract probe against any unified image archive — a released image, a locally built one, or a custom image — before putting it into production.

```bash
image/validate-incus.sh <incus-project> <path-to-archive>
```

Requires Incus 7.0 or newer and an existing, disposable, non-`default` Incus project. The script creates and deletes an image and an instance within that project; it refuses to run against `default`.

The script imports the archive under a generated alias, launches one VM, and drives it through the full guest contract: payload delivery, status-file transitions, serial console lifecycle lines, absence of secrets on the console, and clean poweroff. It then deletes exactly the instance, alias, and image it created — nothing else in the project is touched.

This validates the guest protocol and lifecycle only. It does not validate
larger-root growth, Secure Boot enforcement, host or network isolation, or
resource ceilings.

!!! warning "Console diagnostics can carry sensitive output"
    The reference image's guest script never writes job secrets to the serial console, and `image/validate-incus.sh` checks for a known probe secret to confirm this. A custom guest image is responsible for the same guarantee — the probe checks for its own injected secret and does not catch every possible leak from a non-conforming image.

On success the script prints `Incus guest contract validated for <archive>` and exits `0`. Any contract deviation exits non-zero with the failing check reported on stderr.

### Custom images

Any image that satisfies the guest contract works with `incus-gh-runner`, regardless of how it was built. Validate it with `image/validate-incus.sh` before deploying it. See [Guest contract reference](../reference/guest-contract.md) for the payload, status, and console-line schemas your image must implement.

## Troubleshooting

### `distrobuilder is not on PATH`
Run the build through `mise exec --`, not directly, and make sure `mise install` has completed.

### `output directory must be empty`
Point `image/build.sh` at a new or empty directory.

### `sha256sum: ... computed checksum did NOT match`
The download is corrupt or incomplete. Delete both files and re-download.

### `gh attestation verify` fails
Confirm `$tag` matches the release you downloaded and that `$asset` is the exact file you checksummed. Do not import the image.

### `refusing to run against the default Incus project`
Pass a dedicated, disposable project to `image/validate-incus.sh`; it will not run against `default`.

### `Incus server 7.0 or newer is required`
Upgrade the Incus daemon on the host running the validation before retrying.

### Guest contract validation fails partway
Inspect the failing check reported on stderr. A stalled status transition or a missing console lifecycle line points to a guest image that does not implement the contract — see [Guest contract reference](../reference/guest-contract.md).

## Related

- [Configuration reference](../reference/configuration.md) — `incus.image` and related keys
- [Guest contract reference](../reference/guest-contract.md) — payload, status, and console-line schemas
- [How to deploy](./deploy.md) — end-to-end production deployment
- [How to operate](./operate.md) — day-2 operations and troubleshooting

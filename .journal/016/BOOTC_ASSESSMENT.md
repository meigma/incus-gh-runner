# bootc image migration assessment

Date: 2026-07-20
Decision: rejected; retain distrobuilder

## Question

Evaluate whether the reference Incus VM should move from distrobuilder to bootc
so OCI becomes the authored system-image format and can participate more
directly in the project's attestation workflow.

## Experiment

The spike ran on a disposable Latitude `c3.small.x86` bare-metal server in
Mexico City with 6 cores / 12 threads, 32 GB RAM, an 800 GB NVMe disk, Ubuntu
24.04, Linux 6.8.0-136-generic, `/dev/kvm`, and Incus 7.0.1.

The prototype:

1. Built an OCI bootc image containing GitHub Actions runner 2.335.1 and the
   existing `incus-gh-runner` guest contract.
2. Converted the OCI image into qcow2 with the unified osbuild
   `image-builder` CLI.
3. Wrapped the qcow2 plus `metadata.yaml` as an Incus unified VM image.
4. Ran the repository's exact `image/validate-incus.sh` acceptance validator.

No prototype code was committed or proposed for merge. The implementation
worktree was `feat/bootc-image-experiment`.

## Results

Fedora 44 passed the complete guest contract. Incus imported and booted the VM,
the Incus agent became ready, the validator delivered a transient JIT-shaped
payload, the guest reported running and terminal success, the secret was absent
from the serial console, and the VM powered itself off.

The final passing artifact measurements were:

| Item | Result |
| --- | --- |
| OCI image ID | `c9c39a86630b827243837c6a3571c79e42e05a6056e09a07da015fa409a606ad` |
| OCI digest | `sha256:eb2f763a32d31c2c3687a6ccc6c45e77ed7e9a7da17d1c0d469564a6d02b8abe` |
| OCI size | 2,775,646,013 bytes |
| qcow2 virtual size | 12,339,642,368 bytes |
| qcow2 allocated size | 1,847,889,920 bytes |
| Incus archive size | 1,810,133,856 bytes |
| Incus archive SHA-256 | `88e69c47d9a54e6f0fc93cd145c610e8e525661c1df460f68967d57680efdc23` |
| Fresh Fedora OCI build | about 1 minute 10 seconds |
| Cached OCI rebuild | 13.8 seconds |
| OCI-to-qcow2 conversion | about 3 minutes |
| qcow2-to-Incus packaging | 1 minute 12 seconds |
| `bootc container lint` | 9 passed, 1 skipped, 4 hygiene warnings |
| Incus guest contract | passed |

CentOS Stream 10 was tested first. It booted under raw QEMU/KVM, but its kernel
has `CONFIG_NET_9P` disabled. Incus 7.0.1 uses a 9p share for its VM agent, so
the guest never became manageable by Incus. Fedora 44 included the necessary
9p modules.

## Integration costs discovered

- osbuild filled the GPT disk through the final usable sector, while Incus
  runs `sgdisk --move-second-header` during import. Adding 2 MiB of trailing
  disk space before packaging was required.
- The unified image builder required an explicit default filesystem. Fedora's
  XFS userspace produced features unsupported by the Ubuntu 6.8 build-host
  kernel; `--bootc-default-fs ext4` worked.
- The Incus agent copied from its 9p shares retained `nfs_t` on Fedora. SELinux
  blocked execution until its tmpfs was mounted with a `bin_t` context.
- bootc makes `/opt` read-only. The existing runner contract writes under
  `/opt/actions-runner`, requiring that path to become a compatibility symlink
  into `/var/opt/actions-runner`.
- Direct OCI signing or attestation covers the source OCI image, not the
  derived qcow2 and Incus archive. A production design would still need a
  conversion attestation binding the OCI digest to the final archive checksum.
- The working Fedora path would also change the guest distribution from Ubuntu
  to Fedora and introduce bootc `/var` lifecycle considerations across updates.

## Decision

The maintainer rejected the bootc migration after reviewing these results. The
working prototype demonstrated technical feasibility, but the extra conversion
bridge, kernel and SELinux compatibility work, larger artifact, distribution
change, and remaining derived-artifact provenance boundary outweighed the OCI
benefit for this project.

Retain the existing distrobuilder-based Ubuntu 24.04 reference-image plan. Do
not treat bootc as an open migration thread unless a future maintainer
explicitly reopens the decision with materially different requirements or
tooling.

## Cleanup

The Latitude server was deleted and confirmed absent through the Latitude API.
Its large OCI, qcow2, and Incus archive artifacts were intentionally discarded.
The temporary implementation worktree and branch were abandoned without a PR.

## External references

- [bootc filesystem model](https://bootc.dev/bootc/filesystem.html)
- [osbuild bootc-image-builder deprecation notice](https://osbuild.org/docs/bootc/deprecation-notice/)
- [Incus image format](https://linuxcontainers.org/incus/docs/main/reference/image_format/)


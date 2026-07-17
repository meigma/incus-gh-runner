---
title: Reusable Incus runner image
status: working-proposal
date: 2026-07-17
session: 001
---

# Reusable Incus runner image

## Position

This repository should publish a reusable Incus VM image alongside the
controller. The image is a convenient reference implementation and starting
point for downstream customization, not a mandatory controller dependency.

The controller must be able to use any operator-supplied image that satisfies
the eventual guest contract. It does not own how that image is built or which
workflow toolchain it contains.

## Working decisions

- Build the reference image declaratively with
  [`lxc/distrobuilder`](https://linuxcontainers.org/distrobuilder/docs/latest/),
  using `build-incus --vm` rather than Canonical's LXD-focused imagebuilder.
- Assemble the VM disk offline. The build may use root, loop devices, mounts,
  a chroot, and `qemu-img`, but it does not boot a VM or require KVM.
- Start with an Ubuntu 24.04 LTS `x86_64` image and defer cross-architecture
  builds until the first image works end to end.
- Publish a unified Incus image tarball and SHA-256 checksum as versioned
  release assets.
- Pin the GitHub Actions runner version in the image definition so each image
  release is reproducible and auditable.

These are working choices. The first build and boot spike may change them.

## Minimum guest contract

The controller proposal will define the exact bootstrap interface. Regardless
of its transport, a compliant image must:

- boot unattended as an Incus VM in a preconfigured environment;
- accept one runtime-provided GitHub JIT runner configuration;
- start one Actions runner for one job without embedded GitHub credentials;
- remove or otherwise protect transient JIT material after use;
- power off after the runner process exits; and
- leave enough guest or console diagnostics for the controller to collect
  before deleting the instance.

The reference image should also be safe to clone: machine identity and other
host-specific state must be initialized per instance rather than baked into the
artifact.

The precise configuration transport, filesystem paths, readiness signal, and
diagnostic collection mechanism remain open until they are proven together
with the controller.

## Build pipeline

The intended hosted build is approximately:

1. Run directly on a standard `ubuntu-24.04` GitHub-hosted VM, not
   `ubuntu-slim` and not inside a job container.
2. Install a pinned distrobuilder and its filesystem, partitioning, and QEMU
   utility dependencies.
3. Fetch and verify the pinned GitHub Actions runner archive.
4. Run:

   ```bash
   sudo distrobuilder build-incus image.yaml build/ \
     --vm \
     --type=unified
   ```

5. Generate the checksum and upload the image as a workflow artifact or release
   asset.

This path is compatible with the absence of KVM because distrobuilder creates,
mounts, populates, and converts the disk without starting it. It does require
passwordless root access and usable loop devices. A small hosted-CI spike must
prove those assumptions before the release workflow is designed around them.

## Validation and release

Artifact construction and functional validation are separate gates:

- **Hosted build:** prove the image definition is reproducible and produce the
  tarball plus checksum.
- **Incus validation:** import the artifact, launch a VM, exercise the guest
  bootstrap, observe the expected poweroff, and check for transient secret
  residue.

The second gate requires an Incus-capable environment. It can be manual for the
first spike and later move to a dedicated self-hosted validation runner. A
successful hosted build alone does not prove the image boots.

## Reference image contents

Keep the initial image deliberately small:

- the base operating system and kernel needed for an Incus VM;
- the Incus guest agent and per-instance initialization support;
- the pinned GitHub Actions runner and its documented runtime dependencies;
- a one-shot bootstrap service implementing the guest contract; and
- only the tools needed by the first real validation workflow.

Large language SDKs and an attempt to mirror GitHub's hosted runner image are
out of scope. Downstream consumers should extend the checked-in distrobuilder
definition or use their own compliant image.

## First delivery slices

1. Build a minimal unified VM image on `ubuntu-24.04` without KVM.
2. Import and boot it manually in Incus; prove unattended startup and poweroff
   with a disposable payload.
3. Finalize the guest bootstrap contract with the controller's first real JIT
   job.
4. Publish the proven artifact and checksum in a release.

## Open questions

- Which controller-to-guest transport should carry the JIT configuration?
- What readiness and diagnostic signals are reliable across controller
  restarts?
- Should the reference image include Docker, or should that be a downstream
  variant?
- Will the resulting compressed image comfortably fit GitHub-hosted runner
  disk and release-asset limits?
- When should native `arm64` builds enter the release matrix?

## References

- [Distrobuilder documentation](https://linuxcontainers.org/distrobuilder/docs/latest/)
- [Distrobuilder VM build requirements](https://linuxcontainers.org/distrobuilder/docs/latest/howto/install/)
- [Incus image format](https://linuxcontainers.org/incus/docs/main/reference/image_format/)
- [GitHub-hosted runner privileges](https://docs.github.com/en/actions/reference/runners/github-hosted-runners#administrative-privileges)

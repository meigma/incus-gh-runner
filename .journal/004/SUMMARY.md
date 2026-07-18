---
id: 004
title: Continue phase 2 guest image work
date: 2026-07-17
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 002, 003]
---

## Goal
Continue the v1 implementation plan with the next phase 2 evidence slice: prove a reproducible reference VM image and a disposable one-shot guest contract for receiving and consuming GitHub Actions Runner JIT configuration.

## Outcome
The goal was met for the planned evidence slice. PR #9 landed the reference image, guest lifecycle contract, local compliance tests, hosted offline build proof, and an Incus-capable live validator on `master` as squash commit `85f273a`. The separate live Incus boot validation remains open because the development Mac has no Incus daemon.

## Key Decisions
- Use a root-only payload file followed by a ready marker as the commit point -> Incus agent file transfer is simple, observable, and avoids cloud-init timing ambiguity.
- Delete transient payload files before starting the runner -> the JIT configuration survives only in the ephemeral runner process after handoff.
- Expose secret-free status plus serial-console lifecycle transitions and always power off terminally -> the controller can diagnose running and stopped guests without logging credentials.
- Separate offline construction proof from live boot proof -> hosted GitHub runners can build and inspect a unified qcow2 image without KVM, while `image/validate-incus.sh` owns the destructive live lifecycle gate.
- Manage distrobuilder through mise as a Linux-only checksum-backed source tool -> project tooling stays pinned, verified, cached, and outside `image/build.sh` even though upstream publishes no binary.

## Changes
- `image/image.yaml` - defines the Ubuntu 24.04 x86_64 Incus VM, pinned Actions Runner, Incus agent, and one-shot systemd guest units.
- `image/guest/` - validates the versioned payload, removes transient inputs, runs the runner unprivileged, reports lifecycle state, and powers off on every terminal path.
- `image/build.sh` - consumes the mise-provided distrobuilder executable and creates a checksummed unified Incus VM archive.
- `image/validate-incus.sh` - safely imports, boots, injects, observes, and removes a disposable live validation VM in a non-default Incus project.
- `image/tests/guest-entrypoint-test.sh` - proves success, fail-closed validation, secret cleanup, diagnostics, poweroff, and mise ownership locally.
- `mise.toml` and `mise.lock` - install distrobuilder 3.3.1 from its verified source release and compile it with pinned mise Go and the required stub tags.
- `.github/workflows/reference-image.yml` - builds and inspects the image without KVM and uploads a one-day proof artifact.
- `docs/docs/reference-image.md` and `README.md` - document the image, payload contract, proof boundary, diagnostics, and validation commands.

## Open Threads
- Run `image/validate-incus.sh` with the merged artifact in an explicitly disposable Incus project and capture real import, boot, agent handoff, cleanup, console, and shutdown evidence.
- Exercise a genuine GitHub Actions Runner JIT registration only in the later controller-to-guest end-to-end phase; the current live validator deliberately uses a credential-free runner probe.

## Lessons
- mise tool-level `depends` orders installation but does not put that dependency on post-install hook `PATH`; source-build hooks must resolve the exact mise-managed compiler rather than accepting a system fallback.
- A checksum-backed mise HTTP tool plus post-install compilation preserves release-source integrity and caching when upstream does not publish executable artifacts.

## References
- [PR #9](https://github.com/meigma/incus-gh-runner/pull/9)
- [Exact-head Reference Image run 29627521172](https://github.com/meigma/incus-gh-runner/actions/runs/29627521172)
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`
- `.journal/001/IMAGE_PROPOSAL.md`

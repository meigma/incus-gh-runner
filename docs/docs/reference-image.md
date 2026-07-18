# Reference runner image

The repository's reference image is an Ubuntu 24.04 x86_64 Incus VM assembled
offline with distrobuilder. It contains Actions Runner 2.335.1 and a one-shot
guest service, but no GitHub credential or registration state.

This image is a reference implementation. The controller may use any operator
image that satisfies the guest contract below.

## Build proof

`image/build.sh` builds distrobuilder 3.3.1 from its checksummed, vendored source
release and then creates a unified Incus VM tarball:

```sh
image/build.sh build/reference-image
```

The command requires a Linux host with passwordless `sudo` and distrobuilder's
VM runtime dependencies. It does not start a VM or require `/dev/kvm`. The
`Reference Image` workflow runs the build on `ubuntu-24.04`, verifies the
archive checksum and qcow2 virtual size, and retains the proof artifact for one
day.

An offline build proves construction only. Boot and guest-lifecycle validation
must run separately against a disposable Incus project.

## Controller-to-guest contract

A compliant image must provide an Incus agent and this one-shot exchange:

1. The controller creates and starts the VM, then waits for the Incus agent.
2. It writes `/run/incus-gh-runner/payload.json` as root with mode `0600`.
3. After the payload write is complete, it creates
   `/run/incus-gh-runner/payload.ready`. The ready marker is the commit point;
   the controller must never create it before the payload is complete.
4. The guest validates and consumes the payload, deletes both input files, and
   starts the pinned runner as the unprivileged `actions-runner` user.
5. The guest writes its current state to
   `/run/incus-gh-runner/status.json`, emits secret-free state transitions to
   the serial console, and powers off after the runner exits.

The payload schema is intentionally small:

```json
{
  "version": 1,
  "jit_config": "<one-runner JIT configuration>"
}
```

Unknown fields, unsupported versions, and empty JIT configurations fail
closed. The guest never logs the JIT configuration. Both payload files live on
the `/run` tmpfs and are removed before the runner process starts. The JIT
configuration remains present in the runner process arguments while the runner
is active because that is the upstream runner interface; it disappears with
the ephemeral process and VM.

The status document has `version`, `state`, and an optional `exit_code`:

```json
{"version":1,"state":"running"}
```

The controller can pull this file through the Incus agent while the VM is
running. After terminal poweroff, it can collect the Incus serial-console log;
the service writes only lifecycle states and exit codes there. Runner job logs
and the transient payload are not part of this diagnostic channel.

## Compliance boundary

An operator-supplied image is compliant when it:

- boots unattended as an Incus VM and exposes the Incus agent;
- implements the versioned payload and ready-marker exchange above;
- runs exactly one JIT-configured Actions runner as a non-root user;
- removes transient payload files before starting the runner;
- exposes status through the Incus agent without including secrets;
- emits secret-free lifecycle diagnostics to the serial console; and
- powers off on success, runner failure, or invalid payload.

The image must also reset machine identity when cloned and must not contain a
GitHub App key, token, JIT configuration, or previous runner registration.

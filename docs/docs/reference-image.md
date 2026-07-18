# Reference runner image

The repository's reference image is an Ubuntu 24.04 x86_64 Incus VM assembled
offline with distrobuilder. It contains Actions Runner 2.335.1 and a one-shot
guest service, but no GitHub credential or registration state.

This image is a reference implementation. The controller may use any operator
image that satisfies the guest contract below.

## Build proof

mise verifies the pinned distrobuilder 3.3.1 source release, compiles its
vendored dependencies once into the tool cache with the optional container
integrations disabled, and puts the resulting binary on `PATH`.
`image/build.sh` then creates a unified Incus VM tarball:

```sh
mise exec -- image/build.sh build/reference-image
```

The command requires a Linux host with passwordless `sudo` and distrobuilder's
VM runtime dependencies. It does not start a VM or require `/dev/kvm`. The
`Reference Image` workflow runs the build on `ubuntu-24.04`, verifies the
archive checksum and qcow2 virtual size, and retains the proof artifact for one
day.

An offline build proves construction only. Boot and guest-lifecycle validation
must run separately against a disposable Incus project.

## Incus boot validation

On an Incus-capable host, validate the built artifact against an explicitly
named disposable project:

```sh
image/validate-incus.sh incus-gh-runner-test build/reference-image/incus-gh-runner-ubuntu-24.04-x86_64.tar.xz
```

The daemon must be Incus 6.5 or newer because earlier releases cannot retrieve
virtual-machine console history.

The validator refuses the `default` project. It imports the archive under a
unique alias, launches one uniquely named VM, waits for the Incus agent, and
temporarily replaces the runner entrypoint with a slow probe. It then exercises
the real payload/ready-marker exchange, observes the running status, verifies
that both transient input files are already gone, waits for guest-driven
poweroff, and checks the live VM's serial history during its bounded diagnostic
grace window for the expected lifecycle and absence of the probe secret. Its
exit trap deletes only the exact instance and
alias it created, and deletes the image fingerprint only when that fingerprint
was not present before the run.

This probe validates the image boot and guest contract without consuming a
GitHub credential. A genuine Actions Runner JIT registration remains the phase
4 end-to-end proof.

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
   the serial console, gives the controller 30 seconds to collect terminal
   diagnostics, and powers off after the runner exits.

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
running. When it observes terminal status, it collects the live Incus
serial-console history before deleting the VM. If the controller is unavailable,
the guest powers itself off after the bounded grace window. The service writes
only lifecycle states, exit codes, and the grace duration there. Runner job
logs stay in the guest journal and are not part of this diagnostic channel; the
transient payload is never written to either channel.

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

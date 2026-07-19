# Slice 2 live evidence

This is the retained evidence index for the first Incus-isolation proof
increment. It is not the full Slice 2 exit gate: the disposable host had no
`/dev/kvm`, so hostile VM dataplane, IPv6 spoofing, and resource-exhaustion
tests remain open.

## Incus isolation baseline

Evidence: `evidence/slice2-baseline-incus-7.0.1/`

- Incus server/client: 7.0.1, standalone, nftables, no network API listener.
- Storage: dedicated existing ZFS pool; the effective response includes the
  server-generated `volatile.initial_source` field normalized by the validator.
- Network: host-owned bridge and ACL in the `default` project.
- Runner boundary: restricted project with inherited allowlisted bridge,
  project-local runner profile, explicit instance/project ceilings, dual ACL
  attachment, anti-spoofing, and port isolation.
- Result: the read-only validator matched the effective API state.
- Negative result: changing the effective network default egress action from
  `reject` to `allow` caused `network drift detected`; restoring `reject`
  returned the validator to green.

Run `sha256sum --check checksums.sha256` inside that directory to verify the
bundle. `disposable-apply.sh` records the one-off API application sequence used
on the disposable host; it is evidence, not supported production automation.

## Project-restricted TLS authority

Evidence: `evidence/slice2-authority-incus-7.0.1/`

The restricted certificate was bound to only the disposable runner project,
the client pinned the exact server certificate, and `/var/lib/incus/server.ca`
was absent. The following passed:

- project-scoped inventory;
- create, start, asynchronous operation waits, guest-agent file push/pull,
  console-log read, stop, and delete;
- foreign-project filtering and project-metadata denial;
- denial of project-restriction weakening, global configuration mutation, and
  certificate self-escalation;
- denial of host-path disks, Unix devices, unmanaged NICs, extra disks,
  `raw.qemu`, forbidden profile devices, and containers; and
- exact leaf-certificate revocation.

The result proves the container-compatible API authority path, not KVM boot or
VM guest-agent readiness. It also exposed a production blocker: the restricted
identity can still mutate project-local profiles, including direct NIC ACL,
anti-spoofing, and port-isolation settings. The host-owned bridge/ACL narrows
external egress authority, but cross-runner controls remain mutable. The
current release therefore keeps the dedicated-host requirement and does not
replace `incus-admin` with this incomplete boundary.

`reproduce.sh` is the exact disposable-host spike. It contains no client
private key; the generated key was removed with the test configuration.

## Teardown

The exact Multipass VM `incus-gh-runner-slice2-20260718` and all of its Incus
projects, instances, certificates, images, networks, pools, and temporary
files were permanently purged after the checksummed evidence was copied here.

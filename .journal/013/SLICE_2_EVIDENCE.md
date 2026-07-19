# Slice 2 live evidence

This is the retained evidence index for the Incus-isolation proof increments.
The exact-head paid-host run now closes the KVM, hostile dataplane, IPv6
no-bypass, admission-ceiling, and bounded resource-survival gates. Slice 2 is
still not complete because the controller identity can mutate project-local
profiles and therefore cannot yet be described as least authority.

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

## KVM runtime and hostile-runner acceptance

Evidence: `evidence/slice2-runtime-incus-7.0.1/`

The final run used PR #29 head
`3d787dc1a0aac7a59e34b68e4ebc4f318ee7854f`, helper digest
`93fa9da2ced9718f8a4f5fde171f3a091eaaea68cc14359e0aef1a9384930e60`,
baseline digest
`c2aac4737d94483bf308fa356546c7c50499a0ec51c3aa261397a47126c438d2`,
and the exact-head reference-image fingerprint
`ae1e2b082d50b4f6daf6bdf35561f12b170fd20cacfc09ffcf2a4149c330db1a`.
Hosted CI, CodeQL, Pages, Kusari Inspector, and the image build were green on
that head.

The disposable c3.small.x86 bare-metal host ran Ubuntu 24.04, Incus 7.0.1,
nftables, a file-backed disposable ZFS pool, and the rendered two-runner CUE
baseline. The passing run proved:

- two concurrent VMs used the exact profile and image, reached the agent, ran
  under KVM, held `/dev/kvm` open from the exact Incus-reported QEMU PIDs,
  reported Secure Boot active, exposed no `/dev/incus`, and grew the 8 GiB
  image root filesystem to about 20 GiB;
- a third VM was rejected by the exact two-VM project ceiling;
- self-assigned, alternate-source, and link-local IPv6 could not reach the
  peer while guest-local positive controls and approved IPv4 recovery passed;
- cross-runner TCP and neighbor resolution, host-canary access, unauthorized
  direct/proxied destinations, MAC spoofing, and IPv4 source spoofing were
  blocked while approved GitHub proxy traffic recovered; and
- one VM sustained eight CPU workers, a 2 GiB memory hold, and a 1 GiB
  synchronous disk workload for ten minutes while all 602 Incus API and peer
  samples succeeded. API p95/max latency was 21.6/34.7 ms, peer max gap was
  1.13 s, minimum host `MemAvailable` was 24,386,736,128 bytes, ZFS stayed
  healthy, and neither Incus nor the host kernel reported a resource failure.

Both the parent and helper checksum manifests verify. The host receipts also
show zero final instances, a passing production validator before the
disposable marker, the exact imported image, and a healthy final zpool. The
retained limitations are intentional: the gate does not attribute IPv6 drops
to one overlapping control, execute an untrusted EFI payload, benchmark NIC or
ZFS throughput limits, prove aggregate project CPU/memory runtime throttling,
or establish least-privilege Incus authorization.

## Teardown

The exact Multipass VM `incus-gh-runner-slice2-20260718` and all of its Incus
projects, instances, certificates, images, networks, pools, and temporary
files were permanently purged after the checksummed evidence was copied here.

The exact paid bare-metal server `sv_ZozMazxYBN7kw` was also permanently
deleted after the final archive was copied and reverified. The provider API
then returned `404 NotFound`, and the server was absent from the project list;
the redacted teardown receipt is retained with the runtime evidence.

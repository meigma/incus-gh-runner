# Incus isolation baseline

This directory contains a reviewable desired-state example, a CUE policy module,
and a read-only drift validator for a single-purpose Incus 7 runner host. None
of them configure or mutate Incus. Render or adapt an environment-specific
baseline, apply it through the Incus CLI or your infrastructure-management
system, then validate the effective API state.

The example establishes:

- a dedicated restricted project with explicit aggregate VM, CPU, memory,
  network, and per-pool disk ceilings;
- one host-owned managed bridge and network ACL, one project-local profile, and
  one dedicated ZFS pool;
- per-VM CPU, memory, root-disk, network-bandwidth, and requested disk-I/O
  limits;
- MAC and IPv4 anti-spoofing, explicit NIC-level IPv6 address denial, IPv6
  filtering, and bridge-port isolation;
- a network ACL attached to both the host-owned bridge and the profile NIC,
  with rejected and logged unmatched ingress and egress; and
- egress only to an explicit DNS resolver and HTTP CONNECT proxy.

Incus 7.0.1 rejects creation of a managed bridge in a non-default project;
only OVN networks can be created and managed there. This standalone-host
baseline therefore sets `features.networks=false` and `limits.networks=0` in
the runner project. A host administrator owns the bridge and ACL in the
`default` project, and `restricted.networks.access` allowlists only that bridge
for the runner project.

Both ACL attachment points are intentional. The host-owned network attachment
keeps the external-traffic ACL on the bridge if the project-local profile omits
its copy. However, Incus bridge ACLs applied only at network level cannot
enforce intra-bridge policy. The `network_bridge_acl_devices` extension applies
the same ACL directly at the bridged NIC, and
`security.port_isolation=true` independently blocks communication between
isolated runner ports. The validator requires the network-level and NIC-level
default actions to reject and log unmatched traffic.

## Adapt the example

The dependency-free module under [`cue/`](cue/) accepts a closed set of names,
host capacity, runner sizing, network endpoints, and storage inputs. It derives
aggregate limits and emits a complete baseline while keeping the security
controls non-overridable. It also emits the controller project, sole profile,
and `capacity.max_runners` as one partial configuration so those values cannot
drift from the baseline. Its default example is continuously checked for
semantic equality with `baseline.example.json`.

Registry publication is not part of this proof increment. Until the `@v0`
module interface is reviewed and published, `baseline.example.json` remains the
portable deployment artifact. Copy it outside the checkout and change every
environment-specific value before applying it:

- replace the `192.0.2.10/32` proxy and `192.0.2.53/32` DNS documentation
  addresses with dedicated endpoint IPv4 `/32` CIDRs;
- replace the bridge subnet, names, ZFS `source` and `zfs.pool_name`, and
  capacity limits; managed bridge names must be 2 to 15 characters, start with
  a lowercase letter, and otherwise contain only lowercase letters, digits, or
  hyphens; the storage source must be a dedicated existing zpool or dataset
  under Incus control;
- size aggregate limits below physical host capacity so Incus, the controller,
  and the host retain explicit CPU, memory, and disk headroom; keep the VM
  count at or above `capacity.max_runners`, and size aggregate CPU, memory, and
  disk for that many profile-limited VMs;
- configure the runner listener and job tooling to use the proxy by IP; and
- configure the proxy itself to allow only GitHub or GHES plus explicitly
  approved dependency destinations.

Create and manage the bridge and network ACL in the Incus `default` project;
create the restricted project and `github-runner` profile in the named runner
project. Do not move the bridge or ACL into that runner project. The validator
queries their API objects with `project=default` and queries the profile with
the restricted project name.

Incus ACL rules match addresses and ports, not DNS names. Do not replace the
proxy rule with unrestricted TCP/443 and describe that as GitHub allowlisting.
The example documentation addresses are deliberately non-routable, so an
unchanged example fails closed by having no useful external connectivity.
The bridge keeps DHCP but sets `raw.dnsmasq=port=0` so runners cannot bypass the
declared resolver through the bridge host's DNS forwarding service.

Load `br_netfilter` before starting runner VMs and persist it through reboots:

```console
sudo modprobe br_netfilter
printf 'br_netfilter\n' | sudo tee /etc/modules-load.d/incus-gh-runner.conf >/dev/null
test -d /sys/module/br_netfilter
```

Incus requires bridge netfilter when the profile enables IPv4 or IPv6 address
filtering. The read-only API validator cannot observe kernel-module state; the
operator must verify the module after provisioning and after every reboot.

The aggregate project CPU and memory values are admission budgets: Incus uses
the declared per-instance limits when deciding whether another VM fits. They
are not runtime aggregate throttles that dynamically divide CPU or memory
among already-running VMs. Keep explicit physical host headroom even when the
project's derived totals are correct.

Use only the named `github-runner` profile in the controller configuration.
Do not add a second controller profile: an additional profile can add devices
or relax limits after this validator has checked the reference profile. During
preflight the controller pins this profile's effective configuration and
devices, revalidates its digest before create, and materializes that snapshot
directly into each VM with no mutable profile attachment.

Incus 7.0 through 7.2 do not advertise the
`projects_restricted_virtual_machines_nesting` extension. On those supported
versions, the exact profile's `security.nesting=false` setting is the explicit
compensating control; there is no project-level VM nesting restriction to set.
The validator reports this residual on every successful run and fails once a
server advertises the future extension, forcing the baseline to adopt
`restricted.virtual-machines.nesting=block` instead of silently retaining the
weaker compatibility path.

## Validate without changing Incus

The installed controller binary also provides a standalone validator:

```console
incus-gh-runner validate /etc/incus-gh-runner/incus-baseline.json
```

It compiles the embedded CUE policy in process, checks the rendered baseline
against that policy, and reads the effective Incus state through the local Unix
socket. It does not invoke external `cue`, `incus`, or `jq` executables. The
validator uses only read operations and never creates, changes, or deletes
Incus resources. Its default socket is `/var/lib/incus/unix.socket`; select a
different local socket explicitly when required:

```console
incus-gh-runner validate --socket /run/incus/unix.socket /etc/incus-gh-runner/incus-baseline.json
```

`validate` does not load the controller YAML configuration or require GitHub
credentials. Its inputs are the explicit baseline path and local socket path.
Socket access is still root-equivalent, so run it only from a trusted host
administration context.

It rejects malformed manifests, missing API extensions, non-`nftables`
firewalls, clustered hosts, any Incus network API listener, and any effective
project, network, ACL, profile, or storage-pool drift. The only accepted
authority is `dedicated-host-unix-socket` with both HTTPS listener settings
empty. The storage comparison ignores only the server-generated
`volatile.initial_source` field observed on Incus 7.0.1; `source`,
`zfs.pool_name`, and every other effective storage setting remain fail-closed.

The rendered baseline records the resource ceilings derived by CUE, but not the
physical host totals and reserved-headroom inputs used to derive them. Runtime
validation can detect drift in those effective ceilings; it cannot re-measure
or re-prove physical-host headroom. Re-render and review the baseline whenever
host capacity or reservations change.

The current Unix-socket controller connection remains root-equivalent, so this
baseline requires a dedicated single-purpose host and treats controller
compromise as host compromise. A disposable Incus 7.0.1 authority spike proved
the container-compatible inventory, create, start, guest-agent file transfer,
console, stop, delete, project-denial, restriction, and certificate-revocation
paths through a project-restricted TLS identity. It did not prove KVM boot or
VM guest-agent readiness. The `user.incus-gh-runner.owner` key scopes cleanup;
it is not authorization against another Incus writer.

Project-restricted TLS is intentionally rejected for production despite that
positive lifecycle result. Incus project restriction narrows a client to named
projects, but the resulting authority is still broad within a project. Moving
the bridge and ACL to `default` materially narrows that authority because a
runner-project client cannot mutate those objects. The project-local profile
remains mutable, though: such a client can remove the direct NIC ACL,
filtering, or port isolation, change NIC defaults, or introduce another
profile. A future TLS design must pin the server certificate and prove a
least-privilege authorization boundary that cannot mutate those remaining
isolation controls before this validator can report success for it.

## Assurance boundaries

`validate` proves API configuration shape, not runtime enforcement. It does
not re-measure physical capacity or continuously attest host kernel and VM
behavior. Project CPU and memory totals are admission budgets, not aggregate
runtime throttles for already-running VMs.

Treat Secure Boot as required configuration, not proof that an untrusted EFI
payload is rejected. Likewise, the NIC bandwidth and ZFS disk-I/O values are
requested limits until their effective throughput has been benchmarked on the
production host. Explicit NIC-level IPv6 denial and filtering remain required,
but operators must validate their runtime behavior on the deployed host.

The Go test suite exercises policy validation and the read-only Incus adapter
without replacing command-line executables.

## Incus references

- [Project restrictions and aggregate limits](https://linuxcontainers.org/incus/docs/main/reference/projects/)
- [Bridged NIC filtering, port isolation, ACLs, and bandwidth limits](https://linuxcontainers.org/incus/docs/main/reference/devices_nic/)
- [Bridge network configuration](https://linuxcontainers.org/incus/docs/main/reference/network_bridge/)
- [Network ACL behavior and bridge limitations](https://linuxcontainers.org/incus/docs/main/howto/network_acls/)
- [Disk size and I/O limits](https://linuxcontainers.org/incus/docs/main/reference/devices_disk/)
- [Storage pools and drivers](https://linuxcontainers.org/incus/docs/main/reference/storage_drivers/)

# Incus isolation baseline

This directory contains reviewable desired-state examples, a CUE policy
module, and a read-only drift validator for one single-purpose Incus 7
runner compute host. The baseline supports a dedicated standalone host or
one clustered member, and either a local Unix socket or an exact HTTPS
listener, without changing the host-isolation policy. None of these
artifacts configure or mutate Incus. Render or adapt an
environment-specific baseline, apply it through a trusted Incus administration
path, then validate the effective API state.

The example establishes:

- a dedicated restricted project with explicit aggregate VM, CPU, memory,
  network, and per-pool disk ceilings;
- one host-owned managed bridge and network ACL, one project-local profile, and
  one dedicated ZFS or LVM thin pool;
- per-VM CPU, memory, root-disk, network-bandwidth, and requested disk-I/O
  limits;
- MAC and IPv4 anti-spoofing, explicit NIC-level IPv6 address denial, IPv6
  filtering, and bridge-port isolation;
- a network ACL attached to both the host-owned bridge and the profile NIC,
  with rejected and logged unmatched ingress and egress; and
- egress only to an explicit DNS resolver and HTTP CONNECT proxy, plus at most
  16 optional exact IPv4 TCP or UDP endpoints.

Incus 7.0.1 rejects creation of a managed bridge in a non-default project;
only OVN networks can be created and managed there. This baseline therefore
sets `features.networks=false` and `limits.networks=0` in
the runner project. A host administrator owns the bridge and ACL in the
`default` project, and `restricted.networks.access` allowlists only that bridge
for the runner project.

## Connection authority modes

The baseline accepts two connection authority modes:

- `dedicated-host-unix-socket` requires an empty `core.https_address`.
- `dedicated-host-https` requires `core.https_address` to equal one concrete
  host and port.

Both require `dedicated_single_purpose_host_required=true` and
`unix_socket_is_root_equivalent=true`. The default server profile keeps
`server.standalone=true` and an empty `cluster.https_address`. The cluster
server profile sets `server.standalone=false` and requires
`server.cluster_https_address` to equal the connected member's
`cluster.https_address`. The validator compares those values against the
member it is connected to; it does not validate every cluster member and does
not add placement or failover semantics. It also does not change the guest
contract, runner image requirements, project topology, or VM lifecycle.

For HTTPS, enroll the controller certificate as restricted to the runner
project from the outset. An unrestricted TLS certificate is an Incus
administrator, and HTTPS transport alone does not narrow authority. Project
restriction still depends on the project, host-owned network and ACL,
dedicated storage, and workload-isolation controls described below.

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
host capacity, runner sizing, controlled network endpoints, storage inputs,
an optional exact HTTPS listener, and an optional cluster server profile. It
derives aggregate limits and emits a complete baseline while keeping the
security controls non-overridable. Empty `inputs.server.coreHTTPSAddress`
renders `dedicated-host-unix-socket`; a concrete host and port renders
`dedicated-host-https` and the same exact listener into the baseline. The
default server profile keeps `server.standalone=true` and an empty cluster
listener; `examples/cluster` sets `standalone=false` and a concrete member
`cluster.https_address`. The optional `additionalEgress` list accepts only
named IPv4 `/32` endpoints with one TCP or UDP port each; it cannot express
CIDR ranges, port ranges, actions, or rule state. The module also emits the
controller project, sole profile, and `capacity.max_runners` as one partial
configuration so those values cannot drift from the baseline. Its default ZFS
and LVM examples are checked for semantic equality with
`baseline.example.json` and `baseline.lvm.example.json`, respectively.

Registry publication is not part of this proof increment. Until the `@v0`
module interface is reviewed and published, `baseline.example.json` and
`baseline.lvm.example.json` remain the portable Unix-socket deployment
artifacts. HTTPS deployments must set
`inputs.server.coreHTTPSAddress` in an environment-specific CUE configuration
and render a baseline with `dedicated-host-https` authority. Cluster-member
deployments must also set `inputs.server.standalone` to `false` and
`inputs.server.clusterHTTPSAddress` to that member's `cluster.https_address`.
Copy or render the
appropriate baseline outside the checkout and change every
environment-specific value before applying it:

- replace the `192.0.2.10/32` proxy and `192.0.2.53/32` DNS documentation
  addresses with dedicated endpoint IPv4 `/32` CIDRs;
- replace the bridge subnet, names, storage source, and capacity limits;
  managed bridge names must be 2 to 15 characters, start with a lowercase
  letter, and otherwise contain only lowercase letters, digits, or hyphens;
  use either a dedicated existing zpool or dataset for ZFS, or an existing VG,
  thin-pool name, and default volume size for LVM;
- size aggregate limits below physical compute-host capacity so Incus and the
  host retain explicit CPU, memory, and disk headroom; socket-mode deployments
  must also reserve for the colocated controller. Keep the VM count at or above
  `capacity.max_runners`, and size aggregate CPU, memory, and disk for that many
  profile-limited VMs;
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
Use additional endpoints only for reviewed services that cannot traverse the
proxy, and apply an independent host firewall when an endpoint terminates on the
managed bridge host.
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
IncusOS has no general-purpose host shell. Establish and verify this
kernel-module prerequisite through the appliance administration surface, and
run Incus enrollment and drift-validation commands from an existing trusted
administration workstation.

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

The dedicated-host baseline preserves the Incus 7.0 through 7.2 compatibility
path for VM nesting. Those versions do not advertise
`projects_restricted_virtual_machines_nesting`, so the exact profile setting
`security.nesting=false` is the compensating control. The validator reports
this residual on every successful dedicated-host run.

The validator also rejects a dedicated-host server that advertises that newer
extension, forcing a future dedicated-host baseline update to enforce
`restricted.virtual-machines.nesting=block` rather than silently retaining the
weaker compatibility path. This is a baseline-version gate, not a general
controller server-version limit: the controller can connect to newer Incus
servers even while this dedicated-host isolation baseline rejects them.

The cluster server profile requires `projects_restricted_virtual_machines_nesting`
and sets `restricted.virtual-machines.nesting=block`. Incus still requires
`security.nesting=false` on the runner profile when that project restriction
is `block`. A missing extension or project nesting drift fails cluster
validation.

## Validate without changing Incus

The installed controller binary also provides a standalone validator. Name the
local socket explicitly for Unix-socket validation:

```console
incus-gh-runner validate \
  --socket /var/lib/incus/unix.socket \
  /etc/incus-gh-runner/incus-baseline.json
```

For HTTPS validation, supply all connection inputs as flags:

```console
incus-gh-runner validate \
  --url https://incus.example.com:8443 \
  --client-cert operator.crt \
  --client-key operator.key \
  --server-cert server.crt \
  /etc/incus-gh-runner/incus-baseline.json
```

The command compiles the embedded CUE policy in process, checks the rendered
baseline against that policy, and reads the effective Incus state with GET
operations only. It never creates, changes, or deletes Incus resources and
does not invoke external `cue`, `incus`, or `jq` executables. It does not load
the controller YAML configuration, controller environment variables, or
GitHub credentials. Its connection inputs are flag-only. If neither
`--socket` nor `--url` is present, the CLI retains the standalone validator
default `/var/lib/incus/unix.socket`; an explicit URL requires all three
certificate flags, and an explicit socket and URL together are rejected.

HTTPS validation performs normal TLS verification and requires the server's
presented leaf certificate to equal `--server-cert` for HTTP and WebSocket
connections. Obtain that certificate out of band and compare its fingerprint
with a trusted operator record. Do not use insecure retrieval, blind trust on
first use, or a TLS-verification bypass.

The validator reads sensitive server configuration, the named runner project,
the network and ACL in the `default` project, the runner-project profile, and
the global storage pool. Use a separate operator credential with an
administrative view that exposes all of those resources and values; hidden
listener or global-resource values fail validation. The controller certificate
should remain restricted to the runner project. Do not broaden it to make
validation pass. On IncusOS, which has no general-purpose host shell, run the
HTTPS validator and Incus enrollment commands from an existing trusted
administration workstation.

Validation rejects malformed manifests, missing API extensions, non-`nftables`
firewalls, a server topology that does not match `server.standalone`, a
listener that does not match the selected authority mode or
`cluster.https_address`, and any effective project, network, ACL, profile, or
storage-pool drift. Unix-socket authority requires an empty
`core.https_address`. HTTPS authority requires the exact `core.https_address`
recorded in the baseline. A standalone profile requires an empty
`cluster.https_address`. A cluster profile requires the connected member's
exact `cluster.https_address`.

The validator reads the connected member's server, storage-pool, and
default-project network objects. Storage `source` and `cluster.https_address`
are member-local; render the baseline for the member named by `--url` or
`--socket`.

The storage comparison ignores only the server-generated
`volatile.initial_source` field observed on Incus 7.0.1; `source`, the selected
driver's derived settings, and every other effective storage setting remain
fail-closed.

The rendered baseline records the resource ceilings derived by CUE, but not
the physical host totals and reserved-headroom inputs used to derive them.
Runtime validation can detect drift in those effective ceilings; it cannot
re-measure or re-prove physical-host headroom. Re-render and review the
baseline whenever host capacity or reservations change.

The local socket remains root-equivalent. A restricted HTTPS controller
certificate narrows access to the runner project but does not make the owner
marker authorization or remove the dedicated-host requirement. The
`user.incus-gh-runner.owner` key scopes the controller's intended cleanup; any
writer with project access can forge it.

## Assurance boundaries

`validate` proves API configuration shape, not runtime enforcement. It does
not re-measure physical capacity or continuously attest host kernel and VM
behavior. Project CPU and memory totals are admission budgets, not aggregate
runtime throttles for already-running VMs.

Treat Secure Boot as required configuration, not proof that an untrusted EFI
payload is rejected. Likewise, the NIC bandwidth and storage disk-I/O values are
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
- [Exposing the Incus HTTPS listener](https://linuxcontainers.org/incus/docs/main/howto/server_expose/)
- [TLS client certificates and direct trust enrollment](https://linuxcontainers.org/incus/docs/main/authentication/#tls-client-certificates)

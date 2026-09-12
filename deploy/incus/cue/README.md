# Incus runner CUE module

This dependency-free CUE module models the supported operator inputs and emits
the complete fail-closed Incus baseline consumed by
`incus-gh-runner validate <baseline>`. Its module path is reserved as
`github.com/meigma/incus-gh-runner/config@v0`, but this proof increment is not
yet published to a registry.

The public contract has two definitions:

- `#Inputs` is a closed operator surface. It accepts Incus object names, host
  capacity and reserved headroom, runner sizing, one IPv4 bridge, controlled
  DNS and HTTP CONNECT proxy endpoints, up to 16 optional exact egress
  endpoints, one supported storage-driver input, an optional exact Incus
  HTTPS listener, and an optional cluster server profile.
- `#Deployment` derives the complete `output` baseline and a partial
  `controller` configuration. Aggregate project CPU, memory, disk, and VM
  limits come from the runner count and per-runner sizing; the controller
  fragment pins its project, sole profile, and `capacity.max_runners` to the
  same inputs.

Security-sensitive Incus values are exact constraints, not CUE defaults. An
operator cannot use the module to select the `default` project, relax project
restrictions, disable Secure Boot, add raw Incus configuration, change
default-deny ACL actions, or remove NIC filtering and port isolation.

The default server profile keeps `server.standalone=true` and an empty
`server.cluster_https_address`. Its empty `inputs.server.coreHTTPSAddress`
renders `authority.mode: dedicated-host-unix-socket` and an empty
`server.core_https_address`. A concrete host and port renders
`authority.mode: dedicated-host-https` and requires that exact listener in
`server.core_https_address`. Setting `inputs.server.standalone` to `false`
selects the cluster server profile and requires
`inputs.server.clusterHTTPSAddress` to be the connected member's concrete
`cluster.https_address`. That profile also requires
`projects_restricted_virtual_machines_nesting` and
`restricted.virtual-machines.nesting=block`. It does not add placement or
failover policy. Wildcard and unspecified listeners are rejected. Both
profiles keep `dedicated_single_purpose_host_required=true` and
`unix_socket_is_root_equivalent=true`. The default dedicated-host profile
keeps the Incus 7.0-7.2 nesting residual and does not require the project-level
nesting extension.

The module cannot enable NIC-level IPv6 assignment: the profile fixes
`ipv6.address=none` alongside IPv6 filtering. It does not expose arbitrary ACL
rule bodies. Each `additionalEgress` item renders one enabled allow rule to an
IPv4 `/32` and one TCP or UDP port; CIDR ranges, port ranges, actions, and rule
state are not configurable. Duplicate endpoint tuples and more than 16
additional rules are rejected. The controlled DNS and proxy rules remain
fixed by the hardened deployment contract.
Managed bridge names must contain 2 to 15 characters, start with a lowercase
letter, and otherwise contain only lowercase letters, digits, or hyphens. Incus
uses the name as a Linux network-interface name.

Storage is a closed choice between ZFS and an LVM thin pool. Omitting
`storage.driver` preserves the default ZFS behavior and requires only the
existing `source`; the renderer pins `zfs.pool_name` to that source. The LVM
variant requires `driver: "lvm"`, `source`, `thinPoolName`, and a positive
`volumeSizeGiB` no larger than 16384. It derives `lvm.vg_name` from `source`,
renders the size with the `GiB` suffix, and fixes the driver-specific
description and configuration. Neither variant accepts an arbitrary storage
configuration map or keys from the other driver.

To render an HTTPS baseline, add the following unification to an
environment-specific example and replace the documentation address:

```cue
_deployment: runner.#Deployment & {
	inputs: server: coreHTTPSAddress: "192.0.2.20:8443"
}
```

To render a cluster-member HTTPS baseline, use `examples/cluster` or add the
following unification and replace the documentation addresses:

```cue
_deployment: runner.#Deployment & {
	inputs: server: {
		standalone:          false
		coreHTTPSAddress:    "192.0.2.20:8443"
		clusterHTTPSAddress: "192.0.2.20:8444"
	}
}
```

The controller export deliberately does not choose `incus.socket` or
`incus.url` and does not emit certificate paths. Select one complete
controller connection mode separately. The baseline only couples the selected
authority mode to the effective Incus listener.

## Render the example

From this directory:

```console
mise exec -- cue vet -c ./examples/default
mise exec -- cue export ./examples/default -e baseline --out json
mise exec -- cue export ./examples/default -e controller --out yaml
mise exec -- cue vet -c ./examples/lvm
mise exec -- cue export ./examples/lvm -e baseline --out json
mise exec -- cue export ./examples/lvm -e controller --out yaml
mise exec -- cue vet -c ./examples/additional-egress
mise exec -- cue export ./examples/additional-egress -e baseline --out json
mise exec -- cue export ./examples/additional-egress -e controller --out yaml
mise exec -- cue vet -c ./examples/cluster
mise exec -- cue export ./examples/cluster -e baseline --out json
mise exec -- cue export ./examples/cluster -e controller --out yaml
```

The default and LVM examples use non-routable documentation endpoints and
render the exact contents of `../baseline.example.json` and
`../baseline.lvm.example.json`, respectively.
The additional-egress example demonstrates an optional fourth ACL rule without
providing another portable JSON fixture.
The cluster example demonstrates `server.standalone=false` and a concrete
member cluster listener without providing another portable JSON fixture.
The controller export is intentionally partial: merge it into the full
controller configuration alongside one explicit connection mode and
environment-specific GitHub, image, owner, and minimum-runner settings.

Host capacity and reserved headroom are generation-time inputs. The rendered
baseline contains the resulting Incus ceilings, not those physical-host
measurements. `incus-gh-runner validate` embeds this CUE policy and checks both
the rendered baseline and live Incus state in process, but it cannot re-prove
physical-host headroom at runtime. Re-render after host capacity or reservation
changes. The derived project CPU and memory ceilings are admission budgets
based on the declared per-runner limits; they are not aggregate runtime
throttles for already-running VMs.

## Deferred publication boundary

Registry publication is deliberately deferred until this input contract has
been reviewed and exercised. A later release increment should inspect a
`cue mod publish --out` OCI layout without pushing it, publish an exact `@v0`
semantic version from an approval-gated clean tag, and retain the module with
the release artifacts. Production validation must not depend on registry
availability: the host continues to consume a rendered baseline and perform
read-only Incus API drift checks with the embedded policy.

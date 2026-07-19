# Incus runner CUE module

This dependency-free CUE module models the supported operator inputs and emits
the complete fail-closed Incus baseline consumed by
`incus-gh-runner validate <baseline>`. Its module path is reserved as
`github.com/meigma/incus-gh-runner/config@v0`, but this proof increment is not
yet published to a registry.

The public contract has two definitions:

- `#Inputs` is a closed operator surface. It accepts Incus object names, host
  capacity and reserved headroom, runner sizing, one IPv4 bridge, controlled
  DNS and HTTP CONNECT proxy endpoints, and the ZFS source.
- `#Deployment` derives the complete `output` baseline and a partial
  `controller` configuration. Aggregate project CPU, memory, disk, and VM limits
  come from the runner count and per-runner sizing; the controller fragment
  pins its project, sole profile, and `capacity.max_runners` to the same inputs.

Security-sensitive Incus values are exact constraints, not CUE defaults. An
operator cannot use the module to enable the Incus HTTPS listener, use the
`default` project, relax project restrictions, disable Secure Boot, add raw
Incus configuration, change default-deny ACL actions, or remove NIC filtering
and port isolation. It also cannot enable NIC-level IPv6 assignment: the
profile fixes `ipv6.address=none` alongside IPv6 filtering. The module
intentionally does not expose arbitrary direct egress rules; v0 keeps the
controlled DNS and proxy boundary fixed by the hardened deployment contract.
Managed bridge names must contain 2 to 15 characters, start with a lowercase
letter, and otherwise contain only lowercase letters, digits, or hyphens. Incus
uses the name as a Linux network-interface name.

## Render the example

From this directory:

```console
mise exec -- cue vet -c ./examples/default
mise exec -- cue export ./examples/default -e baseline --out json
mise exec -- cue export ./examples/default -e controller --out yaml
```

The example uses non-routable documentation endpoints and renders the exact
contents of `../baseline.example.json`. The contract test compares the two as
normalized JSON, so the CUE policy and validator fixture cannot drift silently.
The controller export is intentionally partial: merge it into the full
controller configuration alongside environment-specific GitHub, image, owner,
and minimum-runner settings.

Host capacity and reserved headroom are generation-time inputs. The rendered
baseline contains the resulting Incus ceilings, not those physical-host
measurements. `incus-gh-runner validate` embeds this CUE policy and checks both
the rendered baseline and live Incus state in process, but it cannot re-prove
physical-host headroom at runtime. Re-render after host capacity or reservation
changes. The derived project CPU and memory ceilings are admission budgets
based on the declared per-runner limits; they are not aggregate runtime
throttles for already-running VMs.

## Validate the module

From the repository root:

```console
mise exec -- bash deploy/incus/cue/tests/render-test.sh
```

The test runs formatting, module tidiness, concrete vetting, the golden export,
a non-default sizing/port example, and negative weakening cases. The CUE binary
is checksum-pinned for all supported development platforms through `mise`.

## Deferred publication boundary

Registry publication is deliberately deferred until this input contract has
been reviewed and exercised. A later release increment should inspect a
`cue mod publish --out` OCI layout without pushing it, publish an exact `@v0`
semantic version from an approval-gated clean tag, and retain the module with
the release artifacts. Production validation must not depend on registry
availability: the host continues to consume a rendered baseline and perform
read-only Incus API drift checks with the embedded policy.

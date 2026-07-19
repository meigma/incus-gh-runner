#!/usr/bin/env bash
set -Eeuo pipefail

manifest="${1:?baseline manifest is required}"
project="$(jq -er '.names.project' "$manifest")"
network="$(jq -er '.names.network' "$manifest")"
network_acl="$(jq -er '.names.network_acl' "$manifest")"
profile="$(jq -er '.names.profile' "$manifest")"
storage_pool="$(jq -er '.names.storage_pool' "$manifest")"

[[ "$project" == slice2-* && "$network" == slice2-* && \
  "$network_acl" == slice2-* && "$profile" == slice2-* && \
  "$storage_pool" == slice2-* ]] || {
  printf 'refusing non-slice2 live target\n' >&2
  exit 2
}

if incus project show "$project" >/dev/null 2>&1 || \
  incus network show "$network" --project default >/dev/null 2>&1 || \
  incus network acl show "$network_acl" --project default >/dev/null 2>&1; then
  printf 'one or more slice2 target resources already exist\n' >&2
  exit 2
fi
incus storage show "$storage_pool" >/dev/null

storage_put="$(jq -c '{description: .storage_pool.description, config: .storage_pool.config}' "$manifest")"
incus query -X PUT "/1.0/storage-pools/${storage_pool}" -d "$storage_put" >/dev/null

network_create="$(
  jq -c '{
    name: .names.network,
    description: .network.description,
    type: .network.type,
    config: (.network.config | with_entries(select(.key | startswith("security.acls") | not)))
  }' "$manifest"
)"
incus query -X POST '/1.0/networks?project=default' -d "$network_create" >/dev/null

acl_create="$(
  jq -c '{
    name: .names.network_acl,
    description: .network_acl.description,
    config: .network_acl.config,
    ingress: .network_acl.ingress,
    egress: .network_acl.egress
  }' "$manifest"
)"
incus query -X POST '/1.0/network-acls?project=default' -d "$acl_create" >/dev/null

network_put="$(jq -c '{description: .network.description, config: .network.config}' "$manifest")"
incus query -X PUT "/1.0/networks/${network}?project=default" -d "$network_put" >/dev/null

project_create="$(
  jq -c '{
    name: .names.project,
    description: .project.description,
    config: (.project.config | with_entries(select(.key | startswith("features."))))
  }' "$manifest"
)"
incus query -X POST /1.0/projects -d "$project_create" >/dev/null

profile_create="$(
  jq -c '{
    name: .names.profile,
    description: .profile.description,
    config: .profile.config,
    devices: .profile.devices
  }' "$manifest"
)"
incus query -X POST "/1.0/profiles?project=${project}" -d "$profile_create" >/dev/null

project_put="$(jq -c '{description: .project.description, config: .project.config}' "$manifest")"
incus query -X PUT "/1.0/projects/${project}" -d "$project_put" >/dev/null

printf 'applied live baseline for %s\n' "$project"

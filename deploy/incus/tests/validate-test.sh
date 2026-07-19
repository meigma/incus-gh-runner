#!/usr/bin/env bash
set -Eeuo pipefail

test_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
incus_dir="$(cd -- "${test_dir}/.." && pwd)"
validator="${incus_dir}/validate.sh"
baseline="${incus_dir}/baseline.example.json"

for command_name in bash jq mktemp; do
  command -v "$command_name" >/dev/null || {
    printf 'required command is unavailable: %s\n' "$command_name" >&2
    exit 1
  }
done

temp_root="$(mktemp -d)"
trap 'rm -rf -- "$temp_root"' EXIT

fake_incus="${temp_root}/incus"
calls="${temp_root}/calls"

printf '%s\n' '#!/usr/bin/env bash' >"$fake_incus"
printf '%s\n' 'set -Eeuo pipefail' >>"$fake_incus"
printf '%s\n' 'printf "%s\n" "$*" >>"$FAKE_CALLS"' >>"$fake_incus"
printf '%s\n' '[[ "$#" -eq 4 && "$1" == query && "$2" == -X && "$3" == GET ]] || exit 91' >>"$fake_incus"
printf '%s\n' 'path="$4"' >>"$fake_incus"
printf '%s\n' 'scenario="${FAKE_SCENARIO:-valid}"' >>"$fake_incus"
printf '%s\n' '[[ "$scenario" != query-failure ]] || exit 92' >>"$fake_incus"
printf '%s\n' 'case "$path" in' >>"$fake_incus"
printf '%s\n' '  /1.0)' >>"$fake_incus"
printf '%s\n' '    jq --arg scenario "$scenario" '\''{' >>"$fake_incus"
printf '%s\n' '      auth: "trusted",' >>"$fake_incus"
printf '%s\n' '      api_extensions: (' >>"$fake_incus"
printf '%s\n' '        if $scenario == "missing-extension" then .server.required_api_extensions[1:]' >>"$fake_incus"
printf '%s\n' '        elif $scenario == "nesting-project-extension-available" then (.server.required_api_extensions + [.residual_controls.project_vm_nesting_restriction.future_api_extension])' >>"$fake_incus"
printf '%s\n' '        else .server.required_api_extensions' >>"$fake_incus"
printf '%s\n' '        end' >>"$fake_incus"
printf '%s\n' '      ),' >>"$fake_incus"
printf '%s\n' '      config: {' >>"$fake_incus"
printf '%s\n' '        "core.https_address": (if $scenario == "exposed-api" then "0.0.0.0:8443" else .server.core_https_address end),' >>"$fake_incus"
printf '%s\n' '        "cluster.https_address": .server.cluster_https_address' >>"$fake_incus"
printf '%s\n' '      },' >>"$fake_incus"
printf '%s\n' '      environment: {' >>"$fake_incus"
printf '%s\n' '        server_version: .server.minimum_version,' >>"$fake_incus"
printf '%s\n' '        server_clustered: false,' >>"$fake_incus"
printf '%s\n' '        firewall: .server.firewall_driver' >>"$fake_incus"
printf '%s\n' '      }' >>"$fake_incus"
printf '%s\n' '    }'\'' "$FAKE_BASELINE"' >>"$fake_incus"
printf '%s\n' '    ;;' >>"$fake_incus"
printf '%s\n' '  "/1.0/projects/$(jq -r .names.project "$FAKE_BASELINE")")' >>"$fake_incus"
printf '%s\n' '    jq --arg scenario "$scenario" '\''{' >>"$fake_incus"
printf '%s\n' '      name: .names.project,' >>"$fake_incus"
printf '%s\n' '      description: .project.description,' >>"$fake_incus"
printf '%s\n' '      config: (if $scenario == "project-drift" then (.project.config + {restricted: "false"}) else .project.config end)' >>"$fake_incus"
printf '%s\n' '    }'\'' "$FAKE_BASELINE"' >>"$fake_incus"
printf '%s\n' '    ;;' >>"$fake_incus"
printf '%s\n' '  "/1.0/networks/$(jq -r .names.network "$FAKE_BASELINE")?project=default")' >>"$fake_incus"
printf '%s\n' '    jq '\''{name: .names.network, description: .network.description, type: .network.type, managed: .network.managed, config: .network.config}'\'' "$FAKE_BASELINE"' >>"$fake_incus"
printf '%s\n' '    ;;' >>"$fake_incus"
printf '%s\n' '  "/1.0/network-acls/$(jq -r .names.network_acl "$FAKE_BASELINE")?project=default")' >>"$fake_incus"
printf '%s\n' '    jq '\''{name: .names.network_acl, description: .network_acl.description, config: .network_acl.config, ingress: .network_acl.ingress, egress: .network_acl.egress}'\'' "$FAKE_BASELINE"' >>"$fake_incus"
printf '%s\n' '    ;;' >>"$fake_incus"
printf '%s\n' '  "/1.0/profiles/$(jq -r .names.profile "$FAKE_BASELINE")?project=$(jq -r .names.project "$FAKE_BASELINE")")' >>"$fake_incus"
printf '%s\n' '    jq --arg scenario "$scenario" '\''{' >>"$fake_incus"
printf '%s\n' '      name: .names.profile,' >>"$fake_incus"
printf '%s\n' '      description: .profile.description,' >>"$fake_incus"
printf '%s\n' '      config: .profile.config,' >>"$fake_incus"
printf '%s\n' '      devices: (if $scenario == "profile-extra-device" then (.profile.devices + {gpu: {type: "gpu"}}) else .profile.devices end)' >>"$fake_incus"
printf '%s\n' '    }'\'' "$FAKE_BASELINE"' >>"$fake_incus"
printf '%s\n' '    ;;' >>"$fake_incus"
printf '%s\n' '  "/1.0/storage-pools/$(jq -r .names.storage_pool "$FAKE_BASELINE")")' >>"$fake_incus"
printf '%s\n' '    jq --arg scenario "$scenario" '\''{' >>"$fake_incus"
printf '%s\n' '      name: .names.storage_pool,' >>"$fake_incus"
printf '%s\n' '      description: .storage_pool.description,' >>"$fake_incus"
printf '%s\n' '      driver: .storage_pool.driver,' >>"$fake_incus"
printf '%s\n' '      config: (' >>"$fake_incus"
printf '%s\n' '        (.storage_pool.config + {"volatile.initial_source": .storage_pool.config.source}) |' >>"$fake_incus"
printf '%s\n' '        if $scenario == "storage-source-drift" then (.source = "unexpected-zpool")' >>"$fake_incus"
printf '%s\n' '        elif $scenario == "storage-extra-generated-key" then (."volatile.unexpected" = "value")' >>"$fake_incus"
printf '%s\n' '        else .' >>"$fake_incus"
printf '%s\n' '        end' >>"$fake_incus"
printf '%s\n' '      )' >>"$fake_incus"
printf '%s\n' '    }'\'' "$FAKE_BASELINE"' >>"$fake_incus"
printf '%s\n' '    ;;' >>"$fake_incus"
printf '%s\n' '  *) exit 93 ;;' >>"$fake_incus"
printf '%s\n' 'esac' >>"$fake_incus"
chmod 0755 "$fake_incus"

run_validator() {
  local scenario="$1"
  local manifest="$2"
  local output="$3"

  : >"$calls"
  FAKE_SCENARIO="$scenario" \
    FAKE_BASELINE="$manifest" \
    FAKE_CALLS="$calls" \
    INCUS_GH_RUNNER_INCUS_BIN="$fake_incus" \
    "$validator" "$manifest" >"$output" 2>&1
}

expect_failure() {
  local scenario="$1"
  local manifest="$2"
  local pattern="$3"
  local output="${temp_root}/${scenario}.output"

  if run_validator "$scenario" "$manifest" "$output"; then
    printf 'expected validation failure for %s\n' "$scenario" >&2
    exit 1
  fi
  grep -Fq "$pattern" "$output" || {
    printf 'missing failure text for %s: %s\n' "$scenario" "$pattern" >&2
    cat "$output" >&2
    exit 1
  }
}

jq -e . "$baseline" >/dev/null
valid_output="${temp_root}/valid.output"
run_validator valid "$baseline" "$valid_output"
grep -Fq 'Incus isolation baseline matches' "$valid_output"
grep -Fq 'exact profile security.nesting=false is the compensating control' "$valid_output"
[[ "$(wc -l <"$calls" | tr -d ' ')" == 6 ]]
if grep -Ev '^query -X GET /1[.]0' "$calls" >/dev/null; then
  printf 'validator attempted a non-read-only Incus command\n' >&2
  cat "$calls" >&2
  exit 1
fi
grep -Fxq "query -X GET /1.0/networks/$(jq -r .names.network "$baseline")?project=default" "$calls"
grep -Fxq "query -X GET /1.0/network-acls/$(jq -r .names.network_acl "$baseline")?project=default" "$calls"
grep -Fxq "query -X GET /1.0/profiles/$(jq -r .names.profile "$baseline")?project=$(jq -r .names.project "$baseline")" "$calls"

custom_proxy_port="${temp_root}/custom-proxy-port.json"
jq '.network_acl.egress[2].destination_port = "8080"' "$baseline" >"$custom_proxy_port"
run_validator valid "$custom_proxy_port" "${temp_root}/custom-proxy-port.output"

expect_failure missing-extension "$baseline" 'required Incus API extension is unavailable'
expect_failure nesting-project-extension-available "$baseline" 'baseline must be upgraded to enforce the project-level restriction'
expect_failure exposed-api "$baseline" 'core.https_address drift detected'
expect_failure project-drift "$baseline" 'project drift detected'
expect_failure profile-extra-device "$baseline" 'profile drift detected'
expect_failure query-failure "$baseline" 'read-only query failed for server'
expect_failure storage-source-drift "$baseline" 'storage pool drift detected'
expect_failure storage-extra-generated-key "$baseline" 'storage pool drift detected'

loopback_tls="${temp_root}/loopback-tls.json"
jq '.authority.mode = "project-restricted-loopback-tls"' "$baseline" >"$loopback_tls"
expect_failure valid "$loopback_tls" 'required fail-closed invariant is invalid'

incoherent_authority="${temp_root}/incoherent-authority.json"
jq '.server.core_https_address = "127.0.0.1:8443"' "$baseline" >"$incoherent_authority"
expect_failure valid "$incoherent_authority" 'required fail-closed invariant is invalid'

indirect_acl="${temp_root}/indirect-acl.json"
jq 'del(.profile.devices.eth0["security.acls"]) | .network.config["security.acls"] = .names.network_acl' "$baseline" >"$indirect_acl"
expect_failure valid "$indirect_acl" 'required fail-closed invariant is invalid'

project_local_network="${temp_root}/project-local-network.json"
jq '.project.config["features.networks"] = "true" | .project.config["limits.networks"] = "1"' "$baseline" >"$project_local_network"
expect_failure valid "$project_local_network" 'required fail-closed invariant is invalid'

missing_network_acl="${temp_root}/missing-network-acl.json"
jq 'del(.network.config["security.acls"])' "$baseline" >"$missing_network_acl"
expect_failure valid "$missing_network_acl" 'required fail-closed invariant is invalid'

permissive_network_default="${temp_root}/permissive-network-default.json"
jq '.network.config["security.acls.default.egress.action"] = "allow"' "$baseline" >"$permissive_network_default"
expect_failure valid "$permissive_network_default" 'required fail-closed invariant is invalid'

unlogged_nic_default="${temp_root}/unlogged-nic-default.json"
jq '.profile.devices.eth0["security.acls.default.egress.logged"] = "false"' "$baseline" >"$unlogged_nic_default"
expect_failure valid "$unlogged_nic_default" 'required fail-closed invariant is invalid'

weakened_known_restriction="${temp_root}/weakened-known-restriction.json"
jq '.project.config["restricted.devices.unix-char"] = "allow"' "$baseline" >"$weakened_known_restriction"
expect_failure valid "$weakened_known_restriction" 'required fail-closed invariant is invalid'

missing_required_manifest_extension="${temp_root}/missing-required-manifest-extension.json"
jq '.server.required_api_extensions -= ["network_bridge_acl_devices"]' "$baseline" >"$missing_required_manifest_extension"
expect_failure valid "$missing_required_manifest_extension" 'required fail-closed invariant is invalid'

unexpected_manifest_key="${temp_root}/unexpected-key.json"
jq '.project.config["restricted.devices.nic_typo"] = "managed"' "$baseline" >"$unexpected_manifest_key"
expect_failure valid "$unexpected_manifest_key" 'required fail-closed invariant is invalid'

missing_storage_source="${temp_root}/missing-storage-source.json"
jq 'del(.storage_pool.config.source)' "$baseline" >"$missing_storage_source"
expect_failure valid "$missing_storage_source" 'required fail-closed invariant is invalid'

wide_acl_destination="${temp_root}/wide-acl-destination.json"
jq '.network_acl.egress[0].destination = "0.0.0.0/0"' "$baseline" >"$wide_acl_destination"
expect_failure valid "$wide_acl_destination" 'required fail-closed invariant is invalid'

malformed_acl_destination="${temp_root}/malformed-acl-destination.json"
jq '.network_acl.egress[0].destination = "999.0.0.1/32"' "$baseline" >"$malformed_acl_destination"
expect_failure valid "$malformed_acl_destination" 'required fail-closed invariant is invalid'

proxy_on_dns_port="${temp_root}/proxy-on-dns-port.json"
jq '.network_acl.egress[2].destination_port = "53"' "$baseline" >"$proxy_on_dns_port"
expect_failure valid "$proxy_on_dns_port" 'required fail-closed invariant is invalid'

invalid_proxy_port="${temp_root}/invalid-proxy-port.json"
jq '.network_acl.egress[2].destination_port = "65536"' "$baseline" >"$invalid_proxy_port"
expect_failure valid "$invalid_proxy_port" 'required fail-closed invariant is invalid'

printf 'Incus isolation validator tests passed\n'

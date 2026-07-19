#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf 'usage: %s [baseline.json]\n' "$0" >&2
}

fail() {
  printf 'Incus isolation validation failed: %s\n' "$*" >&2
  exit 1
}

if [[ "$#" -gt 1 ]]; then
  usage
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
manifest="${1:-${script_dir}/baseline.example.json}"
incus_bin="${INCUS_GH_RUNNER_INCUS_BIN:-incus}"

command -v jq >/dev/null || fail 'required command is unavailable: jq'
command -v "$incus_bin" >/dev/null || fail "required command is unavailable: ${incus_bin}"
[[ -f "$manifest" ]] || fail "baseline does not exist: ${manifest}"

manifest_filter='
  def keys_are($wanted): keys == ($wanted | sort);
  def string_map: type == "object" and all(.[]; type == "string");
  def safe_name: type == "string" and test("^[a-z][a-z0-9-]{0,62}$");
  def positive_size: type == "string" and test("^[1-9][0-9]*(KiB|MiB|GiB|TiB|kB|MB|GB|TB|Kbit|Mbit|Gbit)$");
  def decimal_between($minimum; $maximum):
    . as $value |
    ($value | type == "string" and test("^(0|[1-9][0-9]*)$")) and
    (($value | tonumber) >= $minimum and ($value | tonumber) <= $maximum);
  def ipv4_cidr:
    type == "string" and
    (split("/") as $parts |
      ($parts | length) == 2 and
      ($parts[1] | decimal_between(0; 32)) and
      (($parts[0] | split(".")) as $octets |
        ($octets | length) == 4 and
        all($octets[]; decimal_between(0; 255))));
  def ipv4_host_cidr: ipv4_cidr and endswith("/32");
  def rule:
    type == "object" and
    keys_are(["action", "state", "description", "destination", "protocol", "destination_port"]) and
    .action == "allow" and .state == "enabled" and
    (.description | type == "string" and length > 0) and
    (.destination | ipv4_host_cidr) and
    (.protocol == "tcp" or .protocol == "udp") and
    (.destination_port | decimal_between(1; 65535));

  . as $baseline |
  type == "object" and
  keys_are(["schema_version", "authority", "names", "server", "residual_controls", "project", "network", "network_acl", "profile", "storage_pool"]) and
  .schema_version == 1 and

  (.authority |
    type == "object" and
    keys_are(["mode", "dedicated_single_purpose_host_required", "unix_socket_is_root_equivalent"]) and
    .mode == "dedicated-host-unix-socket" and
    .dedicated_single_purpose_host_required == true and
    .unix_socket_is_root_equivalent == true) and

  (.names |
    type == "object" and
    keys_are(["project", "network", "network_acl", "profile", "storage_pool"]) and
    all(.[]; safe_name) and .project != "default") and

  (.server |
    type == "object" and
    keys_are(["minimum_version", "required_api_extensions", "firewall_driver", "standalone", "core_https_address", "cluster_https_address"]) and
    (.minimum_version | type == "string" and test("^[0-9]+[.][0-9]+([.][0-9]+)?$")) and
    (.required_api_extensions | type == "array" and length > 0 and all(.[]; type == "string" and length > 0) and unique == .) and
    .firewall_driver == "nftables" and .standalone == true and
    (.core_https_address | type == "string") and .cluster_https_address == "") and

  (.residual_controls |
    type == "object" and keys_are(["project_vm_nesting_restriction"]) and
    (.project_vm_nesting_restriction |
      type == "object" and
      keys_are(["status", "future_api_extension", "compensating_profile_key", "compensating_profile_value"]) and
      .status == "unsupported-by-incus-7.0-through-7.2" and
      .future_api_extension == "projects_restricted_virtual_machines_nesting" and
      .compensating_profile_key == "security.nesting" and
      .compensating_profile_value == "false")) and

  (.project |
    type == "object" and keys_are(["description", "config"]) and
    (.description | type == "string" and length > 0) and
    (.config |
      string_map and
      keys_are([
        "features.images", "features.networks", "features.profiles", "features.storage.buckets", "features.storage.volumes",
        "images.auto_update_cached", "images.auto_update_interval",
        "limits.containers", "limits.cpu", "limits.disk", "limits.instances", "limits.memory", "limits.networks", "limits.virtual-machines",
        "restricted", "restricted.backups", "restricted.cluster.target",
        "restricted.containers.interception", "restricted.containers.lowlevel", "restricted.containers.nesting", "restricted.containers.privilege",
        "restricted.devices.disk", "restricted.devices.gpu", "restricted.devices.infiniband", "restricted.devices.nic", "restricted.devices.pci", "restricted.devices.proxy",
        "restricted.devices.unix-block", "restricted.devices.unix-char", "restricted.devices.unix-hotplug", "restricted.devices.usb",
        "restricted.networks.access", "restricted.snapshots", "restricted.storage-pools.access", "restricted.virtual-machines.lowlevel"
      ] + [("limits.disk.pool." + $baseline.names.storage_pool)]))) and
  (.network |
    type == "object" and keys_are(["description", "type", "managed", "config"]) and
    (.description | type == "string" and length > 0) and
    .type == "bridge" and .managed == true and
    (.config |
      string_map and
      keys_are([
        "bridge.driver", "dns.mode", "dns.nameservers",
        "ipv4.address", "ipv4.dhcp", "ipv4.firewall", "ipv4.nat", "ipv4.routing", "ipv6.address",
        "raw.dnsmasq", "security.acls",
        "security.acls.default.egress.action", "security.acls.default.egress.logged",
        "security.acls.default.ingress.action", "security.acls.default.ingress.logged"
      ]))) and
  (.network_acl |
    type == "object" and keys_are(["description", "config", "ingress", "egress"]) and
    (.description | type == "string" and length > 0) and
    .config == {} and .ingress == [] and
    (.egress |
      type == "array" and length == 3 and all(.[]; rule) and
      ([.[] | select(.destination_port == "53")] as $dns |
        ($dns | length) == 2 and
        ([$dns[] | .protocol] | sort) == ["tcp", "udp"] and
        ([$dns[] | .destination] | unique | length) == 1) and
      ([.[] | select(.destination_port != "53")] as $proxy |
        ($proxy | length) == 1 and $proxy[0].protocol == "tcp"))) and
  (.profile |
    type == "object" and keys_are(["description", "config", "devices"]) and
    (.description | type == "string" and length > 0) and
    (.config |
      string_map and
      keys_are(["boot.autostart", "limits.cpu", "limits.memory", "security.guestapi", "security.nesting", "security.secureboot"])) and
    (.devices |
      type == "object" and keys_are(["eth0", "root"]) and all(.[]; string_map) and
      (.eth0 |
        keys_are([
          "type", "network", "limits.max", "security.acls",
          "security.acls.default.egress.action", "security.acls.default.egress.logged",
          "security.acls.default.ingress.action", "security.acls.default.ingress.logged",
          "security.ipv4_filtering", "security.ipv6_filtering", "security.mac_filtering", "security.port_isolation"
        ])) and
      (.root | keys_are(["type", "path", "pool", "size", "limits.max"])))) and
  (.storage_pool |
    type == "object" and keys_are(["description", "driver", "config"]) and
    (.description | type == "string" and length > 0) and
    .driver == "zfs" and
    (.config |
      string_map and keys_are(["source", "zfs.pool_name"]) and
      (.source | length > 0) and (."zfs.pool_name" | length > 0))) and

  .server.core_https_address == "" and
  .server.minimum_version == "7.0" and
  .server.required_api_extensions == [
    "container_nic_ipfilter",
    "instance_nic_bridged_port_isolation",
    "network_acl",
    "network_bridge_acl",
    "network_bridge_acl_devices",
    "projects_limits_disk_pool",
    "projects_networks",
    "projects_networks_restricted_access",
    "projects_restricted_storage_pool_access",
    "projects_restrictions"
  ] and

  .project.config["features.images"] == "true" and
  .project.config["features.networks"] == "false" and
  .project.config["features.profiles"] == "true" and
  .project.config["features.storage.buckets"] == "true" and
  .project.config["features.storage.volumes"] == "true" and
  .project.config["images.auto_update_cached"] == "false" and
  .project.config["images.auto_update_interval"] == "0" and
  .project.config["limits.containers"] == "0" and
  (.project.config["limits.cpu"] | test("^[1-9][0-9]*$")) and
  (.project.config["limits.memory"] | positive_size) and
  (.project.config["limits.disk"] | positive_size) and
  (.project.config["limits.instances"] | test("^[1-9][0-9]*$")) and
  .project.config["limits.instances"] == .project.config["limits.virtual-machines"] and
  .project.config["limits.networks"] == "0" and
  .project.config.restricted == "true" and
  .project.config["restricted.backups"] == "block" and
  .project.config["restricted.cluster.target"] == "block" and
  .project.config["restricted.containers.interception"] == "block" and
  .project.config["restricted.containers.lowlevel"] == "block" and
  .project.config["restricted.containers.nesting"] == "block" and
  .project.config["restricted.containers.privilege"] == "unprivileged" and
  .project.config["restricted.devices.disk"] == "block" and
  .project.config["restricted.devices.gpu"] == "block" and
  .project.config["restricted.devices.infiniband"] == "block" and
  .project.config["restricted.devices.nic"] == "managed" and
  .project.config["restricted.devices.pci"] == "block" and
  .project.config["restricted.devices.proxy"] == "block" and
  .project.config["restricted.devices.unix-block"] == "block" and
  .project.config["restricted.devices.unix-char"] == "block" and
  .project.config["restricted.devices.unix-hotplug"] == "block" and
  .project.config["restricted.devices.usb"] == "block" and
  .project.config["restricted.snapshots"] == "block" and
  .project.config["restricted.virtual-machines.lowlevel"] == "block" and
  .project.config["restricted.networks.access"] == .names.network and
  .project.config["restricted.storage-pools.access"] == .names.storage_pool and
  .project.config[("limits.disk.pool." + .names.storage_pool)] == .project.config["limits.disk"] and

  .network.config["bridge.driver"] == "native" and
  .network.config["dns.mode"] == "none" and
  .network.config["ipv4.dhcp"] == "true" and
  .network.config["ipv4.firewall"] == "true" and
  .network.config["ipv4.nat"] == "true" and
  .network.config["ipv4.routing"] == "true" and
  (.network.config["ipv4.address"] | ipv4_cidr) and
  .network.config["ipv6.address"] == "none" and
  .network.config["raw.dnsmasq"] == "port=0" and
  .network.config["security.acls"] == .names.network_acl and
  .network.config["security.acls.default.egress.action"] == "reject" and
  .network.config["security.acls.default.egress.logged"] == "true" and
  .network.config["security.acls.default.ingress.action"] == "reject" and
  .network.config["security.acls.default.ingress.logged"] == "true" and
  ([.network_acl.egress[] | select(.destination_port == "53") | .destination] | unique | .[0] | rtrimstr("/32")) == .network.config["dns.nameservers"] and

  (.profile.config["limits.cpu"] | test("^[1-9][0-9]*$")) and
  (.profile.config["limits.memory"] | positive_size) and
  .profile.config["boot.autostart"] == "false" and
  .profile.config[.residual_controls.project_vm_nesting_restriction.compensating_profile_key] == .residual_controls.project_vm_nesting_restriction.compensating_profile_value and
  .profile.config["security.guestapi"] == "false" and
  .profile.config["security.nesting"] == "false" and
  .profile.config["security.secureboot"] == "true" and
  .profile.devices.eth0.type == "nic" and
  .profile.devices.eth0.network == .names.network and
  .profile.devices.eth0["security.acls"] == .names.network_acl and
  .profile.devices.eth0["security.acls.default.egress.action"] == "reject" and
  .profile.devices.eth0["security.acls.default.egress.logged"] == "true" and
  .profile.devices.eth0["security.acls.default.ingress.action"] == "reject" and
  .profile.devices.eth0["security.acls.default.ingress.logged"] == "true" and
  .profile.devices.eth0["security.ipv4_filtering"] == "true" and
  .profile.devices.eth0["security.ipv6_filtering"] == "true" and
  .profile.devices.eth0["security.mac_filtering"] == "true" and
  .profile.devices.eth0["security.port_isolation"] == "true" and
  (.profile.devices.eth0["limits.max"] | positive_size) and
  .profile.devices.root.type == "disk" and
  .profile.devices.root.path == "/" and
  .profile.devices.root.pool == .names.storage_pool and
  (.profile.devices.root.size | positive_size) and
  (.profile.devices.root["limits.max"] | positive_size)
'

if ! jq -e "$manifest_filter" "$manifest" >/dev/null; then
  fail 'baseline schema or a required fail-closed invariant is invalid'
fi

version_at_least() {
  local have="${1%%-*}"
  local need="${2%%-*}"
  local have_major have_minor have_patch need_major need_minor need_patch

  IFS=. read -r have_major have_minor have_patch <<<"$have"
  IFS=. read -r need_major need_minor need_patch <<<"$need"
  have_patch="${have_patch:-0}"
  need_patch="${need_patch:-0}"

  [[ "$have_major" =~ ^[0-9]+$ && "$have_minor" =~ ^[0-9]+$ && "$have_patch" =~ ^[0-9]+$ ]] || return 1
  [[ "$need_major" =~ ^[0-9]+$ && "$need_minor" =~ ^[0-9]+$ && "$need_patch" =~ ^[0-9]+$ ]] || return 1

  (( have_major > need_major )) ||
    (( have_major == need_major && have_minor > need_minor )) ||
    (( have_major == need_major && have_minor == need_minor && have_patch >= need_patch ))
}

normalize_response() {
  jq -ce '
    if type == "object" and has("metadata") and (has("status_code") or has("type"))
    then .metadata
    else .
    end
  '
}

query_object() {
  local path="$1"
  local label="$2"
  local raw

  if ! raw="$("$incus_bin" query -X GET "$path")"; then
    fail "read-only query failed for ${label}"
  fi

  if ! normalize_response <<<"$raw"; then
    fail "read-only query returned invalid JSON for ${label}"
  fi
}

compare_object() {
  local label="$1"
  local expected_filter="$2"
  local actual_filter="$3"
  local actual_json="$4"
  local expected actual

  expected="$(jq -cS "$expected_filter" "$manifest")"
  if ! actual="$(jq -ceS "$actual_filter" <<<"$actual_json")"; then
    fail "${label} response is missing required fields"
  fi

  if [[ "$expected" != "$actual" ]]; then
    printf 'expected %s: %s\n' "$label" "$expected" >&2
    printf 'actual %s:   %s\n' "$label" "$actual" >&2
    fail "${label} drift detected"
  fi
}

core_https_address="$(jq -er '.server.core_https_address' "$manifest")"
[[ -z "$core_https_address" ]] ||
  fail 'server.core_https_address must be empty; network API authority is not accepted by this baseline'

project="$(jq -er '.names.project' "$manifest")"
network="$(jq -er '.names.network' "$manifest")"
network_acl="$(jq -er '.names.network_acl' "$manifest")"
profile="$(jq -er '.names.profile' "$manifest")"
storage_pool="$(jq -er '.names.storage_pool' "$manifest")"

server_json="$(query_object '/1.0' 'server')"
server_version="$(jq -er '.environment.server_version' <<<"$server_json")" ||
  fail 'server response does not contain environment.server_version'
minimum_version="$(jq -er '.server.minimum_version' "$manifest")"
version_at_least "$server_version" "$minimum_version" ||
  fail "Incus ${minimum_version} or newer is required; found ${server_version}"

[[ "$(jq -er '.auth' <<<"$server_json")" == trusted ]] || fail 'validator requires a trusted read-only API view'
[[ "$(jq -er '.environment.server_clustered' <<<"$server_json")" == false ]] || fail 'clustered Incus is outside this dedicated-host baseline'
[[ "$(jq -er '.environment.firewall' <<<"$server_json")" == "$(jq -er '.server.firewall_driver' "$manifest")" ]] ||
  fail 'server firewall driver drift detected'
[[ "$(jq -r '.config["core.https_address"] // ""' <<<"$server_json")" == "$core_https_address" ]] ||
  fail 'core.https_address drift detected'
[[ "$(jq -r '.config["cluster.https_address"] // ""' <<<"$server_json")" == "$(jq -er '.server.cluster_https_address' "$manifest")" ]] ||
  fail 'cluster.https_address must remain empty'

while IFS= read -r extension; do
  jq -e --arg extension "$extension" '.api_extensions | index($extension) != null' <<<"$server_json" >/dev/null ||
    fail "required Incus API extension is unavailable: ${extension}"
done < <(jq -r '.server.required_api_extensions[]' "$manifest")

future_nesting_extension="$(jq -er '.residual_controls.project_vm_nesting_restriction.future_api_extension' "$manifest")"
if jq -e --arg extension "$future_nesting_extension" '.api_extensions | index($extension) != null' <<<"$server_json" >/dev/null; then
  fail "server supports ${future_nesting_extension}; baseline must be upgraded to enforce the project-level restriction"
fi

project_json="$(query_object "/1.0/projects/${project}" 'project')"
network_json="$(query_object "/1.0/networks/${network}?project=default" 'default-project network')"
network_acl_json="$(query_object "/1.0/network-acls/${network_acl}?project=default" 'default-project network ACL')"
profile_json="$(query_object "/1.0/profiles/${profile}?project=${project}" 'profile')"
storage_json="$(query_object "/1.0/storage-pools/${storage_pool}" 'storage pool')"

compare_object 'project' \
  '{description: .project.description, config: .project.config}' \
  '{description: .description, config: .config}' \
  "$project_json"
compare_object 'network' \
  '{description: .network.description, type: .network.type, managed: .network.managed, config: .network.config}' \
  '{description: .description, type: .type, managed: .managed, config: .config}' \
  "$network_json"
compare_object 'network ACL' \
  '{description: .network_acl.description, config: .network_acl.config, ingress: (.network_acl.ingress | sort_by([.action, .destination, .protocol, .destination_port, .description])), egress: (.network_acl.egress | sort_by([.action, .destination, .protocol, .destination_port, .description]))}' \
  '{description: .description, config: .config, ingress: (.ingress | sort_by([.action, .destination, .protocol, .destination_port, .description])), egress: (.egress | sort_by([.action, .destination, .protocol, .destination_port, .description]))}' \
  "$network_acl_json"
compare_object 'profile' \
  '{description: .profile.description, config: .profile.config, devices: .profile.devices}' \
  '{description: .description, config: .config, devices: .devices}' \
  "$profile_json"
compare_object 'storage pool' \
  '{description: .storage_pool.description, driver: .storage_pool.driver, config: .storage_pool.config}' \
  '{description: .description, driver: .driver, config: (.config | if has("volatile.initial_source") and (."volatile.initial_source" | type != "string") then error("volatile.initial_source must be a string") else del(."volatile.initial_source") end)}' \
  "$storage_json"

printf 'NOTICE: Incus 7.0-7.2 cannot enforce VM nesting at project level; exact profile security.nesting=false is the compensating control.\n' >&2

printf 'Incus isolation baseline matches %s\n' "$manifest"

#!/usr/bin/env bash
set -Eeuo pipefail

readonly mutation_opt_in='I_UNDERSTAND_THIS_PROJECT_IS_DISPOSABLE'
readonly disposable_key='user.incus-gh-runner.disposable'

usage() {
  cat >&2 <<EOF
usage: $0 --project <project> --profile <profile> --image <local-image> \\
  --allowed-url <url> [--allowed-url <url> ...] \\
  --forbidden-url <url> [--forbidden-url <url> ...] \\
  [--egress-proxy <url>] \\
  [--runtime-probe <executable> --baseline <rendered-baseline>] \\
  [--evidence-directory <directory>] [--execute]

Without --execute, the harness only validates inputs and exports effective Incus
configuration. Live mutation additionally requires:

  * the host br_netfilter kernel module loaded;
  * a non-default project with ${disposable_key}=true; and
  * INCUS_GH_RUNNER_LIVE_MUTATION=${mutation_opt_in}

The harness creates and removes only two generated, marker-checked VMs. URLs
must be HTTP(S) origins with no path beyond an optional trailing slash, user
information, query, or fragment so they are safe to retain in the evidence
bundle. Every allowed URL is checked from both VMs; the first is reused for
spoof and recovery probes. Every forbidden URL is tested with direct proxy
bypass; when a proxy is configured, its denial policy is tested separately.
During live execution, the optional runtime-probe and baseline pair extends the
scenario with KVM, reported Secure Boot, Incus agent, IPv6 no-bypass, admission,
and bounded resource-pressure gates. Preflight never executes the runtime probe.
Without that pair, retain the effective resource and IPv6 configuration as
evidence and prove those behaviors separately when required.
EOF
}

project=''
profile=''
image=''
egress_proxy=''
runtime_probe=''
baseline=''
evidence_directory='incus-hostile-isolation-evidence'
execute=false
declare -a allowed_urls=()
declare -a forbidden_urls=()

while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --project|--profile|--image|--allowed-url|--forbidden-url|--egress-proxy|--runtime-probe|--baseline|--evidence-directory)
      [[ "$#" -ge 2 ]] || {
        printf 'missing value for %s\n' "$1" >&2
        usage
        exit 2
      }
      case "$1" in
        --project) project="$2" ;;
        --profile) profile="$2" ;;
        --image) image="$2" ;;
        --allowed-url) allowed_urls+=("$2") ;;
        --forbidden-url) forbidden_urls+=("$2") ;;
        --egress-proxy) egress_proxy="$2" ;;
        --runtime-probe) runtime_probe="$2" ;;
        --baseline) baseline="$2" ;;
        --evidence-directory) evidence_directory="$2" ;;
      esac
      shift 2
      ;;
    --execute)
      execute=true
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      usage
      exit 2
      ;;
  esac
done

[[ -n "$project" && -n "$profile" && -n "$image" ]] || {
  printf 'project, profile, and image are required\n' >&2
  usage
  exit 2
}
[[ "${#allowed_urls[@]}" -gt 0 ]] || {
  printf 'at least one allowed URL is required\n' >&2
  usage
  exit 2
}
[[ "${#forbidden_urls[@]}" -gt 0 ]] || {
  printf 'at least one forbidden URL is required\n' >&2
  usage
  exit 2
}
[[ ! -L "$evidence_directory" ]] || {
  printf 'evidence directory must not be a symbolic link: %s\n' "$evidence_directory" >&2
  exit 2
}
if [[ -n "$runtime_probe" && -z "$baseline" ]] ||
  [[ -z "$runtime_probe" && -n "$baseline" ]]; then
  printf 'runtime probe and baseline must be provided together\n' >&2
  exit 2
fi

validate_local_name() {
  local label="$1"
  local value="$2"

  [[ "${#value}" -le 63 && "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || {
    printf '%s must be a safe local name beginning with an alphanumeric character: %s\n' \
      "$label" "$value" >&2
    exit 2
  }
}

validate_local_image() {
  local value="$1"

  [[ "${#value}" -le 255 && "$value" =~ ^[A-Za-z0-9][A-Za-z0-9._/@+~-]*$ ]] || {
    printf 'image must be a safe local alias or fingerprint without a remote prefix: %s\n' \
      "$value" >&2
    exit 2
  }
}

validate_local_name project "$project"
validate_local_name profile "$profile"
validate_local_image "$image"
[[ "$project" != default ]] || {
  printf 'refusing to inspect or mutate the default Incus project\n' >&2
  exit 2
}

validate_evidence_url() {
  local label="$1"
  local value="$2"

  [[ ! "$value" =~ [[:space:]] ]] &&
    [[ "$value" =~ ^https?://[^/?#@]+/?$ ]] || {
      printf '%s must be an HTTP(S) origin without user information, path, query, fragment, or whitespace: %s\n' \
        "$label" "$value" >&2
      exit 2
    }
}

for allowed_url in "${allowed_urls[@]}"; do
  validate_evidence_url 'allowed URL' "$allowed_url"
done
if [[ -n "$egress_proxy" ]]; then
  validate_evidence_url 'egress proxy' "$egress_proxy"
fi
for forbidden_url in "${forbidden_urls[@]}"; do
  validate_evidence_url 'forbidden URL' "$forbidden_url"
done
unset allowed_url forbidden_url

spoof_probe_url="${allowed_urls[0]}"
readonly spoof_probe_url

for command_name in incus jq sha256sum; do
  command -v "$command_name" >/dev/null || {
    printf 'required command is unavailable: %s\n' "$command_name" >&2
    exit 1
  }
done

if [[ -n "$runtime_probe" ]]; then
  [[ ! -L "$runtime_probe" && -f "$runtime_probe" && -x "$runtime_probe" ]] || {
    printf 'runtime probe must be a non-symlink regular executable: %s\n' \
      "$runtime_probe" >&2
    exit 2
  }
  [[ ! -L "$baseline" && -f "$baseline" && -r "$baseline" ]] || {
    printf 'baseline must be a non-symlink readable regular file: %s\n' \
      "$baseline" >&2
    exit 2
  }
  jq --exit-status 'type == "object"' "$baseline" >/dev/null 2>&1 || {
    printf 'baseline must contain one JSON object: %s\n' "$baseline" >&2
    exit 2
  }
  runtime_probe="$(cd -- "$(dirname -- "$runtime_probe")" && pwd -P)/$(basename -- "$runtime_probe")"
  baseline="$(cd -- "$(dirname -- "$baseline")" && pwd -P)/$(basename -- "$baseline")"
fi

umask 077
mkdir -p "$evidence_directory"
if [[ -n "$(find "$evidence_directory" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
  printf 'evidence directory must be empty: %s\n' "$evidence_directory" >&2
  exit 1
fi
chmod 0700 "$evidence_directory"
evidence_directory="$(cd "$evidence_directory" && pwd)"

started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
readonly started_at
run_id="hostile-$(date -u +%Y%m%d%H%M%S)-$$"
readonly run_id
readonly vm_a="incus-gh-runner-${run_id}-a"
readonly vm_b="incus-gh-runner-${run_id}-b"
readonly results_file="${evidence_directory}/results.jsonl"
: >"$results_file"
runtime_probe_json='null'

vm_a_expected=false
vm_b_expected=false
cleanup_failed=false
completed=false

incus_cmd() {
  incus --project "$project" "$@"
}

record_result() {
  local name="$1"
  local outcome="$2"
  local detail="$3"

  jq --null-input --compact-output \
    --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg name "$name" \
    --arg outcome "$outcome" \
    --arg detail "$detail" \
    '{timestamp: $timestamp, name: $name, outcome: $outcome, detail: $detail}' \
    >>"$results_file"
}

safe_filename() {
  local value="$1"
  printf '%s' "${value//[^[:alnum:]._-]/_}"
}

capture_optional() {
  local output="$1"
  shift

  if ! "$@" >"$output" 2>"${output}.error"; then
    return 0
  fi
  rm -f -- "${output}.error"
}

snapshot_named_resources() {
  local kind="$1"
  local suffix="$2"
  local list_file="$3"
  local name
  local safe_name

  while IFS= read -r name; do
    [[ -n "$name" ]] || continue
    safe_name="$(safe_filename "$name")"
    case "$kind" in
      network)
        incus_cmd network show "$name" >"${evidence_directory}/network-${safe_name}-${suffix}.yaml"
        capture_optional \
          "${evidence_directory}/network-${safe_name}-${suffix}-leases.json" \
          incus_cmd network list-leases "$name" --format json
        ;;
      acl)
        incus_cmd network acl show "$name" >"${evidence_directory}/network-acl-${safe_name}-${suffix}.yaml"
        ;;
      storage)
        incus_cmd storage show "$name" >"${evidence_directory}/storage-${safe_name}-${suffix}.yaml"
        ;;
    esac
  done < <(jq --raw-output '.[].name // empty' "$list_file")
}

snapshot_effective_configuration() {
  local suffix="$1"

  incus project show "$project" >"${evidence_directory}/project-${suffix}.yaml"
  incus_cmd profile show "$profile" >"${evidence_directory}/profile-${suffix}.yaml"
  incus_cmd image info "$image" >"${evidence_directory}/image-${suffix}.yaml"
  incus_cmd list --format json >"${evidence_directory}/instances-${suffix}.json"
  incus_cmd network list --format json >"${evidence_directory}/networks-${suffix}.json"
  incus_cmd network acl list --format json >"${evidence_directory}/network-acls-${suffix}.json"
  incus_cmd storage list --format json >"${evidence_directory}/storage-${suffix}.json"
  incus version >"${evidence_directory}/incus-version-${suffix}.txt"
  capture_optional "${evidence_directory}/server-${suffix}.txt" incus info
  capture_optional \
    "${evidence_directory}/trusted-identities-${suffix}.json" \
    incus config trust list --format json
  capture_optional \
    "${evidence_directory}/client-remotes-${suffix}.json" \
    incus remote list --format json

  snapshot_named_resources network "$suffix" "${evidence_directory}/networks-${suffix}.json"
  snapshot_named_resources acl "$suffix" "${evidence_directory}/network-acls-${suffix}.json"
  snapshot_named_resources storage "$suffix" "${evidence_directory}/storage-${suffix}.json"
}

cleanup_instance() {
  local instance="$1"
  local expected="$2"
  local actual_id
  local disposable

  [[ "$expected" == true ]] || return 0
  if ! incus_cmd info "$instance" >/dev/null 2>&1; then
    printf '%s already absent\n' "$instance" >>"${evidence_directory}/cleanup.log"
    return 0
  fi

  actual_id="$(incus_cmd config get "$instance" user.incus-gh-runner.acceptance-id 2>/dev/null || true)"
  disposable="$(incus_cmd config get "$instance" user.incus-gh-runner.acceptance-disposable 2>/dev/null || true)"
  if [[ "$actual_id" != "$run_id" || "$disposable" != true ]]; then
    printf 'refusing to delete %s: acceptance markers changed\n' "$instance" \
      | tee -a "${evidence_directory}/cleanup.log" >&2
    cleanup_failed=true
    return 0
  fi

  if incus_cmd delete --force "$instance" >>"${evidence_directory}/cleanup.log" 2>&1; then
    printf 'deleted %s\n' "$instance" >>"${evidence_directory}/cleanup.log"
  else
    printf 'failed to delete %s\n' "$instance" \
      | tee -a "${evidence_directory}/cleanup.log" >&2
    cleanup_failed=true
  fi
}

write_checksums() {
  (
    cd "$evidence_directory"
    while IFS= read -r path; do
      sha256sum "${path#./}"
    done < <(find . -type f ! -name checksums.sha256 -print | LC_ALL=C sort)
  ) >"${evidence_directory}/checksums.sha256"
}

finish() {
  local exit_code="$?"
  local finished_at
  local mode=preflight
  local final_outcome=failed

  trap - EXIT
  set +e
  cleanup_instance "$vm_b" "$vm_b_expected"
  cleanup_instance "$vm_a" "$vm_a_expected"
  if [[ "$execute" == true ]]; then
    mode=execute
    capture_optional "${evidence_directory}/instances-after-cleanup.json" \
      incus_cmd list --format json
  fi
  if [[ "$cleanup_failed" == true && "$exit_code" -eq 0 ]]; then
    exit_code=1
  fi
  if [[ "$exit_code" -eq 0 && "$completed" == true && "$cleanup_failed" == false ]]; then
    final_outcome=passed
  fi
  finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  if ! jq --null-input \
    --arg run_id "$run_id" \
    --arg mode "$mode" \
    --arg project "$project" \
    --arg profile "$profile" \
    --arg image "$image" \
    --arg egress_proxy "$egress_proxy" \
    --arg started_at "$started_at" \
    --arg finished_at "$finished_at" \
    --arg outcome "$final_outcome" \
    --argjson runtime_probe "$runtime_probe_json" \
    --argjson allowed_urls "$(printf '%s\n' "${allowed_urls[@]}" | jq -R . | jq -s .)" \
    --argjson forbidden_urls "$(printf '%s\n' "${forbidden_urls[@]}" | jq -R . | jq -s .)" \
    --slurpfile results "$results_file" \
    '{
      version: 1,
      run_id: $run_id,
      mode: $mode,
      project: $project,
      profile: $profile,
      image: $image,
      allowed_urls: $allowed_urls,
      egress_proxy: (if $egress_proxy == "" then null else $egress_proxy end),
      forbidden_urls: $forbidden_urls,
      started_at: $started_at,
      finished_at: $finished_at,
      outcome: $outcome,
      results: $results,
      runtime_probe: $runtime_probe,
      limitations: (
        if $runtime_probe == null then
          [
            "IPv6 spoof behavior is not exercised by this bounded harness.",
            "Resource-ceiling exhaustion is not exercised by this bounded harness."
          ]
        else
          $runtime_probe.limitations
        end
      )
    }' >"${evidence_directory}/manifest.json"; then
    printf 'failed to finalize evidence manifest\n' >&2
    exit_code=1
  fi
  if ! write_checksums; then
    printf 'failed to checksum evidence bundle\n' >&2
    exit_code=1
  fi
  set -e
  exit "$exit_code"
}

trap finish EXIT

snapshot_effective_configuration before
record_result effective-configuration passed 'exported project, profile, image, network, ACL, storage, and available identity configuration'

if [[ "$execute" != true ]]; then
  completed=true
  record_result mutation-gate passed 'preflight completed without creating or modifying Incus resources'
  printf 'read-only preflight passed; evidence written to %s\n' "$evidence_directory"
  exit 0
fi

disposable_value="$(incus project get "$project" "$disposable_key" 2>/dev/null || true)"
[[ "$disposable_value" == true ]] || {
  printf 'refusing live mutation: project %s must set %s=true\n' "$project" "$disposable_key" >&2
  exit 2
}
[[ "${INCUS_GH_RUNNER_LIVE_MUTATION:-}" == "$mutation_opt_in" ]] || {
  printf 'refusing live mutation: set INCUS_GH_RUNNER_LIVE_MUTATION=%s\n' "$mutation_opt_in" >&2
  exit 2
}
record_result mutation-gate passed 'non-default disposable project and exact operator opt-in verified'

[[ -d /sys/module/br_netfilter ]] || {
  printf 'refusing live mutation: host br_netfilter kernel module is not loaded\n' >&2
  exit 1
}
record_result bridge-netfilter passed 'host br_netfilter kernel module is loaded'

for instance in "$vm_a" "$vm_b"; do
  if incus_cmd info "$instance" >/dev/null 2>&1; then
    printf 'generated acceptance instance already exists: %s\n' "$instance" >&2
    exit 1
  fi
done

launch_vm() {
  local instance="$1"

  incus_cmd launch "$image" "$instance" </dev/null \
    --vm \
    --profile "$profile" \
    --config user.incus-gh-runner.acceptance-id="$run_id" \
    --config user.incus-gh-runner.acceptance-disposable=true
}

vm_a_expected=true
launch_vm "$vm_a" >"${evidence_directory}/launch-a.log" 2>&1
vm_b_expected=true
launch_vm "$vm_b" >"${evidence_directory}/launch-b.log" 2>&1

wait_for_agent() {
  local instance="$1"

  for _ in $(seq 1 120); do
    if incus_cmd exec "$instance" -- true >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  printf 'Incus agent did not become ready for %s\n' "$instance" >&2
  return 1
}

wait_for_agent "$vm_a"
wait_for_agent "$vm_b"
incus_cmd config show "$vm_a" --expanded >"${evidence_directory}/instance-a-expanded.yaml"
incus_cmd config show "$vm_b" --expanded >"${evidence_directory}/instance-b-expanded.yaml"
record_result concurrent-vms passed 'two generated hostile VMs launched and became agent-responsive together'

run_runtime_probe() {
  local image_info="${evidence_directory}/image-runtime-probe.txt"
  local image_fingerprint
  local probe_sha256
  local probe_evidence="${evidence_directory}/runtime-probe"
  local probe_result="${probe_evidence}/result.json"
  local probe_log="${evidence_directory}/runtime-probe.log"
  local probe_exit=0
  local parsed_result=''
  local -a probe_args

  incus_cmd image info "$image" >"$image_info"
  image_fingerprint="$(awk '$1 == "Fingerprint:" {print $2}' "$image_info")"
  [[ "$image_fingerprint" =~ ^[a-f0-9]{64}$ ]] || {
    record_result runtime-probe failed 'could not resolve one immutable image fingerprint'
    printf 'could not resolve one immutable image fingerprint for %s\n' "$image" >&2
    return 1
  }
  probe_sha256="$(sha256sum "$runtime_probe" | awk '{print $1}')"
  [[ "$probe_sha256" =~ ^[a-f0-9]{64}$ ]] || {
    record_result runtime-probe failed 'could not digest the runtime probe executable'
    return 1
  }

  probe_args=(
    probe
    --baseline "$baseline"
    --project "$project"
    --profile "$profile"
    --image-fingerprint "$image_fingerprint"
    --vm-a "$vm_a"
    --vm-b "$vm_b"
    --run-id "$run_id"
    --allowed-url "$spoof_probe_url"
    --evidence-directory "$probe_evidence"
  )
  if [[ -n "$egress_proxy" ]]; then
    probe_args+=(--egress-proxy "$egress_proxy")
  fi

  "$runtime_probe" "${probe_args[@]}" >"$probe_log" 2>&1 || probe_exit="$?"
  if [[ ! -L "$probe_result" && -f "$probe_result" ]]; then
    parsed_result="$(
      jq --compact-output \
        --arg run_id "$run_id" \
        --arg image_fingerprint "$image_fingerprint" \
        --arg acceptance_sha256 "$probe_sha256" \
        'select(
          .version == 1 and
          .run_id == $run_id and
          .image_fingerprint == $image_fingerprint and
          .acceptance_sha256 == $acceptance_sha256 and
          (.source_revision | type) == "string" and
          (.source_revision | test("^[a-f0-9]{40}([a-f0-9]{24})?$")) and
          .source_modified == false and
          .stress_duration == "10m0s" and
          .poll_interval == "1s" and
          (.limitations | type) == "array" and
          ([.limitations[] | type] | all(. == "string"))
        )' "$probe_result" 2>/dev/null
    )" || true
    if [[ -n "$parsed_result" ]]; then
      runtime_probe_json="$parsed_result"
    fi
  fi

  if [[ "$probe_exit" -ne 0 ]]; then
    record_result runtime-probe failed "runtime probe exited with status ${probe_exit}"
    printf 'runtime probe failed; inspect %s\n' "$probe_log" >&2
    return 1
  fi
  if [[ "$runtime_probe_json" == null ]] ||
    ! jq --exit-status '.outcome == "passed"' <<<"$runtime_probe_json" >/dev/null; then
    record_result runtime-probe failed 'runtime probe did not emit a matching passing result'
    printf 'runtime probe did not emit a matching passing result; inspect %s\n' \
      "$probe_log" >&2
    return 1
  fi
  record_result runtime-probe passed 'KVM, reported Secure Boot, agent, IPv6 no-bypass, admission, and pressure-survival gates passed'
}

if [[ -n "$runtime_probe" ]]; then
  run_runtime_probe
fi

guest_network_state() {
  local instance="$1"

  incus_cmd exec "$instance" -- bash -s <<'GUEST_NETWORK'
set -Eeuo pipefail
interface="$(ip -json route show default | jq --exit-status --raw-output '.[0].dev')"
gateway="$(ip -json route show default | jq --exit-status --raw-output '.[0].gateway')"
mac="$(cat "/sys/class/net/${interface}/address")"
ip -json -4 address show dev "$interface" \
  | jq --exit-status \
    --arg interface "$interface" \
    --arg gateway "$gateway" \
    --arg mac "$mac" \
    '.[0].addr_info
     | map(select(.family == "inet" and .scope == "global"))
     | first
     | {interface: $interface, gateway: $gateway, mac: $mac,
        ipv4: .local, prefix: .prefixlen}'
GUEST_NETWORK
}

guest_network_state "$vm_a" >"${evidence_directory}/guest-a-network.json"
guest_network_state "$vm_b" >"${evidence_directory}/guest-b-network.json"
vm_a_ip="$(jq --exit-status --raw-output .ipv4 "${evidence_directory}/guest-a-network.json")"
vm_b_ip="$(jq --exit-status --raw-output .ipv4 "${evidence_directory}/guest-b-network.json")"
[[ "$vm_a_ip" != "$vm_b_ip" ]] || {
  printf 'hostile VMs unexpectedly share IPv4 address %s\n' "$vm_a_ip" >&2
  exit 1
}

start_probe_listener() {
  local instance="$1"
  local port="$2"

  incus_cmd exec "$instance" -- bash -s -- "$port" <<'GUEST_LISTENER'
set -Eeuo pipefail
port="$1"
command -v curl >/dev/null
command -v systemd-socket-activate >/dev/null
cat >/run/incus-gh-runner-isolation-response <<'RESPONSE'
#!/bin/sh
printf 'HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 22\r\n\r\nincus-isolation-probe\n'
RESPONSE
chmod 0700 /run/incus-gh-runner-isolation-response
nohup systemd-socket-activate \
  --listen="0.0.0.0:${port}" \
  --accept \
  -- /run/incus-gh-runner-isolation-response \
  >/run/incus-gh-runner-isolation-listener.log 2>&1 </dev/null &
printf '%s\n' "$!" >/run/incus-gh-runner-isolation-listener.pid
GUEST_LISTENER

  for _ in $(seq 1 20); do
    if incus_cmd exec "$instance" -- \
      curl --fail --silent --show-error --noproxy '*' --max-time 2 \
      "http://127.0.0.1:${port}/" \
      | grep -Fxq incus-isolation-probe; then
      return 0
    fi
    sleep 1
  done
  printf 'local probe listener did not become ready in %s\n' "$instance" >&2
  return 1
}

readonly probe_port=18080
start_probe_listener "$vm_a" "$probe_port"
start_probe_listener "$vm_b" "$probe_port"

expect_direct_block() {
  local source_instance="$1"
  local destination_ip="$2"
  local result_name="$3"
  local output="${evidence_directory}/${result_name}.log"

  if incus_cmd exec "$source_instance" -- \
    curl --silent --show-error --noproxy '*' --connect-timeout 3 --max-time 5 \
    "http://${destination_ip}:${probe_port}/" >"$output" 2>&1; then
    record_result "$result_name" failed 'unexpected cross-runner connection succeeded'
    printf 'cross-runner connection unexpectedly succeeded: %s -> %s\n' \
      "$source_instance" "$destination_ip" >&2
    return 1
  fi
  record_result "$result_name" passed 'cross-runner TCP connection was blocked'
}

expect_direct_block "$vm_a" "$vm_b_ip" cross-runner-a-to-b
expect_direct_block "$vm_b" "$vm_a_ip" cross-runner-b-to-a

expect_neighbor_block() {
  local source_instance="$1"
  local destination_ip="$2"
  local result_name="$3"
  local output="${evidence_directory}/${result_name}.log"

  incus_cmd exec "$source_instance" -- ip neighbor show to "$destination_ip" \
    >"$output" 2>&1 || true
  if grep -Eq 'lladdr[[:space:]]+[[:xdigit:]:]+([[:space:]]|$)' "$output"; then
    record_result "$result_name" failed 'destination resolved to a link-layer neighbor after the blocked connection probe'
    printf 'cross-runner neighbor unexpectedly resolved: %s -> %s\n' \
      "$source_instance" "$destination_ip" >&2
    return 1
  fi
  record_result "$result_name" passed 'destination did not resolve to a link-layer neighbor after the connection probe'
}

expect_neighbor_block "$vm_a" "$vm_b_ip" cross-runner-l2-a-to-b
expect_neighbor_block "$vm_b" "$vm_a_ip" cross-runner-l2-b-to-a

guest_egress() {
  local instance="$1"
  local url="$2"
  local output="$3"
  local -a curl_args=(
    curl --fail --silent --show-error --location
    --connect-timeout 10 --max-time 30 --output /dev/null
    --write-out '%{http_code} %{remote_ip}\n'
  )

  if [[ -n "$egress_proxy" ]]; then
    curl_args+=(--noproxy '' --proxy "$egress_proxy")
  else
    curl_args+=(--noproxy '*')
  fi
  incus_cmd exec "$instance" -- "${curl_args[@]}" "$url" >"$output" 2>&1
}

for index in "${!allowed_urls[@]}"; do
  for instance_suffix in "a:$vm_a" "b:$vm_b"; do
    suffix="${instance_suffix%%:*}"
    instance="${instance_suffix#*:}"
    result_name="allowed-egress-${suffix}-${index}"
    if guest_egress "$instance" "${allowed_urls[$index]}" \
      "${evidence_directory}/${result_name}.log"; then
      record_result "$result_name" passed "approved HTTP(S) egress destination ${index} succeeded"
    else
      record_result "$result_name" failed "approved HTTP(S) egress destination ${index} failed"
      printf 'approved egress destination %s failed from %s\n' "$index" "$instance" >&2
      exit 1
    fi
  done
done

expect_forbidden_direct_block() {
  local instance="$1"
  local url="$2"
  local result_name="$3"
  local output="${evidence_directory}/${result_name}.log"
  local -a curl_args=(
    curl --insecure --silent --show-error --location
    --connect-timeout 3 --max-time 5 --output /dev/null
    --noproxy '*'
  )

  if incus_cmd exec "$instance" -- "${curl_args[@]}" "$url" >"$output" 2>&1; then
    record_result "$result_name" failed 'forbidden destination was directly reachable while bypassing the proxy'
    printf 'forbidden destination was directly reachable from %s: %s\n' "$instance" "$url" >&2
    return 1
  fi
  record_result "$result_name" passed 'direct forbidden destination access failed while bypassing the proxy'
}

expect_forbidden_proxy_block() {
  local instance="$1"
  local url="$2"
  local result_name="$3"
  local output="${evidence_directory}/${result_name}.log"
  local -a curl_args=(
    curl --fail --insecure --silent --show-error --location
    --connect-timeout 3 --max-time 5 --output /dev/null
    --noproxy '' --proxy "$egress_proxy"
  )

  if incus_cmd exec "$instance" -- "${curl_args[@]}" "$url" >"$output" 2>&1; then
    record_result "$result_name" failed 'configured proxy allowed a forbidden destination'
    printf 'configured proxy allowed a forbidden destination from %s: %s\n' \
      "$instance" "$url" >&2
    return 1
  fi
  record_result "$result_name" passed 'configured proxy denied the forbidden destination'
}

for index in "${!forbidden_urls[@]}"; do
  expect_forbidden_direct_block \
    "$vm_a" "${forbidden_urls[$index]}" "forbidden-direct-a-${index}"
  expect_forbidden_direct_block \
    "$vm_b" "${forbidden_urls[$index]}" "forbidden-direct-b-${index}"
  if [[ -n "$egress_proxy" ]]; then
    expect_forbidden_proxy_block \
      "$vm_a" "${forbidden_urls[$index]}" "forbidden-proxy-a-${index}"
    expect_forbidden_proxy_block \
      "$vm_b" "${forbidden_urls[$index]}" "forbidden-proxy-b-${index}"
  fi
done

test_mac_spoof() {
  local instance="$1"
  local spoof_mac="$2"
  local output="$3"

  incus_cmd exec "$instance" -- bash -s -- "$spoof_probe_url" "$egress_proxy" "$spoof_mac" \
    >"$output" 2>&1 <<'GUEST_MAC_SPOOF'
set -Eeuo pipefail
allowed_url="$1"
egress_proxy="$2"
spoof_mac="$3"
interface="$(ip -json route show default | jq --exit-status --raw-output '.[0].dev')"
original_mac="$(cat "/sys/class/net/${interface}/address")"
restored=false
restore_mac() {
  if [[ "$restored" != true ]]; then
    ip link set dev "$interface" down || true
    ip link set dev "$interface" address "$original_mac" || true
    ip link set dev "$interface" up || true
    restored=true
  fi
}
trap restore_mac EXIT
curl_args=(curl --fail --silent --show-error --location --connect-timeout 5 --max-time 10 --output /dev/null)
if [[ -n "$egress_proxy" ]]; then
  curl_args+=(--noproxy '' --proxy "$egress_proxy")
else
  curl_args+=(--noproxy '*')
fi
ip link set dev "$interface" down
ip link set dev "$interface" address "$spoof_mac"
ip link set dev "$interface" up
sleep 2
if "${curl_args[@]}" "$allowed_url"; then
  printf 'approved egress succeeded with spoofed MAC %s\n' "$spoof_mac" >&2
  exit 41
fi
restore_mac
trap - EXIT
for _ in $(seq 1 10); do
  if "${curl_args[@]}" "$allowed_url"; then
    exit 0
  fi
  sleep 1
done
printf 'approved egress did not recover after restoring MAC %s\n' "$original_mac" >&2
exit 42
GUEST_MAC_SPOOF
}

if test_mac_spoof "$vm_a" 02:00:00:00:00:a1 "${evidence_directory}/mac-spoof-a.log"; then
  record_result mac-spoof-a passed 'approved egress failed under a spoofed MAC and recovered after restoration'
else
  record_result mac-spoof-a failed 'MAC spoof test or recovery failed'
  printf 'MAC spoof protection test failed; inspect mac-spoof-a.log\n' >&2
  exit 1
fi

ipv4_to_int() {
  local address="$1"
  local a b c d octet

  IFS=. read -r a b c d <<<"$address"
  for octet in "$a" "$b" "$c" "$d"; do
    [[ "$octet" =~ ^[0-9]+$ && "$octet" -le 255 ]] || return 1
  done
  printf '%u\n' "$(((10#$a << 24) | (10#$b << 16) | (10#$c << 8) | 10#$d))"
}

int_to_ipv4() {
  local value="$1"
  printf '%u.%u.%u.%u\n' \
    "$(((value >> 24) & 255))" \
    "$(((value >> 16) & 255))" \
    "$(((value >> 8) & 255))" \
    "$((value & 255))"
}

choose_spoof_ipv4() {
  local address="$1"
  local prefix="$2"
  local gateway="$3"
  local address_int mask network broadcast candidate_int candidate
  local -a used_addresses=("$address" "$gateway" "$vm_a_ip" "$vm_b_ip")

  [[ "$prefix" =~ ^[0-9]+$ && "$prefix" -ge 8 && "$prefix" -le 30 ]] || return 1
  address_int="$(ipv4_to_int "$address")"
  mask=$(((0xFFFFFFFF << (32 - prefix)) & 0xFFFFFFFF))
  network=$((address_int & mask))
  broadcast=$((network | (0xFFFFFFFF ^ mask)))

  while IFS= read -r candidate; do
    [[ -n "$candidate" ]] && used_addresses+=("$candidate")
  done < <(
    jq --raw-output '.. | .address? // empty | select(type == "string")' \
      "${evidence_directory}"/network-*-leases.json \
      "${evidence_directory}/instances-before.json" \
      2>/dev/null || true
  )

  for ((candidate_int = broadcast - 1; candidate_int > network && candidate_int >= broadcast - 64; candidate_int--)); do
    candidate="$(int_to_ipv4 "$candidate_int")"
    if ! printf '%s\n' "${used_addresses[@]}" | grep -Fxq "$candidate"; then
      printf '%s\n' "$candidate"
      return 0
    fi
  done
  return 1
}

vm_a_prefix="$(jq --exit-status --raw-output .prefix "${evidence_directory}/guest-a-network.json")"
vm_a_gateway="$(jq --exit-status --raw-output .gateway "${evidence_directory}/guest-a-network.json")"
spoof_ipv4="$(choose_spoof_ipv4 "$vm_a_ip" "$vm_a_prefix" "$vm_a_gateway")" || {
  printf 'could not choose an unused IPv4 spoof candidate in %s/%s\n' "$vm_a_ip" "$vm_a_prefix" >&2
  exit 1
}

test_ipv4_spoof() {
  local instance="$1"
  local spoof_address="$2"
  local prefix="$3"
  local output="$4"

  incus_cmd exec "$instance" -- bash -s -- \
    "$spoof_probe_url" "$egress_proxy" "$spoof_address" "$prefix" >"$output" 2>&1 <<'GUEST_IPV4_SPOOF'
set -Eeuo pipefail
allowed_url="$1"
egress_proxy="$2"
spoof_address="$3"
prefix="$4"
interface="$(ip -json route show default | jq --exit-status --raw-output '.[0].dev')"
address_added=false
remove_address() {
  if [[ "$address_added" == true ]]; then
    ip address delete "${spoof_address}/${prefix}" dev "$interface" || true
    address_added=false
  fi
}
trap remove_address EXIT
curl_args=(curl --fail --silent --show-error --location --connect-timeout 5 --max-time 10 --output /dev/null)
if [[ -n "$egress_proxy" ]]; then
  curl_args+=(--noproxy '' --proxy "$egress_proxy")
else
  curl_args+=(--noproxy '*')
fi
ip address add "${spoof_address}/${prefix}" dev "$interface"
address_added=true
if "${curl_args[@]}" --interface "$spoof_address" "$allowed_url"; then
  printf 'approved egress succeeded with spoofed IPv4 source %s\n' "$spoof_address" >&2
  exit 41
fi
remove_address
trap - EXIT
if ! "${curl_args[@]}" "$allowed_url"; then
  printf 'approved egress did not recover after removing spoofed IPv4 source\n' >&2
  exit 42
fi
GUEST_IPV4_SPOOF
}

if test_ipv4_spoof "$vm_a" "$spoof_ipv4" "$vm_a_prefix" \
  "${evidence_directory}/ipv4-spoof-a.log"; then
  record_result ipv4-spoof-a passed "approved egress failed from spoofed source ${spoof_ipv4} and recovered"
else
  record_result ipv4-spoof-a failed 'IPv4 spoof test or recovery failed'
  printf 'IPv4 spoof protection test failed; inspect ipv4-spoof-a.log\n' >&2
  exit 1
fi

snapshot_effective_configuration after
completed=true
acceptance_detail='cross-runner, destination, MAC spoof, IPv4 spoof, and approved-egress checks passed'
if [[ "$runtime_probe_json" != null ]]; then
  acceptance_detail='network isolation, KVM-device use, reported Secure Boot, agent round-trip, IPv6 no-bypass, admission ceiling, and pressure-survival checks passed'
fi
record_result hostile-isolation-acceptance passed "$acceptance_detail"
printf 'hostile VM isolation acceptance passed; evidence written to %s\n' "$evidence_directory"

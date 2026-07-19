#!/usr/bin/env bash
set -Eeuo pipefail

fail() {
  printf 'CUE Incus configuration test failed: %s\n' "$*" >&2
  exit 1
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
module_dir="$(cd -- "${script_dir}/.." && pwd)"
baseline="$(cd -- "${module_dir}/.." && pwd)/baseline.example.json"
cue_bin="${INCUS_GH_RUNNER_CUE_BIN:-cue}"

command -v "$cue_bin" >/dev/null || fail "required command is unavailable: ${cue_bin}"
for command_name in cmp jq mktemp; do
  command -v "$command_name" >/dev/null || fail "required command is unavailable: ${command_name}"
done

tmp_dir="$(mktemp -d)"
trap 'rm -rf -- "$tmp_dir"' EXIT

run_cue() {
  (cd -- "$module_dir" && "$cue_bin" "$@")
}

run_cue fmt --check --files .
run_cue mod tidy --check
run_cue vet -c ./...

[[ "$(run_cue eval cue.mod/module.cue -e module --out text)" == \
  'github.com/meigma/incus-gh-runner/config@v0' ]] || fail 'unexpected public module path'
if run_cue eval cue.mod/module.cue -e deps >/dev/null 2>&1; then
  fail 'the configuration module must remain dependency-free'
fi

run_cue export ./examples/default -e baseline --out json >"${tmp_dir}/rendered.json"
jq -S . "$baseline" >"${tmp_dir}/expected.json"
jq -S . "${tmp_dir}/rendered.json" >"${tmp_dir}/actual.json"
cmp -s "${tmp_dir}/expected.json" "${tmp_dir}/actual.json" ||
  fail 'the default CUE input no longer renders the live-proven baseline'

run_cue export ./tests -t case=customSizing -e _result.output --out json >"${tmp_dir}/custom.json"
run_cue export ./tests -t case=customSizing -e _result.controller --out json \
  >"${tmp_dir}/custom-controller.json"
jq -e '
  .project.config["limits.cpu"] == "12" and
  .project.config["limits.memory"] == "24GiB" and
  .project.config["limits.disk"] == "120GiB" and
  .project.config["limits.virtual-machines"] == "3" and
  .profile.config["limits.cpu"] == "4" and
  .profile.config["limits.memory"] == "8GiB" and
  .profile.devices.root.size == "30GiB" and
  .profile.devices.root["limits.max"] == "150MiB" and
  .profile.devices.eth0["limits.max"] == "250Mbit" and
  .network_acl.egress[2].destination_port == "8080"
' "${tmp_dir}/custom.json" >/dev/null || fail 'custom sizing was not derived consistently'
jq -e '
  .incus.project == "github-runners" and
  .incus.profiles == ["github-runner"] and
  .capacity.max_runners == 3
' "${tmp_dir}/custom-controller.json" >/dev/null ||
  fail 'controller capacity and Incus selection drifted from the derived baseline'

invalid_cases=(
  defaultProject
  insufficientCPUHeadroom
  insufficientMemoryHeadroom
  insufficientStorageHeadroom
  invalidDNS
  proxyOnDNSPort
  unknownInput
  unknownNetworkInput
  weakenDefaultEgress
  weakenSecureBoot
)

expected_failure_text() {
  case "$1" in
    defaultProject) printf '%s' 'dedicated Incus resource name must not be default' ;;
    insufficientCPUHeadroom) printf '%s' '_cpuHeadroom: invalid value' ;;
    insufficientMemoryHeadroom) printf '%s' '_memoryHeadroomGiB: invalid value' ;;
    insufficientStorageHeadroom) printf '%s' '_storageHeadroomGiB: invalid value' ;;
    invalidDNS) printf '%s' 'value must be an IPv4 address' ;;
    proxyOnDNSPort) printf '%s' 'proxy port must be between 1 and 65535' ;;
    unknownInput | unknownNetworkInput) printf '%s' 'field not allowed' ;;
    weakenDefaultEgress | weakenSecureBoot) printf '%s' 'conflicting values' ;;
    *) fail "missing expected failure text for case: $1" ;;
  esac
}

for case_name in "${invalid_cases[@]}"; do
  if run_cue export ./tests -t "case=${case_name}" -e _result.output --out json \
    >"${tmp_dir}/${case_name}.stdout" 2>"${tmp_dir}/${case_name}.stderr"; then
    fail "invalid case unexpectedly rendered: ${case_name}"
  fi
  grep -Fq "$(expected_failure_text "$case_name")" "${tmp_dir}/${case_name}.stderr" ||
    fail "invalid case failed for an unexpected reason: ${case_name}"
done

printf 'CUE Incus configuration tests passed\n'

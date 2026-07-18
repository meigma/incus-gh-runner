#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
guest_entrypoint="${repo_root}/image/guest/incus-gh-runner-guest"
bash -n \
  "${repo_root}/image/build.sh" \
  "$guest_entrypoint" \
  "${repo_root}/image/validate-incus.sh"
grep -Fq '"http:distrobuilder"' "${repo_root}/mise.toml"
grep -Fq 'command -v distrobuilder' "${repo_root}/image/build.sh"
grep -Fq 'grub-install \' "${repo_root}/image/image.yaml"
grep -Fq -- '--removable' "${repo_root}/image/image.yaml"
grep -Fq 'DISTROBUILDER_ROOT_UUID' "${repo_root}/image/image.yaml"
! grep -Fq 'image info "$alias" --format' "${repo_root}/image/validate-incus.sh"

set +e
validation_usage="$(${repo_root}/image/validate-incus.sh 2>&1)"
validation_usage_exit="$?"
set -e
[[ "$validation_usage_exit" -eq 2 ]]
grep -Fq 'usage:' <<<"$validation_usage"

test_root="$(mktemp -d)"
temp_parent="${TMPDIR:-/tmp}"
temp_parent="${temp_parent%/}"
case "$test_root" in
  "${temp_parent}"/*|/tmp/*) ;;
  *) printf 'refusing unexpected temporary directory: %s\n' "$test_root" >&2; exit 1 ;;
esac
trap 'rm -rf -- "$test_root"' EXIT

run_case() {
  local case_name="$1"
  local payload="$2"
  local expected_result="$3"
  local case_root="${test_root}/${case_name}"
  local payload_root="${case_root}/payload"
  local runner_root="${case_root}/runner"
  local output="${case_root}/output.log"
  local marker="${case_root}/runner.marker"
  local poweroff_marker="${case_root}/poweroff.marker"

  mkdir -p "$payload_root" "$runner_root"
  printf '%s\n' "$payload" >"${payload_root}/payload.json"
  : >"${payload_root}/payload.ready"

  cat >"${runner_root}/run.sh" <<'RUNNER'
#!/usr/bin/env bash
set -Eeuo pipefail
[[ "$1" == --jitconfig ]]
[[ "$2" == test-jit-secret ]]
printf 'runner-invoked\n' >"$TEST_RUNNER_MARKER"
RUNNER
  chmod 0755 "${runner_root}/run.sh"

  cat >"${case_root}/poweroff" <<'POWEROFF'
#!/usr/bin/env bash
set -Eeuo pipefail
[[ "$1" == poweroff ]]
printf 'poweroff-requested\n' >"$TEST_POWEROFF_MARKER"
POWEROFF
  chmod 0755 "${case_root}/poweroff"

  set +e
  PAYLOAD_ROOT="$payload_root" \
    RUNNER_ROOT="$runner_root" \
    RUNNER_USER= \
    POWER_OFF_BIN="${case_root}/poweroff" \
    TEST_RUNNER_MARKER="$marker" \
    TEST_POWEROFF_MARKER="$poweroff_marker" \
    "$guest_entrypoint" >"$output" 2>&1
  actual_exit="$?"
  set -e

  if [[ "$expected_result" == success ]]; then
    [[ "$actual_exit" -eq 0 ]]
  else
    [[ "$actual_exit" -ne 0 ]]
  fi
  [[ ! -e "${payload_root}/payload.json" ]]
  [[ ! -e "${payload_root}/payload.ready" ]]
  [[ -f "$poweroff_marker" ]]
  ! grep -Fq 'test-jit-secret' "$output"

  if [[ "$expected_result" == success ]]; then
    [[ -f "$marker" ]]
    jq --exit-status '.version == 1 and .state == "exited" and .exit_code == 0' \
      "${payload_root}/status.json" >/dev/null
  else
    [[ ! -e "$marker" ]]
    jq --exit-status '.version == 1 and .state == "exited" and .exit_code != 0' \
      "${payload_root}/status.json" >/dev/null
  fi
}

run_case valid '{"version":1,"jit_config":"test-jit-secret"}' success
run_case invalid '{"version":1,"jit_config":"test-jit-secret","unexpected":true}' failure

printf 'guest entrypoint contract tests passed\n'

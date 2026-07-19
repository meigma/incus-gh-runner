#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
harness="${repo_root}/scripts/live/live-incus-hostile-isolation.sh"

bash -n "$harness"
grep -Fq "readonly disposable_key='user.incus-gh-runner.disposable'" "$harness"
grep -Fq "readonly mutation_opt_in='I_UNDERSTAND_THIS_PROJECT_IS_DISPOSABLE'" "$harness"
grep -Fq 'the host br_netfilter kernel module loaded' "$harness"
grep -Fq "user.incus-gh-runner.acceptance-id=\"\$run_id\"" "$harness"
grep -Fq 'refusing to delete %s: acceptance markers changed' "$harness"
grep -Fq 'cross-runner-a-to-b' "$harness"
grep -Fq 'cross-runner-l2-a-to-b' "$harness"
grep -Fq 'mac-spoof-a' "$harness"
grep -Fq 'ipv4-spoof-a' "$harness"
grep -Fq "for index in \"\${!allowed_urls[@]}\"" "$harness"
grep -Fq "for instance_suffix in \"a:\$vm_a\" \"b:\$vm_b\"" "$harness"
grep -Fq "guest_egress \"\$instance\" \"\${allowed_urls[\$index]}\"" "$harness"
grep -Fq "\"\$spoof_probe_url\" \"\$egress_proxy\"" "$harness"
grep -Fq '[[ -d /sys/module/br_netfilter ]]' "$harness"
grep -Fq 'host br_netfilter kernel module is not loaded' "$harness"
grep -Fq "record_result bridge-netfilter passed 'host br_netfilter kernel module is loaded'" "$harness"

listener_function="$(
  sed -n '/^start_probe_listener()/,/^}/p' "$harness"
)"
grep -Fq -- '--inetd' <<<"$listener_function"
grep -Fq -- '-- /bin/sh /run/incus-gh-runner-isolation-response' \
  <<<"$listener_function"

launch_vm_function="$(
  sed -n '/^launch_vm()/,/^}/p' "$harness"
)"
grep -Fq 'incus_cmd launch "$image" "$instance" </dev/null' \
  <<<"$launch_vm_function"

direct_forbidden_function="$(
  sed -n '/^expect_forbidden_direct_block()/,/^}/p' "$harness"
)"
grep -Fq -- "--noproxy '*'" <<<"$direct_forbidden_function"
if grep -Fq -- '--proxy' <<<"$direct_forbidden_function"; then
  printf 'direct forbidden probe must not use the configured proxy\n' >&2
  exit 1
fi
proxy_forbidden_function="$(
  sed -n '/^expect_forbidden_proxy_block()/,/^}/p' "$harness"
)"
grep -Fq -- "--proxy \"\$egress_proxy\"" <<<"$proxy_forbidden_function"
grep -Fq -- "--noproxy ''" <<<"$proxy_forbidden_function"
grep -Fq -- 'curl --fail' <<<"$proxy_forbidden_function"
grep -Fq "forbidden-direct-a-\${index}" "$harness"
grep -Fq "forbidden-direct-b-\${index}" "$harness"
grep -Fq "forbidden-proxy-a-\${index}" "$harness"
grep -Fq "forbidden-proxy-b-\${index}" "$harness"

test_root="$(mktemp -d)"
temp_parent="${TMPDIR:-/tmp}"
temp_parent="${temp_parent%/}"
case "$test_root" in
  "${temp_parent}"/*|/tmp/*) ;;
  *) printf 'refusing unexpected temporary directory: %s\n' "$test_root" >&2; exit 1 ;;
esac
trap 'rm -rf -- "$test_root"' EXIT

fake_bin="${test_root}/bin"
mkdir -p "$fake_bin"

cat >"${fake_bin}/incus" <<'FAKE_INCUS'
#!/usr/bin/env bash
set -Eeuo pipefail

printf '%s\n' "$*" >>"$FAKE_INCUS_LOG"

case "$*" in
  'project show secure-test')
    printf 'name: secure-test\nconfig:\n  restricted: "true"\n'
    ;;
  'project get secure-test user.incus-gh-runner.disposable')
    printf '%s\n' "${FAKE_DISPOSABLE_VALUE:-false}"
    ;;
  '--project secure-test profile show runner')
    printf 'name: runner\nconfig: {}\ndevices: {}\n'
    ;;
  '--project secure-test image info runner/image-24.04')
    printf 'Fingerprint: test-image-fingerprint\n'
    ;;
  '--project secure-test list --format json')
    printf '[]\n'
    ;;
  '--project secure-test network list --format json')
    printf '[]\n'
    ;;
  '--project secure-test network acl list --format json')
    printf '[]\n'
    ;;
  '--project secure-test storage list --format json')
    printf '[]\n'
    ;;
  'version')
    printf 'Client version: 7.2\nServer version: 7.2\n'
    ;;
  'info')
    printf 'server_name: fake-incus\n'
    ;;
  'config trust list --format json')
    printf '[]\n'
    ;;
  'remote list --format json')
    printf '{}\n'
    ;;
  *)
    printf 'unexpected fake Incus invocation: %s\n' "$*" >&2
    exit 91
    ;;
esac
FAKE_INCUS
chmod 0755 "${fake_bin}/incus"

common_args=(
  --project secure-test
  --profile runner
  --image runner/image-24.04
  --allowed-url https://github.com/
  --allowed-url https://api.github.com
  --forbidden-url http://169.254.169.254/
)

assert_no_mutation() {
  local log="$1"

  ! grep -Eq '(^| )(launch|delete|exec|init|start|stop|config set)( |$)' "$log"
}

preflight_log="${test_root}/preflight-incus.log"
preflight_evidence="${test_root}/preflight-evidence"
FAKE_INCUS_LOG="$preflight_log" \
PATH="${fake_bin}:$PATH" \
  "$harness" "${common_args[@]}" --evidence-directory "$preflight_evidence"
assert_no_mutation "$preflight_log"
jq --exit-status \
  '.mode == "preflight" and
   .outcome == "passed" and
   .allowed_urls == ["https://github.com/", "https://api.github.com"] and
   (has("allowed_url") | not) and
   (.limitations | length == 2) and
   (.results | any(.name == "mutation-gate" and .outcome == "passed"))' \
  "${preflight_evidence}/manifest.json" >/dev/null
(cd "$preflight_evidence" && sha256sum --check checksums.sha256 >/dev/null)

missing_opt_in_log="${test_root}/missing-opt-in-incus.log"
missing_opt_in_evidence="${test_root}/missing-opt-in-evidence"
set +e
missing_opt_in_output="$({
  FAKE_INCUS_LOG="$missing_opt_in_log" \
  FAKE_DISPOSABLE_VALUE=true \
  PATH="${fake_bin}:$PATH" \
    "$harness" "${common_args[@]}" \
      --evidence-directory "$missing_opt_in_evidence" \
      --execute
} 2>&1)"
missing_opt_in_exit="$?"
set -e
[[ "$missing_opt_in_exit" -eq 2 ]]
grep -Fq 'refusing live mutation: set INCUS_GH_RUNNER_LIVE_MUTATION=' \
  <<<"$missing_opt_in_output"
assert_no_mutation "$missing_opt_in_log"
jq --exit-status '.mode == "execute" and .outcome == "failed"' \
  "${missing_opt_in_evidence}/manifest.json" >/dev/null

missing_marker_log="${test_root}/missing-marker-incus.log"
missing_marker_evidence="${test_root}/missing-marker-evidence"
set +e
missing_marker_output="$({
  FAKE_INCUS_LOG="$missing_marker_log" \
  FAKE_DISPOSABLE_VALUE=false \
  INCUS_GH_RUNNER_LIVE_MUTATION=I_UNDERSTAND_THIS_PROJECT_IS_DISPOSABLE \
  PATH="${fake_bin}:$PATH" \
    "$harness" "${common_args[@]}" \
      --evidence-directory "$missing_marker_evidence" \
      --execute
} 2>&1)"
missing_marker_exit="$?"
set -e
[[ "$missing_marker_exit" -eq 2 ]]
grep -Fq 'must set user.incus-gh-runner.disposable=true' <<<"$missing_marker_output"
assert_no_mutation "$missing_marker_log"

default_log="${test_root}/default-incus.log"
set +e
default_output="$({
  FAKE_INCUS_LOG="$default_log" \
  PATH="${fake_bin}:$PATH" \
    "$harness" \
      --project default \
      --profile runner \
      --image runner/image-24.04 \
      --allowed-url https://github.com/ \
      --forbidden-url http://169.254.169.254/ \
      --evidence-directory "${test_root}/default-evidence" \
      --execute
} 2>&1)"
default_exit="$?"
set -e
[[ "$default_exit" -eq 2 ]]
grep -Fq 'refusing to inspect or mutate the default Incus project' <<<"$default_output"
[[ ! -e "$default_log" ]]

assert_input_rejected() {
  local case_name="$1"
  local expected="$2"
  shift 2
  local log="${test_root}/${case_name}-incus.log"
  local evidence="${test_root}/${case_name}-evidence"
  local output
  local exit_code

  set +e
  output="$({
    FAKE_INCUS_LOG="$log" \
    PATH="${fake_bin}:$PATH" \
      "$harness" "$@" --evidence-directory "$evidence"
  } 2>&1)"
  exit_code="$?"
  set -e

  [[ "$exit_code" -eq 2 ]]
  grep -Fq "$expected" <<<"$output"
  [[ ! -e "$log" ]]
  [[ ! -e "$evidence" ]]
}

assert_input_rejected unsafe-userinfo \
  'must be an HTTP(S) origin without user information' \
  --project secure-test --profile runner --image runner/image-24.04 \
  --allowed-url 'https://token@github.com/' \
  --forbidden-url http://169.254.169.254/
assert_input_rejected allowed-path \
  'must be an HTTP(S) origin without user information' \
  --project secure-test --profile runner --image runner/image-24.04 \
  --allowed-url https://github.com/actions \
  --forbidden-url http://169.254.169.254/
assert_input_rejected forbidden-path \
  'must be an HTTP(S) origin without user information' \
  --project secure-test --profile runner --image runner/image-24.04 \
  --allowed-url https://github.com/ \
  --forbidden-url http://169.254.169.254/latest/meta-data/
assert_input_rejected allowed-query \
  'must be an HTTP(S) origin without user information' \
  --project secure-test --profile runner --image runner/image-24.04 \
  --allowed-url 'https://github.com/?token=secret' \
  --forbidden-url http://169.254.169.254/
assert_input_rejected forbidden-fragment \
  'must be an HTTP(S) origin without user information' \
  --project secure-test --profile runner --image runner/image-24.04 \
  --allowed-url https://github.com/ \
  --forbidden-url 'http://169.254.169.254/#metadata'
assert_input_rejected proxy-path \
  'must be an HTTP(S) origin without user information' \
  --project secure-test --profile runner --image runner/image-24.04 \
  --allowed-url https://github.com/ \
  --forbidden-url http://169.254.169.254/ \
  --egress-proxy http://192.0.2.10/proxy

assert_input_rejected project-leading-option \
  'project must be a safe local name' \
  --project -secure --profile runner --image runner/image-24.04 \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/
assert_input_rejected project-whitespace \
  'project must be a safe local name' \
  --project 'secure test' --profile runner --image runner/image-24.04 \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/
assert_input_rejected project-invalid-character \
  'project must be a safe local name' \
  --project secure/test --profile runner --image runner/image-24.04 \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/

assert_input_rejected profile-leading-option \
  'profile must be a safe local name' \
  --project secure-test --profile -runner --image runner/image-24.04 \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/
assert_input_rejected profile-whitespace \
  'profile must be a safe local name' \
  --project secure-test --profile 'runner profile' --image runner/image-24.04 \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/
assert_input_rejected profile-invalid-character \
  'profile must be a safe local name' \
  --project secure-test --profile runner/profile --image runner/image-24.04 \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/

assert_input_rejected image-leading-option \
  'image must be a safe local alias or fingerprint' \
  --project secure-test --profile runner --image -runner-image \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/
assert_input_rejected image-whitespace \
  'image must be a safe local alias or fingerprint' \
  --project secure-test --profile runner --image 'runner image' \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/
assert_input_rejected image-remote-prefix \
  'image must be a safe local alias or fingerprint' \
  --project secure-test --profile runner --image images:ubuntu/24.04 \
  --allowed-url https://github.com/ --forbidden-url http://169.254.169.254/

printf 'hostile Incus isolation harness contract tests passed\n'

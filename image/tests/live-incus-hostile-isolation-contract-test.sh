#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
harness="${repo_root}/scripts/live/live-incus-hostile-isolation.sh"

bash -n "$harness"
grep -Fq "readonly disposable_key='user.incus-gh-runner.disposable'" "$harness"
grep -Fq "readonly mutation_opt_in='I_UNDERSTAND_THIS_PROJECT_IS_DISPOSABLE'" "$harness"
grep -Fq "incus_cmd launch \"\$image\" \"\$instance\"" "$harness"
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
grep -Fq -- '--runtime-probe <executable> --baseline <rendered-baseline>' "$harness"
grep -Fq 'runtime probe and baseline must be provided together' "$harness"
grep -Fq 'runtime probe must be a non-symlink regular executable' "$harness"
grep -Fq 'baseline must be a non-symlink readable regular file' "$harness"
grep -Fq 'runtime_probe: $runtime_probe' "$harness"
grep -Fq '.acceptance_sha256 == $acceptance_sha256' "$harness"
grep -Fq '.source_revision | test("^[a-f0-9]{40}([a-f0-9]{24})?$")' "$harness"
grep -Fq '.source_modified == false' "$harness"
grep -Fq '.stress_duration == "10m0s"' "$harness"

runtime_probe_function="$(
  sed -n '/^run_runtime_probe()/,/^}/p' "$harness"
)"
for expected in \
  'incus_cmd image info "$image" >"$image_info"' \
  '--baseline "$baseline"' \
  '--project "$project"' \
  '--profile "$profile"' \
  '--image-fingerprint "$image_fingerprint"' \
  '--vm-a "$vm_a"' \
  '--vm-b "$vm_b"' \
  '--run-id "$run_id"' \
  '--allowed-url "$spoof_probe_url"' \
  '--evidence-directory "$probe_evidence"'; do
  grep -Fq -- "$expected" <<<"$runtime_probe_function"
done

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

fake_runtime_probe="${test_root}/incus-gh-runner-acceptance"
cat >"$fake_runtime_probe" <<'FAKE_RUNTIME_PROBE'
#!/usr/bin/env bash
set -Eeuo pipefail

: >"${FAKE_RUNTIME_PROBE_CALLED:?}"
exit 97
FAKE_RUNTIME_PROBE
chmod 0755 "$fake_runtime_probe"

baseline="${test_root}/baseline.json"
printf '{}\n' >"$baseline"
runtime_probe_symlink="${test_root}/runtime-probe-symlink"
baseline_symlink="${test_root}/baseline-symlink.json"
invalid_baseline="${test_root}/invalid-baseline.json"
ln -s "$fake_runtime_probe" "$runtime_probe_symlink"
ln -s "$baseline" "$baseline_symlink"
printf 'not-json\n' >"$invalid_baseline"

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
   .runtime_probe == null and
   .allowed_urls == ["https://github.com/", "https://api.github.com"] and
   (has("allowed_url") | not) and
   (.limitations | length == 2) and
   (.results | any(.name == "mutation-gate" and .outcome == "passed"))' \
  "${preflight_evidence}/manifest.json" >/dev/null
(cd "$preflight_evidence" && sha256sum --check checksums.sha256 >/dev/null)

paired_preflight_log="${test_root}/paired-preflight-incus.log"
paired_preflight_evidence="${test_root}/paired-preflight-evidence"
probe_called="${test_root}/runtime-probe-called"
FAKE_INCUS_LOG="$paired_preflight_log" \
FAKE_RUNTIME_PROBE_CALLED="$probe_called" \
PATH="${fake_bin}:$PATH" \
  "$harness" "${common_args[@]}" \
    --runtime-probe "$fake_runtime_probe" \
    --baseline "$baseline" \
    --evidence-directory "$paired_preflight_evidence"
assert_no_mutation "$paired_preflight_log"
[[ ! -e "$probe_called" ]]
jq --exit-status \
  '.mode == "preflight" and
   .outcome == "passed" and
   .runtime_probe == null and
   (.limitations | length == 2) and
   (.results | any(.name == "mutation-gate" and .outcome == "passed"))' \
  "${paired_preflight_evidence}/manifest.json" >/dev/null
(cd "$paired_preflight_evidence" && sha256sum --check checksums.sha256 >/dev/null)

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

assert_input_rejected runtime-probe-without-baseline \
  'runtime probe and baseline must be provided together' \
  "${common_args[@]}" --runtime-probe "$fake_runtime_probe"
assert_input_rejected baseline-without-runtime-probe \
  'runtime probe and baseline must be provided together' \
  "${common_args[@]}" --baseline "$baseline"
assert_input_rejected runtime-probe-symlink \
  'runtime probe must be a non-symlink regular executable' \
  "${common_args[@]}" \
  --runtime-probe "$runtime_probe_symlink" --baseline "$baseline"
assert_input_rejected baseline-symlink \
  'baseline must be a non-symlink readable regular file' \
  "${common_args[@]}" \
  --runtime-probe "$fake_runtime_probe" --baseline "$baseline_symlink"
assert_input_rejected invalid-baseline \
  'baseline must contain one JSON object' \
  "${common_args[@]}" \
  --runtime-probe "$fake_runtime_probe" --baseline "$invalid_baseline"

printf 'hostile Incus isolation harness contract tests passed\n'

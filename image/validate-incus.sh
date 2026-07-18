#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf 'usage: %s <incus-project> <unified-image.tar.xz>\n' "$0" >&2
}

if [[ "$#" -ne 2 ]]; then
  usage
  exit 2
fi

project="$1"
archive="$2"

if [[ "$project" == default ]]; then
  printf 'refusing to run against the default Incus project\n' >&2
  exit 2
fi

for command_name in incus jq sort; do
  command -v "$command_name" >/dev/null || {
    printf 'required command is unavailable: %s\n' "$command_name" >&2
    exit 1
  }
done

[[ -f "$archive" ]] || {
  printf 'image archive does not exist: %s\n' "$archive" >&2
  exit 1
}

incus_cmd() {
  incus --project "$project" "$@"
}

incus project show "$project" >/dev/null

minimum_server_version=7.0
server_version="$(incus version | sed -n 's/^Server version: //p')"
if [[ -z "$server_version" ]] || \
  [[ "$(printf '%s\n%s\n' "$minimum_server_version" "$server_version" | sort -V | head -n 1)" != "$minimum_server_version" ]]; then
  printf 'Incus server %s or newer is required; found %s\n' \
    "$minimum_server_version" \
    "${server_version:-unknown}" >&2
  exit 1
fi

token="$(date -u +%Y%m%d%H%M%S)-$$"
instance="incus-gh-runner-probe-${token}"
alias="incus-gh-runner-probe-${token}"
fingerprint=''
instance_created=false
alias_created=false
image_owned=false

temp_root="$(mktemp -d)"
temp_parent="${TMPDIR:-/tmp}"
temp_parent="${temp_parent%/}"
case "$temp_root" in
  "${temp_parent}"/*|/tmp/*) ;;
  *) printf 'refusing unexpected temporary directory: %s\n' "$temp_root" >&2; exit 1 ;;
esac

cleanup() {
  local exit_code="$?"

  trap - EXIT
  if [[ "$instance_created" == true ]]; then
    incus_cmd delete --force "$instance" >/dev/null 2>&1 || true
  fi
  if [[ "$alias_created" == true ]]; then
    incus_cmd image alias delete "$alias" >/dev/null 2>&1 || true
  fi
  if [[ "$image_owned" == true && -n "$fingerprint" ]]; then
    incus_cmd image delete "$fingerprint" >/dev/null 2>&1 || true
  fi
  rm -rf -- "$temp_root"
  exit "$exit_code"
}

trap cleanup EXIT

if incus_cmd info "$instance" >/dev/null 2>&1; then
  printf 'generated probe instance already exists: %s\n' "$instance" >&2
  exit 1
fi
if incus_cmd image info "$alias" >/dev/null 2>&1; then
  printf 'generated probe image alias already exists: %s\n' "$alias" >&2
  exit 1
fi

preexisting_fingerprints="$(incus_cmd image list --format json | jq --raw-output '.[].fingerprint')"
incus_cmd image import "$archive" --alias "$alias"
alias_created=true
fingerprint="$(
  incus_cmd image list --format json |
    jq --arg alias "$alias" --exit-status --raw-output \
      '.[] | select(any(.aliases[]?; .name == $alias)) | .fingerprint'
)"
if ! grep -Fxq "$fingerprint" <<<"$preexisting_fingerprints"; then
  image_owned=true
fi

incus_cmd launch "$alias" "$instance" --vm
instance_created=true

agent_ready=false
for _ in $(seq 1 120); do
  if incus_cmd exec "$instance" -- true >/dev/null 2>&1; then
    agent_ready=true
    break
  fi
  sleep 1
done
[[ "$agent_ready" == true ]] || {
  printf 'Incus agent did not become ready for %s\n' "$instance" >&2
  exit 1
}

cat >"${temp_root}/run.sh" <<'RUNNER'
#!/usr/bin/env bash
set -Eeuo pipefail
[[ "$1" == --jitconfig ]]
[[ "$2" == phase2-probe-* ]]
printf 'probe-running\n' >/opt/actions-runner/probe.marker
sleep 15
RUNNER
chmod 0755 "${temp_root}/run.sh"

probe_secret="phase2-probe-${token}"
jq --null-input --compact-output \
  --arg jit_config "$probe_secret" \
  '{version: 1, jit_config: $jit_config}' >"${temp_root}/payload.json"
: >"${temp_root}/payload.ready"

incus_cmd file push "${temp_root}/run.sh" \
  "${instance}/opt/actions-runner/run.sh" \
  --uid 0 --gid 0 --mode 0755
incus_cmd file push "${temp_root}/payload.json" \
  "${instance}/run/incus-gh-runner/payload.json" \
  --uid 0 --gid 0 --mode 0600
incus_cmd file push "${temp_root}/payload.ready" \
  "${instance}/run/incus-gh-runner/payload.ready" \
  --uid 0 --gid 0 --mode 0600

guest_running=false
for _ in $(seq 1 30); do
  status_json="$(incus_cmd file pull "${instance}/run/incus-gh-runner/status.json" - 2>/dev/null || true)"
  if jq --exit-status '.version == 1 and .state == "running"' <<<"$status_json" >/dev/null 2>&1; then
    guest_running=true
    break
  fi
  sleep 1
done
[[ "$guest_running" == true ]] || {
  printf 'guest did not report running state\n' >&2
  exit 1
}

incus_cmd exec "$instance" -- test ! -e /run/incus-gh-runner/payload.json
incus_cmd exec "$instance" -- test ! -e /run/incus-gh-runner/payload.ready
incus_cmd exec "$instance" -- test -f /opt/actions-runner/probe.marker

guest_exited=false
for _ in $(seq 1 60); do
  status_json="$(incus_cmd file pull "${instance}/run/incus-gh-runner/status.json" - 2>/dev/null || true)"
  if jq --exit-status '.version == 1 and .state == "exited" and .exit_code == 0' \
    <<<"$status_json" >/dev/null 2>&1; then
    guest_exited=true
    break
  fi
  sleep 1
done
[[ "$guest_exited" == true ]] || {
  printf 'guest did not report terminal success\n' >&2
  exit 1
}

console_ready=false
for _ in $(seq 1 10); do
  console_log="$(incus_cmd console "$instance" --show-log)"
  if grep -Fq 'incus-gh-runner-guest action=poweroff exit_code=0' <<<"$console_log"; then
    console_ready=true
    break
  fi
  sleep 1
done
[[ "$console_ready" == true ]] || {
  printf 'guest serial console did not report terminal poweroff intent\n' >&2
  exit 1
}

grep -Fq 'incus-gh-runner-guest state=starting' <<<"$console_log"
grep -Fq 'incus-gh-runner-guest state=running' <<<"$console_log"
grep -Fq 'incus-gh-runner-guest state=exited' <<<"$console_log"
if grep -Fq "$probe_secret" <<<"$console_log"; then
  printf 'transient probe secret leaked to the serial console\n' >&2
  exit 1
fi

guest_stopped=false
for _ in $(seq 1 60); do
  instance_status="$(incus_cmd list "$instance" --format json | jq --exit-status --raw-output '.[0].status')"
  if [[ "$instance_status" == Stopped ]]; then
    guest_stopped=true
    break
  fi
  sleep 1
done
[[ "$guest_stopped" == true ]] || {
  printf 'guest did not reach terminal poweroff\n' >&2
  exit 1
}

printf 'Incus guest contract validated for %s\n' "$archive"

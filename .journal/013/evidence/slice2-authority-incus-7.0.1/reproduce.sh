#!/usr/bin/env bash
set -Eeuo pipefail

readonly project='runner-authority-spike'
readonly profile='default'
readonly pool='default'
readonly network='incusbr0'
readonly image='authority-test'
readonly instance='authority-probe'
readonly foreign_instance='authority-foreign-sentinel'
readonly address='https://127.0.0.1:8443'
readonly evidence='/home/ubuntu/incus-authority-evidence'

client_conf=''
client_fingerprint=''
trust_added=false
project_created=false

mkdir -p "$evidence"
chmod 0700 "$evidence"
: >"$evidence/results.tsv"

record() {
  printf '%s\t%s\t%s\n' "$1" "$2" "$3" | tee -a "$evidence/results.tsv"
}

cleanup() {
  local exit_code="$?"
  trap - EXIT
  set +e

  for name in \
    authority-probe \
    bad-hostdisk \
    bad-unix \
    bad-nic \
    bad-extra-disk \
    bad-raw-qemu \
    bad-container; do
    incus delete --force "$name" --project "$project" >/dev/null 2>&1 || true
  done
  incus delete --force "$foreign_instance" --project default >/dev/null 2>&1 || true
  incus profile device remove "$profile" hostdisk --project "$project" >/dev/null 2>&1 || true
  if [[ "$trust_added" == true && -n "$client_fingerprint" ]]; then
    incus config trust remove "$client_fingerprint" >/dev/null 2>&1 || true
  fi
  if [[ "$project_created" == true ]]; then
    incus project delete "$project" >/dev/null 2>&1 || true
  fi
  incus config set core.https_address='' >/dev/null 2>&1 || true
  if [[ -n "$client_conf" ]]; then
    rm -rf -- "$client_conf"
  fi
  exit "$exit_code"
}
trap cleanup EXIT

expect_denied() {
  local label="$1"
  shift
  if "$@" >"$evidence/${label}.stdout" 2>"$evidence/${label}.stderr"; then
    record "$label" FAIL 'operation unexpectedly succeeded'
    return 1
  fi
  record "$label" PASS 'operation was denied'
}

expect_not_visible() {
  local label="$1"
  shift
  if "$@" >"$evidence/${label}.stdout" 2>"$evidence/${label}.stderr"; then
    if jq -e --arg foreign "$foreign_instance" \
      'all(.[]; (.name // "") != $foreign)' \
      "$evidence/${label}.stdout" >/dev/null; then
      record "$label" PASS 'foreign instance was filtered from the response'
      return 0
    fi
    record "$label" FAIL 'foreign instance was visible'
    return 1
  fi
  record "$label" PASS 'foreign project request was denied'
}

rincus() {
  INCUS_CONF="$client_conf" incus "$@"
}

incus version >"$evidence/incus-version.txt"
incus info >"$evidence/server-before.txt"
if [[ -e /var/lib/incus/server.ca ]]; then
  record server-ca-absent FAIL '/var/lib/incus/server.ca exists'
  exit 1
fi
record server-ca-absent PASS 'no CA-wide client trust fallback exists'

incus image info "$image" --project default >"$evidence/image.txt"
incus init "$image" "$foreign_instance" --project default </dev/null

incus project create "$project" \
  -c features.profiles=true \
  -c features.images=false \
  -c features.networks=false \
  -c features.networks.zones=false \
  -c features.storage.volumes=false \
  -c features.storage.buckets=false \
  -c restricted=true \
  -c restricted.devices.disk=block \
  -c restricted.devices.nic=managed \
  -c restricted.networks.access="$network" \
  -c restricted.storage-pools.access="$pool" \
  -c restricted.virtual-machines.lowlevel=block \
  -c limits.containers=1 \
  -c limits.virtual-machines=1 \
  -c limits.instances=1 \
  </dev/null
project_created=true

incus profile device add "$profile" root disk \
  path=/ pool="$pool" --project "$project"
incus profile device add "$profile" eth0 nic \
  network="$network" name=eth0 --project "$project"
incus project show "$project" >"$evidence/project.yaml"
incus profile show "$profile" --project "$project" >"$evidence/profile.yaml"

incus config set core.https_address=127.0.0.1:8443
client_conf="$(mktemp -d)"
INCUS_CONF="$client_conf" incus remote generate-certificate
client_fingerprint="$(
  openssl x509 -in "$client_conf/client.crt" -outform DER |
    sha256sum | awk '{print $1}'
)"

incus config trust add-certificate "$client_conf/client.crt" \
  --name runner-authority-spike \
  --restricted \
  --projects "$project"
trust_added=true
incus config trust show "$client_fingerprint" >"$evidence/restricted-identity.yaml"
openssl x509 -in "$client_conf/client.crt" -noout -subject -issuer -fingerprint -sha256 \
  >"$evidence/client-certificate.txt"

INCUS_CONF="$client_conf" incus remote add restricted "$address" \
  --accept-certificate \
  --project "$project"

client_server_fingerprint="$(
  openssl x509 \
    -in "$client_conf/servercerts/restricted.crt" \
    -noout -fingerprint -sha256
)"
daemon_server_fingerprint="$(
  openssl x509 \
    -in /var/lib/incus/server.crt \
    -noout -fingerprint -sha256
)"
if [[ "$client_server_fingerprint" != "$daemon_server_fingerprint" ]]; then
  record pinned-server-certificate FAIL 'client and daemon fingerprints differ'
  exit 1
fi
printf '%s\n' "$client_server_fingerprint" >"$evidence/server-certificate-fingerprint.txt"
record pinned-server-certificate PASS 'client pins the exact daemon certificate'

project_list="$(rincus project list restricted: --format json | jq -r '.[].name')"
if [[ "$project_list" != "$project" ]]; then
  record project-inventory FAIL "visible projects: $project_list"
  exit 1
fi
record project-inventory PASS 'restricted identity sees only its assigned project'
rincus list restricted: --project "$project" >"$evidence/instances-before.txt"

rincus init "restricted:$image" "restricted:$instance" --project "$project" </dev/null
rincus start "restricted:$instance" --project "$project"

pushed=false
for _ in $(seq 1 60); do
  if printf '%s' authority-probe |
      rincus file push - "restricted:$instance/tmp/authority-probe" \
        --project "$project" >/dev/null 2>&1; then
    pushed=true
    break
  fi
  sleep 1
done
if [[ "$pushed" != true ]]; then
  record guest-agent-file-push FAIL 'guest agent never accepted a file push'
  exit 1
fi
record guest-agent-file-push PASS 'restricted identity wrote through the guest agent'

pulled="$(
  rincus file pull "restricted:$instance/tmp/authority-probe" - \
    --project "$project"
)"
if [[ "$pulled" != authority-probe ]]; then
  record guest-agent-file-pull FAIL 'pulled content mismatch'
  exit 1
fi
record guest-agent-file-pull PASS 'restricted identity read through the guest agent'

rincus console "restricted:$instance" --show-log --project "$project" \
  >"$evidence/console.log"
record console-log PASS 'restricted identity read the console log'
rincus stop "restricted:$instance" --force --project "$project"
rincus delete "restricted:$instance" --project "$project"
if [[ -n "$(rincus list restricted: --project "$project" --format csv --columns n)" ]]; then
  record lifecycle-delete FAIL 'instance remained after delete'
  exit 1
fi
record lifecycle-create-start-stop-delete PASS 'complete asynchronous lifecycle succeeded'

expect_not_visible foreign-all-projects \
  rincus list restricted: --all-projects --format json
expect_not_visible foreign-default-project \
  rincus list restricted: --project default --format json
expect_denied foreign-project-metadata \
  rincus project show restricted:default
expect_denied weaken-project-restrictions \
  rincus project set "restricted:$project" restricted=false
expect_denied global-server-config \
  rincus config set restricted: core.https_address=127.0.0.1:9443
expect_denied self-escalate-certificate \
  rincus query "restricted:/1.0/certificates/$client_fingerprint" \
    -X PATCH \
    -d '{"restricted":false,"projects":[]}'

expect_denied forbidden-host-path \
  bash -c 'INCUS_CONF="$1" incus init "restricted:$2" restricted:bad-hostdisk --project "$3" <<'"'"'YAML'"'"'
devices:
  hostdisk:
    type: disk
    source: /etc
    path: /mnt/host
YAML' _ "$client_conf" "$image" "$project"

expect_denied forbidden-unix-device \
  bash -c 'INCUS_CONF="$1" incus init "restricted:$2" restricted:bad-unix --project "$3" <<'"'"'YAML'"'"'
devices:
  hostdev:
    type: unix-char
    source: /dev/null
YAML' _ "$client_conf" "$image" "$project"

expect_denied forbidden-unmanaged-nic \
  bash -c 'INCUS_CONF="$1" incus init "restricted:$2" restricted:bad-nic --project "$3" <<YAML
devices:
  eth1:
    type: nic
    nictype: bridged
    parent: "$4"
YAML' _ "$client_conf" "$image" "$project" "$network"

expect_denied forbidden-extra-disk \
  bash -c 'INCUS_CONF="$1" incus init "restricted:$2" restricted:bad-extra-disk --project "$3" <<YAML
devices:
  extra:
    type: disk
    pool: "$4"
    source: nonexistent
    path: /mnt/extra
YAML' _ "$client_conf" "$image" "$project" "$pool"

expect_denied forbidden-raw-qemu \
  bash -c 'INCUS_CONF="$1" incus init --empty restricted:bad-raw-qemu --project "$2" --vm -c raw.qemu=-nodefaults </dev/null' \
    _ "$client_conf" "$project"
expect_denied forbidden-profile-host-path \
  rincus profile device add "restricted:$profile" hostdisk disk \
    source=/etc path=/mnt/host \
    --project "$project"

incus project set "$project" limits.containers=0
expect_denied forbidden-container \
  bash -c 'INCUS_CONF="$1" incus init "restricted:$2" restricted:bad-container --project "$3" </dev/null' \
    _ "$client_conf" "$image" "$project"

incus config trust remove "$client_fingerprint"
trust_added=false
expect_denied revoked-certificate \
  rincus list restricted: --project "$project"

incus project show "$project" >"$evidence/project-final.yaml"
incus profile show "$profile" --project "$project" >"$evidence/profile-final.yaml"
incus info >"$evidence/server-final.txt"
record authority-spike PASS 'restricted TLS lifecycle and negative boundaries passed'
(
  cd "$evidence"
  sha256sum -- * >checksums.sha256
)

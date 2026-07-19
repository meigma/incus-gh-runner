#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf 'usage: %s <project> <image-alias> <image-archive> <image-validator> <lifecycle-test>\n' "$0" >&2
}

if [[ "$#" -ne 5 ]]; then
  usage
  exit 2
fi

project="$1"
image_alias="$2"
archive="$3"
validator="$4"
lifecycle_test="$5"

if [[ "${EUID}" -ne 0 ]]; then
  printf '%s must run as root\n' "$0" >&2
  exit 1
fi
if [[ "$project" == default ]]; then
  printf 'refusing to prepare the default Incus project\n' >&2
  exit 2
fi
for path in "$archive" "$validator" "$lifecycle_test"; do
  [[ -f "$path" ]] || {
    printf 'required artifact is unavailable: %s\n' "$path" >&2
    exit 1
  }
done

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install --yes incus jq qemu-system
systemctl enable --now incus

[[ -c /dev/kvm ]] || {
  printf 'KVM device is unavailable on the live-test host\n' >&2
  exit 1
}

incus admin init --minimal
incus project create "$project" \
  --config features.images=true \
  --config features.profiles=false

evidence_directory=/var/log/incus-gh-runner
install -d -m 0700 "$evidence_directory"

"$validator" "$project" "$archive" 2>&1 | tee "${evidence_directory}/image-validation.log"

if incus --project "$project" image info "$image_alias" >/dev/null 2>&1; then
  printf 'persistent live-test image alias already exists: %s\n' "$image_alias" >&2
  exit 1
fi
incus --project "$project" image import "$archive" --alias "$image_alias"

INCUS_GH_RUNNER_TEST_PROJECT="$project" \
INCUS_GH_RUNNER_TEST_IMAGE="$image_alias" \
"$lifecycle_test" \
  -test.run '^TestIncusLifecycleFunctional$' \
  -test.count=1 \
  -test.v \
  2>&1 | tee "${evidence_directory}/incus-lifecycle.log"

printf 'image validation and Incus lifecycle checks passed in Incus project %s\n' "$project"

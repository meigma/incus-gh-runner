#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "$(uname -s)" != Linux ]]; then
  printf 'image/build.sh requires a Linux host\n' >&2
  exit 1
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
output_arg="${1:-build/reference-image}"
if [[ "$output_arg" = /* ]]; then
  output_dir="$output_arg"
else
  output_dir="${repo_root}/${output_arg}"
fi

mkdir -p "$output_dir"
if find "$output_dir" -mindepth 1 -maxdepth 1 -print -quit | grep -q .; then
  printf 'output directory must be empty: %s\n' "$output_dir" >&2
  exit 1
fi

tool_root="$(mktemp -d)"
temp_parent="${TMPDIR:-/tmp}"
temp_parent="${temp_parent%/}"
case "$tool_root" in
  "${temp_parent}"/*|/tmp/*) ;;
  *) printf 'refusing unexpected temporary directory: %s\n' "$tool_root" >&2; exit 1 ;;
esac
trap 'rm -rf -- "$tool_root"' EXIT

distrobuilder_version=3.3.1
distrobuilder_archive="${tool_root}/distrobuilder-${distrobuilder_version}.tar.gz"
distrobuilder_source="${tool_root}/distrobuilder-${distrobuilder_version}"
distrobuilder_bin="${tool_root}/distrobuilder"

curl --fail --location --silent --show-error \
  "https://github.com/lxc/distrobuilder/releases/download/v${distrobuilder_version}/distrobuilder-${distrobuilder_version}.tar.gz" \
  --output "$distrobuilder_archive"
echo "6c411af7178bb55ef649c708f4f38fc3c30e6ecce901c08d8a389448a900a73a  ${distrobuilder_archive}" | sha256sum --check --strict
tar --extract --gzip --file "$distrobuilder_archive" --directory "$tool_root"

(
  cd "$distrobuilder_source"
  go build \
    -mod=vendor \
    -tags=containers_image_storage_stub,containers_image_docker_daemon_stub,containers_image_openpgp \
    -trimpath \
    -o "$distrobuilder_bin" \
    ./distrobuilder
)

cd "$repo_root"
sudo --non-interactive "$distrobuilder_bin" validate image/image.yaml
sudo --non-interactive "$distrobuilder_bin" build-incus \
  image/image.yaml "$output_dir" \
  --vm \
  --type=unified

sudo chown -R "$(id -u):$(id -g)" "$output_dir"
archive="${output_dir}/incus-gh-runner-ubuntu-24.04-x86_64.tar.xz"
[[ -f "$archive" ]] || {
  printf 'expected image archive was not produced: %s\n' "$archive" >&2
  exit 1
}

(
  cd "$output_dir"
  sha256sum "$(basename "$archive")" >"$(basename "$archive").sha256"
)

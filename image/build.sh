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

distrobuilder_bin="$(command -v distrobuilder || true)"
if [[ -z "$distrobuilder_bin" ]]; then
  printf 'distrobuilder is not on PATH; run through mise: mise exec -- image/build.sh %q\n' "$output_arg" >&2
  exit 1
fi

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

#!/usr/bin/env bash
set -euo pipefail

unit_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
unit="$unit_dir/incus-gh-runner.service"

if [[ "$(uname -s)" != "Linux" ]]; then
  printf 'systemd unit verification skipped outside Linux\n'
  exit 0
fi

command -v systemd-analyze >/dev/null
sandbox="$(mktemp -d)"
cleanup() {
  rm -rf -- "$sandbox"
}
trap cleanup EXIT

mkdir -p "$sandbox/usr/lib/systemd/system" "$sandbox/usr/bin" "$sandbox/etc"
cp -a /usr/lib/systemd/system/. "$sandbox/usr/lib/systemd/system/"
install -m 0644 "$unit" "$sandbox/usr/lib/systemd/system/incus-gh-runner.service"
install -m 0755 /bin/true "$sandbox/usr/bin/incus-gh-runner"
printf 'root:x:0:\nincus-admin:x:999:\n' >"$sandbox/etc/group"

systemd-analyze verify --root="$sandbox" incus-gh-runner.service
# systemd expresses the displayed 0.0-10.0 exposure score as 0-100 here.
systemd-analyze security --offline=yes --threshold=50 "$unit" >/dev/null
printf 'systemd unit verification passed\n'

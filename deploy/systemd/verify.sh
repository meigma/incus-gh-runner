#!/usr/bin/env bash
set -euo pipefail

unit_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
unit="$unit_dir/incus-gh-runner.service"
app_credentials="$unit_dir/credentials-github-app.conf"
pat_credentials="$unit_dir/credentials-personal-access-token.conf"

grep -Fq 'INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE=%d/github-app-private-key' "$app_credentials"
grep -Fq 'INCUS_GH_RUNNER_GITHUB_TOKEN_FILE=%d/github-token' "$pat_credentials"
grep -Fq 'LoadCredential=github-app-private-key:/etc/incus-gh-runner/github-app-private-key.pem' "$app_credentials"
grep -Fq 'LoadCredential=github-token:/etc/incus-gh-runner/github-token' "$pat_credentials"
if grep -Eq 'GITHUB_(APP_PRIVATE_KEY|TOKEN)' "$unit"; then
  printf 'base systemd unit must not select a GitHub credential method\n' >&2
  exit 1
fi

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
mkdir -p "$sandbox/etc/systemd/system/incus-gh-runner.service.d"
install -m 0644 "$app_credentials" "$sandbox/etc/systemd/system/incus-gh-runner.service.d/credentials.conf"
systemd-analyze verify --root="$sandbox" incus-gh-runner.service
install -m 0644 "$pat_credentials" "$sandbox/etc/systemd/system/incus-gh-runner.service.d/credentials.conf"
systemd-analyze verify --root="$sandbox" incus-gh-runner.service
# systemd expresses the displayed 0.0-10.0 exposure score as 0-100 here.
systemd-analyze security --offline=yes --threshold=50 "$unit" >/dev/null
printf 'systemd unit verification passed\n'

#!/usr/bin/env bash
set -euo pipefail

unit_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
unit="$unit_dir/incus-gh-runner.service"
app_credentials="$unit_dir/credentials-github-app.conf"
pat_credentials="$unit_dir/credentials-personal-access-token.conf"
proof_credentials="$unit_dir/credentials-job-proof-file.conf"
proof_tpm_credentials="$unit_dir/credentials-job-proof-tpm.conf"
tmpfiles="$unit_dir/incus-gh-runner.tmpfiles.conf"

installed_job_proof=""
[[ "$#" -le 1 ]] || {
  printf 'usage: %s [--installed-job-proof=file|--installed-job-proof=tpm]\n' "$0" >&2
  exit 2
}
case "${1:-}" in
  "") ;;
  --installed-job-proof=file)
    installed_job_proof="file"
    ;;
  --installed-job-proof=tpm)
    installed_job_proof="tpm"
    ;;
  *)
    printf 'usage: %s [--installed-job-proof=file|--installed-job-proof=tpm]\n' "$0" >&2
    exit 2
    ;;
esac

grep -Fq 'INCUS_GH_RUNNER_GITHUB_APP_PRIVATE_KEY_FILE=%d/github-app-private-key' "$app_credentials"
grep -Fq 'INCUS_GH_RUNNER_GITHUB_TOKEN_FILE=%d/github-token' "$pat_credentials"
grep -Fq 'LoadCredential=github-app-private-key:/etc/incus-gh-runner/github-app-private-key.pem' "$app_credentials"
grep -Fq 'LoadCredential=github-token:/etc/incus-gh-runner/github-token' "$pat_credentials"
grep -Fq 'INCUS_GH_RUNNER_JOB_PROOF_SIGNING_KEY_FILE=%d/machine-provenance-key' "$proof_credentials"
grep -Fq 'LoadCredential=machine-provenance-key:/etc/incus-gh-runner/machine-provenance-key.pem' "$proof_credentials"
grep -Fq 'INCUS_GH_RUNNER_JOB_PROOF_SIGNING_KEY_FILE=%d/machine-provenance-key' "$proof_tpm_credentials"
grep -Fq 'LoadCredentialEncrypted=machine-provenance-key:/etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred' "$proof_tpm_credentials"
grep -Fxq 'PrivateDevices=yes' "$unit"
grep -Fq 'e /var/log/incus-gh-runner/diagnostics 0700 - - 30d' "$tmpfiles"
if grep -Eq 'GITHUB_(APP_PRIVATE_KEY|TOKEN)|JOB_PROOF_SIGNING_KEY' "$unit"; then
  printf 'base systemd unit must not select a GitHub or proof credential method\n' >&2
  exit 1
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  printf 'systemd unit verification skipped outside Linux\n'
  exit 0
fi

command -v systemd-analyze >/dev/null
command -v systemd-tmpfiles >/dev/null

require_owner_mode() {
  local path="$1"
  local expected="$2"
  local actual

  if ! actual="$(stat -c '%U:%G %a' "$path")"; then
    printf 'required credential path is missing: %s\n' "$path" >&2
    exit 1
  fi
  if [[ "$actual" != "$expected" ]]; then
    printf 'credential path %s must be %s, found %s\n' "$path" "$expected" "$actual" >&2
    exit 1
  fi
}

verify_installed_job_proof() {
  local expected_dropin="$proof_credentials"
  local source_file="/etc/incus-gh-runner/machine-provenance-key.pem"

  [[ "$EUID" -eq 0 ]] || {
    printf 'installed credential verification must run as root\n' >&2
    exit 1
  }

  if [[ "$installed_job_proof" == "tpm" ]]; then
    expected_dropin="$proof_tpm_credentials"
    source_file="/etc/credstore.encrypted/incus-gh-runner-machine-provenance-key.cred"
    require_owner_mode /etc/credstore.encrypted "root:root 700"
  fi

  if ! cmp --silent "$expected_dropin" /etc/systemd/system/incus-gh-runner.service.d/job-proof.conf; then
    printf 'installed job-proof.conf does not match %s\n' "$expected_dropin" >&2
    exit 1
  fi
  require_owner_mode "$source_file" "root:root 600"
  if ! grep -Fxq 'PrivateDevices=yes' /etc/systemd/system/incus-gh-runner.service; then
    printf 'installed incus-gh-runner.service must retain PrivateDevices=yes\n' >&2
    exit 1
  fi
  systemd-analyze verify incus-gh-runner.service
  printf 'installed %s job proof credential verification passed\n' "$installed_job_proof"
}

if [[ -n "$installed_job_proof" ]]; then
  verify_installed_job_proof
  exit 0
fi

sandbox="$(mktemp -d)"
cleanup() {
  rm -rf -- "$sandbox"
}
trap cleanup EXIT

mkdir -p "$sandbox/usr/lib/systemd/system" "$sandbox/usr/lib/tmpfiles.d" "$sandbox/usr/bin" "$sandbox/etc"
cp -a /usr/lib/systemd/system/. "$sandbox/usr/lib/systemd/system/"
install -m 0644 "$unit" "$sandbox/usr/lib/systemd/system/incus-gh-runner.service"
install -m 0644 "$tmpfiles" "$sandbox/usr/lib/tmpfiles.d/incus-gh-runner.conf"
install -m 0755 /bin/true "$sandbox/usr/bin/incus-gh-runner"
printf 'root:x:0:\nincus-admin:x:999:\n' >"$sandbox/etc/group"

systemd-analyze verify --root="$sandbox" incus-gh-runner.service
systemd-tmpfiles --root="$sandbox" --create incus-gh-runner.conf
dropin_dir="$sandbox/etc/systemd/system/incus-gh-runner.service.d"
mkdir -p "$dropin_dir"

verify_combination() {
  local github_credentials="$1"
  local job_proof_credentials="$2"

  install -m 0644 "$github_credentials" "$dropin_dir/credentials.conf"
  install -m 0644 "$job_proof_credentials" "$dropin_dir/job-proof.conf"
  systemd-analyze verify --root="$sandbox" incus-gh-runner.service
}

for github_credentials in "$app_credentials" "$pat_credentials"; do
  for job_proof_credentials in "$proof_credentials" "$proof_tpm_credentials"; do
    verify_combination "$github_credentials" "$job_proof_credentials"
  done
done
# systemd expresses the displayed 0.0-10.0 exposure score as 0-100 here.
systemd-analyze security --offline=yes --threshold=50 "$unit" >/dev/null
printf 'systemd unit verification passed\n'

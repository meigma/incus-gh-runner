#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf 'usage: %s [output-directory]\n' "$0" >&2
}

if [[ "$#" -gt 1 ]]; then
  usage
  exit 2
fi

for command_name in gh git jq mise shasum; do
  command -v "$command_name" >/dev/null || {
    printf 'required command is unavailable: %s\n' "$command_name" >&2
    exit 1
  }
done

repo_root="$(git rev-parse --show-toplevel)"
output="${1:-${repo_root}/build/live-bundle}"
mkdir -p "$output"
if [[ -n "$(find "$output" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
  printf 'output directory must be empty: %s\n' "$output" >&2
  exit 1
fi

started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
gh workflow run reference-image.yml --ref master

run_id=''
for _ in $(seq 1 30); do
  runs="$(gh run list \
    --workflow reference-image.yml \
    --branch master \
    --event workflow_dispatch \
    --limit 10 \
    --json databaseId,createdAt)"
  run_id="$(jq --arg started_at "$started_at" --raw-output \
    '[.[] | select(.createdAt >= $started_at)] | sort_by(.createdAt) | last | .databaseId // empty' \
    <<<"$runs")"
  [[ -n "$run_id" ]] && break
  sleep 2
done
[[ -n "$run_id" ]] || {
  printf 'could not identify the dispatched reference-image workflow run\n' >&2
  exit 1
}

gh run watch "$run_id" --exit-status
head_sha="$(gh run view "$run_id" --json headSha --jq .headSha)"
image_directory="${output}/reference-image"
mkdir -p "$image_directory"
gh run download "$run_id" \
  --name "reference-image-${head_sha}" \
  --dir "$image_directory"

archive="$(find "$image_directory" -type f -name '*.tar.xz' -print -quit)"
checksum="${archive}.sha256"
[[ -n "$archive" && -f "$checksum" ]] || {
  printf 'downloaded reference-image artifact is incomplete\n' >&2
  exit 1
}
(
  cd "$(dirname "$archive")"
  shasum -a 256 -c "$(basename "$checksum")"
)

binary_directory="${output}/bin"
mkdir -p "$binary_directory"
mise exec -- env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -o "${binary_directory}/incus-gh-runner" ./cmd/incus-gh-runner
mise exec -- env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go test -c -o "${binary_directory}/incus-lifecycle.test" ./internal/adapters/incus

cp "${repo_root}/image/validate-incus.sh" "${output}/validate-incus.sh"
cp "${repo_root}/scripts/live/live-host-prepare.sh" "${output}/live-host-prepare.sh"
chmod 0755 \
  "${binary_directory}/incus-gh-runner" \
  "${binary_directory}/incus-lifecycle.test" \
  "${output}/validate-incus.sh" \
  "${output}/live-host-prepare.sh"

jq --null-input \
  --arg workflow_run_id "$run_id" \
  --arg head_sha "$head_sha" \
  --arg archive "$archive" \
  --arg controller "${binary_directory}/incus-gh-runner" \
  --arg lifecycle_test "${binary_directory}/incus-lifecycle.test" \
  '{
    workflow_run_id: $workflow_run_id,
    head_sha: $head_sha,
    archive: $archive,
    controller: $controller,
    lifecycle_test: $lifecycle_test
  }' >"${output}/manifest.json"

printf 'live-test bundle prepared at %s\n' "$output"

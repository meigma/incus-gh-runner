#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf 'usage: %s <owner/repository> <runner-label> <incus-project> <incus-owner> <controller> <config> [evidence-directory]\n' "$0" >&2
}

if [[ "$#" -lt 6 || "$#" -gt 7 ]]; then
  usage
  exit 2
fi

repository="$1"
runner_label="$2"
incus_project="$3"
incus_owner="$4"
controller="$5"
config="$6"
evidence_directory="${7:-hot-standby-evidence}"

[[ "$repository" =~ ^[^/]+/[^/]+$ ]] || {
  printf 'repository must use owner/name form: %s\n' "$repository" >&2
  exit 2
}
[[ -x "$controller" ]] || {
  printf 'controller is not executable: %s\n' "$controller" >&2
  exit 1
}
[[ -f "$config" ]] || {
  printf 'controller configuration is unavailable: %s\n' "$config" >&2
  exit 1
}
[[ -n "${INCUS_GH_RUNNER_GITHUB_TOKEN:-}" ]] || {
  printf 'INCUS_GH_RUNNER_GITHUB_TOKEN is required\n' >&2
  exit 1
}
for command_name in gh incus jq; do
  command -v "$command_name" >/dev/null || {
    printf 'required command is unavailable: %s\n' "$command_name" >&2
    exit 1
  }
done

mkdir -p "$evidence_directory"
if [[ -n "$(find "$evidence_directory" -mindepth 1 -maxdepth 1 -print -quit)" ]]; then
  printf 'evidence directory must be empty: %s\n' "$evidence_directory" >&2
  exit 1
fi

controller_log="${evidence_directory}/controller-hot-standby.jsonl"
idle_restart_log="${evidence_directory}/controller-idle-restart.jsonl"
busy_restart_log="${evidence_directory}/controller-busy-restart.jsonl"
workflow_log="${evidence_directory}/workflow-hot-standby.log"
cleanup_workflow_log="${evidence_directory}/workflow-cleanup.log"
initial_snapshot="${evidence_directory}/initial-runners.json"
busy_snapshot="${evidence_directory}/busy-runners.json"
replacement_snapshot="${evidence_directory}/replacement-runners.json"
final_snapshot="${evidence_directory}/final-runners.json"
idle_restart_snapshot="${evidence_directory}/idle-restart-runners.json"
busy_restart_snapshot="${evidence_directory}/busy-restart-runners.json"
cleanup_snapshot="${evidence_directory}/cleanup-runners.json"
controller_pid=''
observed_runner=''
observed_run_id=''

stop_controller() {
  if [[ -n "$controller_pid" ]] && kill -0 "$controller_pid" 2>/dev/null; then
    kill -TERM "$controller_pid"
    wait "$controller_pid" || true
  fi
}

stop_controller_checked() {
  kill -TERM "$controller_pid"
  if ! wait "$controller_pid"; then
    printf 'controller did not stop cleanly; inspect %s\n' "$controller_log" >&2
    exit 1
  fi
  controller_pid=''
}

interrupt_check() {
  exit 130
}

terminate_check() {
  exit 143
}

trap stop_controller EXIT
trap interrupt_check INT
trap terminate_check TERM

require_controller() {
  if ! kill -0 "$controller_pid" 2>/dev/null; then
    printf 'controller stopped before the check completed; inspect %s\n' "$controller_log" >&2
    exit 1
  fi
}

runner_snapshot() {
  incus --project "$incus_project" list --format json \
    | jq --arg owner "$incus_owner" \
      '[.[]
        | select(.config["user.incus-gh-runner.owner"] == $owner)
        | {name, status}]'
}

runner_is_ready() {
  local runner_name="$1"

  incus --project "$incus_project" exec "$runner_name" -- \
    journalctl -u incus-gh-runner-guest --no-pager 2>/dev/null \
    | grep -Fq 'Listening for Jobs'
}

wait_for_single_idle() {
  local output="$1"
  local expected_name="${2:-}"
  local snapshot
  local runner_name

  observed_runner=''
  for ((attempt = 1; attempt <= 180; attempt++)); do
    require_controller
    snapshot="$(runner_snapshot)"
    runner_name="$(jq -r \
      --arg expected "$expected_name" \
      'if length == 1 and
          .[0].status == "Running" and
          ($expected == "" or .[0].name == $expected)
       then .[0].name else empty end' <<<"$snapshot")"
    if [[ -n "$runner_name" ]] && runner_is_ready "$runner_name"; then
      printf '%s\n' "$snapshot" >"$output"
      observed_runner="$runner_name"
      return 0
    fi
    sleep 2
  done

  printf 'timed out waiting for one connected idle runner with label %s\n' "$runner_label" >&2
  return 1
}

wait_for_busy() {
  local runner_name="$1"
  local run_id="$2"
  local output="$3"
  local snapshot

  for ((attempt = 1; attempt <= 90; attempt++)); do
    require_controller
    snapshot="$(GH_TOKEN="$INCUS_GH_RUNNER_GITHUB_TOKEN" \
      gh api "repos/${repository}/actions/runs/${run_id}/jobs?per_page=100")"
    if jq -e --arg name "$runner_name" \
      'any(.jobs[]; .runner_name == $name and .status == "in_progress")' \
      <<<"$snapshot" >/dev/null; then
      printf '%s\n' "$snapshot" >"$output"
      return 0
    fi
    sleep 1
  done

  printf 'timed out waiting for runner %s to become busy\n' "$runner_name" >&2
  return 1
}

wait_for_replacement() {
  local consumed_name="$1"
  local output="$2"
  local snapshot
  local replacement_name

  observed_runner=''
  for ((attempt = 1; attempt <= 180; attempt++)); do
    require_controller
    snapshot="$(runner_snapshot)"
    replacement_name="$(jq -r --arg consumed "$consumed_name" \
      '[.[] | select(.name != $consumed and .status == "Running")]
       | if length == 1 then .[0].name else empty end' <<<"$snapshot")"
    if [[ -n "$replacement_name" ]] && runner_is_ready "$replacement_name"; then
      printf '%s\n' "$snapshot" >"$output"
      observed_runner="$replacement_name"
      return 0
    fi
    sleep 2
  done

  printf 'timed out waiting for an idle replacement for %s\n' "$consumed_name" >&2
  return 1
}

wait_for_deletion_log() {
  local runner_name="$1"

  for ((attempt = 1; attempt <= 180; attempt++)); do
    require_controller
    if jq -e --arg name "$runner_name" \
      'select(.msg == "owned Incus runner deleted" and .incus.runner_id == $name)' \
      "$controller_log" >/dev/null; then
      return 0
    fi
    sleep 2
  done

  printf 'timed out waiting for Incus deletion evidence for %s\n' "$runner_name" >&2
  return 1
}

wait_for_no_runners() {
  local output="$1"
  local snapshot

  for ((attempt = 1; attempt <= 180; attempt++)); do
    require_controller
    snapshot="$(runner_snapshot)"
    if [[ "$(jq 'length' <<<"$snapshot")" -eq 0 ]]; then
      printf '%s\n' "$snapshot" >"$output"
      return 0
    fi
    sleep 2
  done

  printf 'timed out waiting for scale-set runner cleanup\n' >&2
  return 1
}

dispatch_workflow() {
  local correlation_id="$1"
  local hold_seconds="$2"
  local dispatch_started
  local run_title
  local runs
  local run_id

  observed_run_id=''
  dispatch_started="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  GH_TOKEN="$INCUS_GH_RUNNER_GITHUB_TOKEN" gh workflow run runner-functional.yml \
    --repo "$repository" \
    --ref master \
    -f "runner_label=${runner_label}" \
    -f "correlation_id=${correlation_id}" \
    -f "hold_seconds=${hold_seconds}" \
    >/dev/null

  run_title="Runner functional check (${correlation_id})"
  run_id=''
  for ((attempt = 1; attempt <= 60; attempt++)); do
    runs="$(GH_TOKEN="$INCUS_GH_RUNNER_GITHUB_TOKEN" gh run list \
      --repo "$repository" \
      --workflow runner-functional.yml \
      --event workflow_dispatch \
      --limit 30 \
      --json databaseId,displayTitle,createdAt)"
    run_id="$(jq --arg title "$run_title" --arg started "$dispatch_started" -r \
      '[.[] | select(.displayTitle == $title and .createdAt >= $started)]
       | sort_by(.createdAt) | last | .databaseId // empty' <<<"$runs")"
    if [[ -n "$run_id" ]]; then
      observed_run_id="$run_id"
      return 0
    fi
    sleep 2
  done

  printf 'could not identify workflow run %s\n' "$run_title" >&2
  return 1
}

started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
check_id="hot-standby-$(date -u +%Y%m%dT%H%M%SZ)-$$"
GH_TOKEN="$INCUS_GH_RUNNER_GITHUB_TOKEN" gh auth status >/dev/null

"$controller" --config "$config" >"$controller_log" 2>&1 &
controller_pid="$!"

wait_for_single_idle "$initial_snapshot"
initial_runner="$observed_runner"
dispatch_workflow "$check_id" 30
run_id="$observed_run_id"

wait_for_busy "$initial_runner" "$run_id" "$busy_snapshot"
wait_for_replacement "$initial_runner" "$replacement_snapshot"
replacement_runner="$observed_runner"

GH_TOKEN="$INCUS_GH_RUNNER_GITHUB_TOKEN" gh run watch "$run_id" \
  --repo "$repository" \
  --exit-status \
  | tee "$workflow_log"

wait_for_single_idle "$final_snapshot" "$replacement_runner"
wait_for_deletion_log "$initial_runner"
stop_controller_checked

controller_log="$idle_restart_log"
"$controller" --config "$config" --min-runners 0 --max-runners 1 >"$controller_log" 2>&1 &
controller_pid="$!"
wait_for_single_idle "$idle_restart_snapshot" "$replacement_runner"

cleanup_check_id="${check_id}-cleanup"
dispatch_workflow "$cleanup_check_id" 30
cleanup_run_id="$observed_run_id"
wait_for_busy "$replacement_runner" "$cleanup_run_id" "$busy_restart_snapshot"
stop_controller_checked

controller_log="$busy_restart_log"
"$controller" --config "$config" --min-runners 0 --max-runners 1 >"$controller_log" 2>&1 &
controller_pid="$!"

GH_TOKEN="$INCUS_GH_RUNNER_GITHUB_TOKEN" gh run watch "$cleanup_run_id" \
  --repo "$repository" \
  --exit-status \
  | tee "$cleanup_workflow_log"

wait_for_no_runners "$cleanup_snapshot"
wait_for_deletion_log "$replacement_runner"
stop_controller_checked

completed_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
jq --null-input \
  --arg check_id "$check_id" \
  --arg repository "$repository" \
  --arg runner_label "$runner_label" \
  --arg initial_runner "$initial_runner" \
  --arg replacement_runner "$replacement_runner" \
  --arg run_id "$run_id" \
  --arg cleanup_run_id "$cleanup_run_id" \
  --arg started_at "$started_at" \
  --arg completed_at "$completed_at" \
  '{
    check_id: $check_id,
    repository: $repository,
    runner_label: $runner_label,
    initial_runner: $initial_runner,
    replacement_runner: $replacement_runner,
    workflow_run_id: ($run_id | tonumber),
    cleanup_workflow_run_id: ($cleanup_run_id | tonumber),
    controller_restarts: 2,
    started_at: $started_at,
    completed_at: $completed_at
  }' >"${evidence_directory}/manifest.json"

printf 'hot-standby check passed; evidence: %s\n' "$evidence_directory"

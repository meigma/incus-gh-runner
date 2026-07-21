---
id: 022
title: Start new work session
started: 2026-07-21
---

## 2026-07-21 15:42 — Kickoff
Goal for the session: the user requested a fresh session; the substantive work
request has not been stated yet.
Current state of the world: `master` is at `d190e3a` (docs pages always deploy
from the default branch). Job machine-proof phases 1–5 are complete through
PR #41, including TPM-bound proof-key storage proven on `sre@ci`. Release
Please PR #24 for `1.0.0` remains open with no tag published. Open threads
include cross-host TPM decrypt evidence (optional), sessions 007/012/013
(package hosting proposal, GitHub App/PAT auth support, SLSA remediation plan),
and the undecided public shipping of the agent-session infrastructure.
Plan: await the user's actual request, then journal work here.

## 2026-07-21 15:44 — Bash script debt survey
Goal restated: identify test/validation bash scripts that landed as permanent
repo fixtures and represent technical debt.
Findings (investigation only, no changes):
- 10 tracked `.sh` files plus 2 extensionless guest scripts; none ever deleted.
- Product code (not debt): `image/guest/incus-gh-runner-{guest,proof}`,
  `image/build.sh`, `image/validate-incus.sh` (docs/release-wired).
- CI-wired bash test frameworks (moon tasks): `deploy/systemd/verify.sh`,
  `deploy/incus/cue/tests/render-test.sh` (asserts exact CUE error text),
  `image/tests/guest-entrypoint-test.sh` (partly grep-the-repo assertions),
  `image/tests/live-incus-hostile-isolation-contract-test.sh` (304 lines that
  grep/sed the *source text* of the 808-line live harness — worst offender).
- Live manual harnesses under `scripts/live/`: `live-hot-standby.sh` (342
  lines, referenced nowhere), `live-bundle-prepare.sh` and
  `live-host-prepare.sh` (only self-references), and
  `live-incus-hostile-isolation.sh` (808 lines, referenced once in
  deploy.md as an operator step).
- Adjacent habit: large inline bash in `release.yml` (581 lines, six 17–35
  line blocks) and `release-dry-run.yml` (one 46-line block).
Next: user to decide remediation scope.

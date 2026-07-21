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

## 2026-07-21 16:05 — Removal implemented, PR #44 open
The maintainer directed removal of everything except product code
(`image/guest/*`, `image/build.sh`, `image/validate-incus.sh`), explicitly
declining long-term maintenance of the harnesses; the CUE test-case file
`deploy/incus/cue/tests/cases.cue` goes too per their answer.
Done on branch `chore/remove-bash-test-debt` (worktree
`.wt/chore-remove-bash-test-debt`, commit `fa64f2e`, 13 files, -2,288 lines):
- Deleted `scripts/live/` (4 harnesses), `image/tests/` (2 contract tests),
  `deploy/systemd/verify.sh`, `deploy/incus/cue/tests/`.
- `moon.yml`: removed tasks `image-contract-test`,
  `incus-isolation-contract-test`, `systemd-unit-test`, their `check` deps,
  and file groups `imageSources`/`systemdSources`/`incusIsolationSources`.
- Docs: deploy.md lost the hostile-harness preflight block, both
  `verify.sh --installed-job-proof` steps, and old step 7 "Validate the
  unit" (step 8 renumbered to 7); README.md and deploy/incus/cue/README.md
  harness mentions removed.
Verified: repo-wide grep for removed basenames returns nothing;
`moon run root:check` passes (graph resolves, strict docs build green);
`git ls-files .journal` empty on the branch.
Coverage intentionally dropped without replacement, recorded in PR #44's
body. Next: hosted CI on PR #44, then maintainer squash-merge.

## 2026-07-21 16:22 — Close
PR #44 was approved by the maintainer, squash-merged as `635097d`, and the
`chore/remove-bash-test-debt` branch and worktree were removed; local
`master` fast-forwarded to the merge commit. Wrote `SUMMARY.md`, marked the
INDEX row complete, and updated `TECH_NOTES.md`: the stale live-harness
pointer is gone and a durable note records the removal decision and the
kept product scripts. No open work remains for this session.

---
id: 011
title: New work session
started: 2026-07-18
---

## 2026-07-18 16:01 — Kickoff
Goal for the session: not yet stated; the user opened a new session and has not
made their first request.
Current state of the world: v1 phases 0 through 7 are complete at `master`
commit `82f5ed4`. Release Please is enabled with the dedicated GitHub App, but
the generated `1.0.0` release PR (#22) was closed unmerged at the maintainer's
direction, so no tag or release exists. GitHub Pages stays gated off on the
current plan. Optional live phase 5 gates (bounded concurrent demand, timed
provisioning/terminal-cleanup restarts) remain open.
Plan: wait for the user's request, then scope the work and update this log.

## 2026-07-18 16:15 — Release-language sweep complete
Goal set: remove leftover build-process language (phases, sessions, TODOs,
slices, "in progress") from the repository before the first release.
Ran workflow `release-language-sweep` (run wf_0f6e83ec-1be): 8 Sonnet 5
scanners over disjoint product-facing file groups, then 8 Opus 4.8 adversarial
verifiers. 16 agents, ~769k tokens. Results: 85 confirmed leftover findings,
13 verifier additions, 3 uncertain (.gitignore agent-tooling entries), 6
scanner findings ruled legitimate (GitHub message sessions, systemd terms).
Full structured findings saved at `.journal/011/language-sweep-findings.json`.
Heaviest files: README.md (27), docs/docs/index.md (14), the three
scripts/live/phase*-*.sh harnesses (15, filenames included), runner-functional
workflow (6), Go test fixtures using phase-numbered names (12). Coupled
renames to watch: phase4-prepare.sh copies phase4-host-prepare.sh; the
phase5 harness run_title must stay byte-identical to runner-functional.yml
run-name; validate-incus.sh phase2-probe prefix must match its heredoc glob.
Agent-protocol infra (.session.md, .agents/skills, scaffold/.journal,
CLAUDE.md, AGENTS.md) deliberately excluded; flagged separately to the user.
Next: await user decision on applying fixes (and on whether agent infra ships).

## 2026-07-18 17:10 — Operator docs overhaul (PR #23)
Approved plan: delete the three phase-narrative docs pages and replace with a
7-page operator Diátaxis set (index; how-to deploy/operate/runner-images;
reference configuration/guest-contract; explanation how-it-works), MkDocs
infra kept. Executed on branch `docs-operator-overhaul` (worktree .wt/).
Workflow `operator-docs-write` (run wf_eed52023-f5a): 7 Sonnet 5 writers →
7 Opus 4.8 verifiers → 1 Opus 4.8 coherence editor; 15 agents, ~695k tokens.
I grounded writers with an inventory extracted by an Explore agent from
config.go, the unit file, controller/adapters, and image/guest sources.
Post-workflow fixes by hand: ungrounded "systemd 250" claim reworded to
capability-based prereq; `owned Incus runner started` log-event meaning
corrected (fires at create completion, still provisioning). Spot-verified
config table, unit directives, guest script messages, log event fields, and
script output strings against source — all match. Strict docs build passes;
process-language grep clean. README links and unit Documentation= URL updated
to new paths. PR #23 opened; CI monitor running.
Still pending from earlier: applying the 98-finding language sweep (separate
task, user decision on agent-infra shipping still open).

## 2026-07-18 17:15 — Docs merged; language cleanup PR open
Docs PR #23 squash-merged at master 5467b99; worktree removed. Applied the
language sweep on branch `cleanup-process-language` (77 findings; docs/
findings mooted by #23): README intro/live-test rewrite and example renames,
scripts/live renamed (live-bundle-prepare.sh, live-host-prepare.sh,
live-hot-standby.sh), runner-functional.yml retitled "Runner Functional
Check" with proof_id→correlation_id coupled to the harness run title,
validate-incus.sh validate-probe- prefix, Go fixture/comment renames, small
config-file wording. Uncertain .gitignore agent-tooling entries kept under a
neutral heading pending the user's agent-infra-shipping decision. Verified:
moon run root:check all green, .github Python tests OK, bash -n on scripts,
residue grep clean. PR #25 open, CI monitored.
Note: Release Please regenerated release PR #24 (1.0.0) after the docs
commit reached master — expected while the gate is enabled; merging it is
the maintainer's call.

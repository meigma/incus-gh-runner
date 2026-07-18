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

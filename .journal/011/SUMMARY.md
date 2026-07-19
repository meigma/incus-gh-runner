---
id: 011
title: Pre-release language cleanup, docs overhaul, and licensing
date: 2026-07-18
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 009, 010]
---

## Goal

Make the repository presentable for its first public release: find and remove
leftover development-process language (phases, sessions, TODOs, slices, proof
narrative), replace the antiquated docs with an operator-focused Diátaxis set,
rewrite the README, and dual-license the project.

## Outcome

The goal was met. PR #23 replaced the three phase-narrative docs pages with a
seven-page operator documentation set, and PR #25 removed all confirmed
process-language residue from product-facing files, rewrote the README, and
added the Apache-2.0/MIT dual license. Both merged clean; `master` finished at
`fd7e72c`. Release Please regenerated release PR #24 (`1.0.0`) against the
cleaned tree; publishing remains an explicit maintainer decision.

## Key Decisions

- Run both the language sweep and the docs authoring as multi-agent workflows restricted to Sonnet 5 writers/scanners with Opus 4.8 adversarial verifiers -> the user required those models, and the verify stage killed six false positives (GitHub "message sessions", systemd terms are legitimate domain language) and caught thirteen scanner misses.
- Ground doc writers in a source-extracted inventory and forbid unverifiable claims -> the two defects that still slipped through (an invented "systemd 250" requirement, a mischaracterized log event) were caught by manual spot-checks against source; agent-written docs need a human-in-the-loop accuracy pass.
- Exclude the agent-session infrastructure (`.session.md`, `.agents/`, `scaffold/`, `CLAUDE.md`, `AGENTS.md`) from the sweep -> it is deliberate tooling, not leftover language; whether it ships publicly is a separate maintainer decision, still open.
- Prefer fewer, single-type Diátaxis documents (3 how-to, 2 reference, 1 explanation, 1 landing) with a strict de-duplication contract -> the user asked for fewer high-quality documents; each fact now lives on exactly one page and other pages link to it.
- Keep coupled renames in lockstep in one commit -> the hot-standby harness's matched run title, the workflow's `correlation_id` input, the validate-incus probe-prefix glob, and the bundle-prepare script copies would each break silently if renamed independently.
- Dual-license as `Apache-2.0 OR MIT` with the SPDX expression in melange/apko metadata -> standard at-your-option dual licensing with the packaging metadata kept consistent from the first release.

## Changes

- `docs/docs/` - three phase-narrative pages deleted; new set: `index.md`, `how-to/{deploy,operate,runner-images}.md`, `reference/{configuration,guest-contract}.md`, `explanation/how-it-works.md`; `mkdocs.yml` nav rebuilt.
- `README.md` - rewritten (description, features, requirements, installation, usage, documentation map, development, license); operational narrative now links to docs instead of duplicating them.
- `scripts/live/` - renamed to `live-bundle-prepare.sh`, `live-host-prepare.sh`, `live-hot-standby.sh`; phase-numbered paths, variables, and messages neutralized.
- `.github/workflows/runner-functional.yml` - retitled `Runner Functional Check`; `proof_id` input renamed `correlation_id` in lockstep with the harness.
- `image/validate-incus.sh` - probe prefix `phase2-probe-` -> `validate-probe-`.
- `internal/` - "phase 1" godoc comments and phase-numbered test fixtures renamed across config, CLI, and adapter tests.
- `LICENSE-APACHE`, `LICENSE-MIT` - added; SPDX `Apache-2.0 OR MIT` recorded in `melange.yaml` copyright and `apko.yaml` OCI annotations.
- `deploy/systemd/incus-gh-runner.service` - `Documentation=` URL updated to the new docs path.
- Small wording fixes in `CONTRIBUTING.md`, `.gitignore`, `.moon/toolchains.yml`, `mise.toml`, `.golangci.yml`, `.github/repository-settings.toml`, `.github/scripts/configure_github_repo.py`, `.github/workflows/reference-image.yml`.

## Open Threads

- Release PR #24 (`1.0.0`) is open and refreshed against the cleaned tree; merging it publishes the first release and remains the maintainer's call.
- Whether the agent-session infrastructure (`.session.md`, `.agents/`, `scaffold/.journal/`, `CLAUDE.md`, `AGENTS.md`) ships in the public repository is undecided; the `.gitignore` entries for it were kept under a neutral heading.
- Optional live acceptance gaps carried from session 008 remain: bounded concurrent demand and deliberately timed provisioning/terminal-cleanup restarts.

## Lessons

- Adversarial verification earns its cost on language sweeps: domain terms ("message session", systemd concepts) look identical to process residue at grep level and need context-aware judgment to keep.
- Workflow-written documentation still requires manual accuracy spot-checks against source; both residual defects were plausible-sounding claims no agent had grounded.
- Renamed workflow inputs and harness-matched run titles are byte-coupled interfaces; treat them like API changes, not text edits.

## References

- [PR #23: docs: replace development docs with operator documentation](https://github.com/meigma/incus-gh-runner/pull/23)
- [PR #25: chore: remove development-process language before first release](https://github.com/meigma/incus-gh-runner/pull/25)
- [PR #24: chore(master): release 1.0.0 (open, maintainer decision)](https://github.com/meigma/incus-gh-runner/pull/24)
- `.journal/011/language-sweep-findings.json` - full structured sweep findings
- `.journal/010/SUMMARY.md` - release-readiness context this session built on

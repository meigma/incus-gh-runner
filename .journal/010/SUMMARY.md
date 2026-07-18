---
id: 010
title: Continue phase 7 release readiness
date: 2026-07-18
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001, 009]
---

## Goal
Continue phase 7 by making the controller and reference VM publishable, completing operator-facing release documentation, and proving the release paths against hosted infrastructure.

## Outcome
The release-readiness goal was met. PRs #20 and #21 landed the reference-image release path, faithful hosted rehearsals, documentation, and safe repository-service gates. The Release Please GitHub App was configured and successfully generated a fully green `1.0.0` release PR, proving the automation end to end. At the maintainer's direction, PR #22 was then closed unmerged and its branch deleted, so no tag or release was created.

## Key Decisions
- Publish the Incus reference VM beside controller artifacts -> it was the largest remaining gap between CI proof and an operator-consumable release.
- Rehearse all artifacts on release PRs -> native packages, the apko image, GoReleaser binaries, and the reference VM now fail before a release tag is created.
- Gate unavailable repository services with variables -> private-repository Pages is unsupported by the current GitHub plan, while documentation must still build and Release Please must fail closed when credentials are absent.
- Use the dedicated GitHub App credential for Release Please -> the private key was streamed from 1Password directly into the Actions secret without local persistence.
- Do not publish `v1.0.0` -> the maintainer explicitly declined making a release; the generated release PR was abandoned after its readiness proof passed.

## Changes
- `.github/workflows/release.yml` - builds, versions, checksums, uploads, and attests the Incus reference VM with the controller release artifacts.
- `.github/workflows/release-dry-run.yml` - rehearses binaries, native packages, the controller OCI image, and reference VM on release PRs.
- `.github/scripts/stage_reference_image_release.py` and tests - stages deterministic versioned VM assets and validates checksums.
- `.github/workflows/release-please.yml` - authenticates through the dedicated GitHub App and supports an explicit repository enablement gate.
- `.github/workflows/docs-pages.yml` - always verifies documentation while skipping unsupported Pages deployment.
- `README.md`, `docs/`, and `.github/repository-settings.toml` - document artifact verification, local operator guidance, required context, and repository-plan constraints.
- Repository configuration - installed `MEIGMA_RELEASE_APP_PRIVATE_KEY`, retained the public App client ID, enabled Release Please, and kept Pages disabled.

## Open Threads
- No `v1.0.0` tag or release exists. Release Please remains enabled and may generate a new release PR after future conventional commits reach `master`; publishing still requires merging that PR.
- GitHub Pages remains disabled while this private repository is on a plan that does not support it; repository-local documentation remains available and continuously verified.
- Bounded concurrent-demand proof and deliberately timed provisioning or terminal-cleanup restart proof remain optional live acceptance gaps carried from phase 5.

## References
- [PR #20: publish reference VM image](https://github.com/meigma/incus-gh-runner/pull/20)
- [PR #21: gate unavailable repository services](https://github.com/meigma/incus-gh-runner/pull/21)
- [PR #22: generated release 1.0.0, closed unmerged](https://github.com/meigma/incus-gh-runner/pull/22)
- [Exact-head release rehearsal run 29662550423](https://github.com/meigma/incus-gh-runner/actions/runs/29662550423)
- [Release Please authentication run 29663419405](https://github.com/meigma/incus-gh-runner/actions/runs/29663419405)
- [Release PR artifact rehearsal run 29663426918](https://github.com/meigma/incus-gh-runner/actions/runs/29663426918)
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`

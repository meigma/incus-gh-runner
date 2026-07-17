---
id: 002
title: Establish repository foundation
date: 2026-07-17
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [001]
---

## Goal

Continue from session 001's v1 design artifacts and execute phase 0 of the implementation plan: turn the untouched Go template into the real `incus-gh-runner` repository foundation, prove its standard gates, and land it through review.

## Outcome

The goal was met. PR [#7](https://github.com/meigma/incus-gh-runner/pull/7) was squash-merged to `master` as `468c0a9`, the main checkout was fast-forwarded, and the implementation worktree plus local and remote feature branches were removed.

The first PR head pinned Incus `v7.0.0`, but Kusari Inspector identified nine unmitigated critical/high-severity CVEs, including multiple CVSS 9.9 host file-access paths. The branch was updated to Incus `v7.2.0`; local verification and the refreshed GitHub, Pages, and Kusari checks all passed before merge.

## Key Decisions

- Rename the complete repository surface, not only the Go module, so CI, release assets, container metadata, docs, environment variables, and helper fixtures cannot publish under the template identity.
- Keep the GitHub and Incus integration packages as minimal adapter constructors. Controller-owned ports wait for phase 1 behavior to reveal the useful contracts instead of freezing speculative interfaces.
- Pin `github.com/actions/scaleset v0.4.0` and `github.com/lxc/incus/v7 v7.2.0`; the security check made `v7.0.0` unacceptable even though it was the LTS release found during initial research.
- Reset Release Please, melange, and apko version state to `0.0.0` because this repository has no releases or tags and must not inherit the template's changelog/version history.
- Document disposable Incus and GitHub functional-test boundaries without provisioning or specifying live infrastructure before the relevant lifecycle slices exist.

## Changes

- `go.mod`, `go.sum` - renamed the module and pinned the scale-set, Incus, and Testify dependencies.
- `cmd/incus-gh-runner`, `internal/cli`, `internal/config`, `internal/projectinfo` - renamed the executable and user-facing CLI/config identity while preserving the signal-aware Cobra/Viper baseline.
- `internal/adapters/github`, `internal/adapters/incus` - added the first third-party construction seams without importing them into controller business logic.
- `moon.yml`, `.golangci.yml`, `mise.toml` - renamed build outputs and project metadata while keeping mise as the locked tool provider and Moon as the task gate.
- `.goreleaser.yaml`, `ghd.toml`, `melange.yaml`, `apko.yaml`, release workflows and helper fixtures - aligned binary, apk, container, provenance, and release-asset names.
- `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `docs/` - replaced template onboarding with project scope, development commands, security policy, and functional-test boundaries.
- `CHANGELOG.md`, `.release-please-manifest.json`, `DELETE_ME.md` - removed inherited template history/onboarding and reset initial release state.

## Open Threads

- Begin phase 1 with the smallest fake-demand reconciliation proof: coalesced demand, a single-owner reconciler, bounded workers, and signal-aware supervision.
- The renamed packaging path has not yet been exercised by a release dry run or actual release; that remains part of the later packaging phase.
- The repository still needs a project license before public release.
- `.github/repository-settings.toml` now describes the non-template repository, but this session did not apply the manifest to live GitHub settings.

## Lessons

- A release being current or LTS does not make it safe to pin without the repository's dependency scanner. Hosted security analysis caught severe Incus `v7.0.0` advisories that the initial version lookup missed.
- Worktrunk correctly recognized and removed the squash-merged branch by tree equivalence. `gh pr merge --delete-branch` merged successfully but could not delete the branch while its Worktrunk worktree still existed, so remote merge state had to be verified before separate cleanup.

## References

- [PR #7: chore: establish repository foundation](https://github.com/meigma/incus-gh-runner/pull/7)
- [Incus v7.2.0 release](https://github.com/lxc/incus/releases/tag/v7.2.0)
- `.journal/001/SUMMARY.md`
- `.journal/001/V1_IMPLEMENTATION_PLAN.md`
- `.journal/001/CONTROLLER_PROPOSAL.md`
- `.journal/001/IMAGE_PROPOSAL.md`

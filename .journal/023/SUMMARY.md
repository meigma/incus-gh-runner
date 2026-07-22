---
id: 023
title: Remove the reference image surface
date: 2026-07-21
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [004, 016, 022]
---

## Goal

Per explicit maintainer decision, stop maintaining the example runner image:
remove everything that references, builds, or validates the custom reference
image or the distrobuilder config, and replace it with a single Diátaxis-style
page exporting the hardening lessons for operators building their own images.

## Outcome

The goal was met. PR #45 squash-merged as `master` commit `0b7a4b6`
(24 files, +238/−1220). The survey found zero Go-code references to the image
surface, so the removal was pure tooling, CI, and docs. The guest-side
contract implementation was kept as product code and moved from `image/guest/`
to top-level `guest/`. The new hardening guide landed at
`docs/docs/how-to/build-runner-images.md`. `moon run root:check` and all
hosted checks passed; the required `ci` check was green at merge.

## Key Decisions

- Keep `image/guest/*` (moved to `guest/`) -> it is the only shipped
  implementation of the guest half of the controller contract, not image
  automation; `guest-contract.md` now documents it as the canonical
  implementation custom images may install verbatim.
- Remove `image/validate-incus.sh` despite it validating *custom* images ->
  consistent with the session 022 no-bash-harness decision; the new guide's
  boot-test section describes the manual equivalent.
- Publish the replacement as a how-to (`build-runner-images.md`), not an
  explanation page -> the content is task-shaped: required wiring steps plus
  a hardening baseline, with schemas linked from `guest-contract.md`.
- Edit the settings manifest but do not run `configure_github_repo.py apply`
  -> the live `Default` ruleset (id 19156537) only requires `ci`, so the
  removed `Reference Image Dry Run` context never blocked merge, and `plan`
  showed broad pre-existing drift (it would create both managed rulesets);
  applying is a separate deliberate maintainer action.

## Changes

- Deleted `image/image.yaml`, `image/build.sh`, `image/validate-incus.sh`,
  `.github/workflows/reference-image.yml`,
  `.github/scripts/stage_reference_image_release.py` plus its unittest, and
  `docs/docs/how-to/runner-images.md`.
- `.github/workflows/release.yml` - removed the `reference-image-release` and
  `attest-reference-image` jobs and the reference-image inspection-summary
  block; `release-dry-run.yml` - removed the `reference-image-dry-run` job.
- `mise.toml`/`mise.lock` - removed the `http:distrobuilder` source-build pin;
  `mise lock` re-run pruned exactly that entry.
- `guest/` - relocated the five guest contract files unchanged.
- `docs/docs/how-to/build-runner-images.md` - new hardening guide (minimal
  package base, Incus agent wiring, pinned runner install under a `nologin`
  user, guest component install table, serial console, machine-id reset,
  fail-closed disk growth, signed boot chain, provenance recording, manual
  guest-contract boot test).
- `README.md`, `docs/docs/index.md`, `how-to/deploy.md`, `how-to/operate.md`,
  `reference/guest-contract.md`, `docs/mkdocs.yml` - rewired every reference;
  `guest-contract.md` gained a "Shipped guest components" section replacing
  the reference-image table.
- `.github/repository-settings.toml` - dropped the `Reference Image Dry Run`
  required-check context.

## Open Threads

- Live GitHub repo settings have drifted from
  `.github/repository-settings.toml` well beyond this change:
  `configure_github_repo.py plan` would create both managed rulesets and
  update general settings. Running `apply` is left as a deliberate
  maintainer decision.
- No scripted guest-contract conformance check exists anymore; operators
  follow the manual boot test in `build-runner-images.md`. Reintroducing a
  validator (likely as Go code) would be a new maintainer decision.

## References

- [PR #45](https://github.com/meigma/incus-gh-runner/pull/45), merge commit
  `0b7a4b6f7d6dbb3114f34023254cd36a9c10d242`.
- Survey details and scope Q&A: `.journal/023/NOTES.md`.
- `.journal/004/SUMMARY.md` - the session that created the reference image;
  `.journal/016/SUMMARY.md` - bootc rejection; `.journal/022/SUMMARY.md` -
  the bash test-script removal precedent.

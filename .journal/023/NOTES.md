---
id: 023
title: Start new work session
started: 2026-07-21
---

## 2026-07-21 16:22 — Kickoff
Goal for the session: the user opened a fresh session without a substantive
work request yet; awaiting their actual goal.
Current state of the world: `master` at `635097d` (PR #44, bash test-script
removal) with a clean tree. Job machine-proof phases 1–5 are complete through
PR #41; release `1.0.0` PR #24 remains open and unmerged. Sessions 007, 012,
013, and 021 remain in-progress. Remaining checks are the Go tests,
release-config tests, and strict docs build.
Plan: wait for the user's request, then load task-relevant skills and record
progress here.

## 2026-07-21 16:30 — Reference-image removal survey
Goal restated: drop the maintained reference image and distrobuilder surface
entirely; replace with one Diátaxis page exporting the hardening lessons for
people building their own images.
Survey results (grep across repo, zero Go-code references to any of it):
- `image/`: image.yaml (distrobuilder config), build.sh, validate-incus.sh,
  guest/ (entrypoint, proof helper, .service, .path, tmpfiles conf).
- Workflows: reference-image.yml (whole file); release.yml jobs
  `reference-image-release` + `attest-reference-image` + draft-release notes
  block; release-dry-run.yml job `reference-image-dry-run`.
- `.github/scripts/stage_reference_image_release.py` + its pytest.
- mise.toml `http:distrobuilder` tool block + mise.lock entry.
- Docs: how-to/runner-images.md (delete/replace); cross-refs in index.md,
  deploy.md, operate.md, guest-contract.md, job-proofs.md, README, mkdocs nav.
- Untouched: melange/apko controller image, attest.yml (shared reusable,
  still used by container attestation), controller Go code, `incus.image`
  config key (generic).
Open scope questions for the maintainer: (1) do `image/guest/*` scripts go
too (they are the only shipped guest-side implementation of the contract);
(2) does validate-incus.sh go (it validates *custom* images, not just the
reference one); (3) Diátaxis type/placement of the new hardening page.

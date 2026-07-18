---
id: 010
title: Continue phase 7 release readiness
started: 2026-07-18
---

## 2026-07-18 14:36 — Kickoff
Goal for the session: Continue the v1 implementation plan with phase 7 release readiness.
Current state of the world: Session 001 established the controller, image, and phased v1 design; phases 0 through 6 have since landed through PRs #7 through #19, with `master` clean at `4979f7d` and live evidence covering the genuine runner lifecycle, hot standby recovery, and service hardening. Phase 7 now owns publishable controller and reference-image artifacts, complete operator documentation, and consolidated end-to-end acceptance evidence; bounded concurrent demand and deliberately timed provisioning or terminal-cleanup restarts remain optional phase 5 proof gaps.
Plan: Inspect the current release and documentation surfaces, choose the smallest phase 7 proof that reduces the most uncertainty, implement and validate it, then refine the remaining release work from observed behavior.

## 2026-07-18 14:50 — Reference-image release slice implemented locally
Inspection found that Release Please, GoReleaser binary assets, and the melange/apko controller OCI image path already existed, while the Incus reference VM was retained only as a one-day CI proof artifact and no GitHub release had yet been created. Chose reference-image publication as the smallest phase 7 uncertainty-reducing slice.
Implemented versioned reference-image staging with checksum validation and deterministic tests; extended the tag release to build, inspect, upload, and attest the VM archive; added a faithful release-PR dry run; wired all release script tests into `moon run root:check`; and documented download, checksum, provenance, and boot verification. The local aggregate gate passed with 13 tasks, including 17 release-script tests and the strict documentation build.
Next: Review and commit the bounded slice, publish it as a PR, dispatch the hosted release rehearsal on the exact branch head, and revise any workflow assumptions exposed by the real Ubuntu build.

## 2026-07-18 15:13 — First phase 7 slices landed and live blockers isolated
PR #20 (`feat(release): publish reference VM image`) landed at `1943892`. Exact-head release rehearsal run `29662550423` passed GoReleaser binaries, native amd64/arm64 Melange packages, apko controller-image assembly, and the new versioned reference VM construction/checksum/qcow2 inspection. The first rehearsal exposed a stale template `--message` OCI smoke test; replacing it with `--help` made the rerun green. The ordinary reference-image proof also passed on the reviewed head.
The merge then exposed repository enablement failures rather than artifact failures. Release Please had no App identifier, and Pages was unavailable for this private repository. The existing `meigma-release-please` App has public client ID `Iv23liZp61F0vaCah14R`, but branch run `29662957051` proved that no `MEIGMA_RELEASE_APP_PRIVATE_KEY` secret is available. GitHub rejected Pages enablement with HTTP 422 because the current plan does not support private-repository Pages.
PR #21 (`fix(ci): gate unavailable repository services`) landed at `82f5ed4`. Repository variables now set `MEIGMA_RELEASE_APP_CLIENT_ID`, `RELEASE_ENABLED=false`, and `PAGES_ENABLED=false`; docs continue to build while deployment is skipped, broken Pages links now target repository-local docs, and Release Please is explicitly disabled until its private key exists. Exact-head Pages dispatch `29662998666` passed with deployment intentionally skipped, and Release Please dispatch `29662998035` skipped cleanly.
Next: An App administrator must generate/install `MEIGMA_RELEASE_APP_PRIVATE_KEY`, then set `RELEASE_ENABLED=true`. After that, rerun Release Please to create the first release PR and exercise the tagged draft-release workflow. Public/private Pages publication remains blocked on a GitHub plan or repository visibility change; it does not block repository-local operator docs or release artifacts.

## 2026-07-18 15:27 — Release Please enabled with the Homelab App key
Retrieved the `key.pem` attachment from the `meigma-release-please` item in the 1Password `Homelab` Vault and streamed it directly into the repository's `MEIGMA_RELEASE_APP_PRIVATE_KEY` Actions secret; the key was neither displayed nor written to disk. Set `RELEASE_ENABLED=true` while retaining the existing public App client ID.
Manual Release Please run `29663419405` succeeded on exact `master` commit `82f5ed4`, including App-token creation, and opened PR #22 (`chore(master): release 1.0.0`) at head `4bfd25b`. The generated PR updates the manifest, changelog, melange package version, and apko package pin. Its release dry-run and ordinary PR checks are now the approval gate; merging it would create the first tag and start publication, so no merge has been performed.
All PR #22 checks passed on exact head `4bfd25b`: CI, documentation build, Kusari Inspector, GoReleaser binary rehearsal, native amd64/arm64 Melange builds, apko container assembly, and the 9m59s Incus reference-image build and inspection. PR #22 is ready for human release approval and remains deliberately unmerged.

## 2026-07-18 15:59 — Close
Phase 7 release readiness is complete through merged PRs #20 and #21. The dedicated Release Please App credential was installed and the generated `1.0.0` PR #22 passed every hosted gate, but the maintainer chose not to make a release; PR #22 was closed unmerged and its branch deleted, leaving no tag or published release. Local `master` is clean at `82f5ed4`, repository-local documentation remains verified, Pages deployment remains gated by the private-repository plan constraint, and the optional phase 5 live edge proofs remain available for future work.

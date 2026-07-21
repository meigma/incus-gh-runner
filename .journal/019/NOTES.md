---
id: 019
title: Implement job machine proof phase 4
started: 2026-07-20
---

## 2026-07-20 17:57 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and implementation plan, then begin Phase 4.
Current state of the world: Phases 1 through 3 are merged on `master` at `c32e134`; the repository can correlate an authenticated GitHub job with an exact JIT runner and owned Incus VM, sign its reconstructed machine snapshot, and deliver the proof through the immutable guest channel. Phase 4's genuine GitHub/Incus consumption proof and bounded evidence remain open, while TPM-bound credential validation remains deferred to Phase 5.
Plan: Re-read Session 014's Phase 4 acceptance contract, inspect the merged implementation and live harnesses, then implement the smallest proof-driven Phase 4 slice and validate it before expanding.

## 2026-07-20 18:07 — Phase 4 enabling slice ready
Reviewed Session 014's complete design and implementation plan against merged `master` at `c32e134`. The design remains internally consistent: Phase 4 is an operational proof gate around the merged claim, not authorization to add artifact provenance, TPM behavior, persistence, or a guest request API. The first incomplete dependency was a default-branch workflow surface and deployable file-backed systemd credential.

Created `feat/job-proof-phase-4-live` from `origin/master` and committed `94b7240` (`feat(provenance): add file-backed live proof gate`). The slice adds a proof-key credential drop-in that composes with both GitHub credential choices, makes the existing disposable runner workflow optionally retrieve the proof before any other work and preserve it as a one-day artifact, and documents file ownership, enrollment, and installation.

Local `actionlint` and the complete `moon run root:check` gate passed after clearing stale `golangci-lint` cache entries left by the removed Phase 3 worktree. PR #39 is open at exact head `94b7240273a1bf642de8adf2640f2aa231e5009a`; CI, CodeQL for Go and Actions, GitHub Pages, and Kusari all passed. The live gate is intentionally still open: merge/review approval, two genuine jobs (hot standby and demand-created), external payload comparison, receipt timing, tamper and wrong-enrollment rejection, controlled delivery failure, secret review, and final zero owned inventory remain next.

## 2026-07-20 19:46 — Phase 4 live gate passed after contract corrections
PR #39 was squash-merged at `173381ff18c220177fd75bda2fb920e38a757990`, local `master` was fast-forwarded, and the integrated feature worktree was removed. Reference Image run `29792916213` built the exact merged Ubuntu 24.04 x86_64 image used on one disposable Latitude `c3.small.x86` host (`sv_ogXka9LXd5JdB`, MEX2, Incus 7.0.1).

The first real `JobStarted` events corrected two design assumptions instead of weakening the proof. GitHub reports `runnerRequestId` as zero in the observed live scale-set messages, so zero now means unreported while negative values remain invalid. Incus also copies `image.*` audit properties onto a VM after the create request; proof reconstruction now excludes those server-added properties and rejects `image.*` keys in source profiles, while ordinary config and device drift remain fail-closed. The fixes are commits `8194eff` and `ec829c3` on `feat/job-proof-live-contract`; PR #40 is open at exact head `ec829c34cfb4e52d301b37147ad7368a8d903555`, with local `mise exec -- moon ci` and every hosted CI, CodeQL, Pages, and Kusari check green.

The final live matrix passed on the corrected controller: hot-standby proof run `29795963776`, scale-from-zero proof run `29796083069`, proof-disabled compatibility run `29796167521`, and controlled missing-proof failure run `29796238603`. Both successful receipts matched the live GitHub runner and Incus UUID/image/launch/profile metadata; external verification rejected the wrong host ID, wrong public key, and tampered envelope. The unavailable proof failed at the first helper and skipped normal work. Timing repeats `29796424995` and `29796509530` measured JobStarted-to-marker visibility at `0.072407s` for hot standby and `-1.800874s` for demand-created; the small negative value is guest/host clock skew and stayed inside the plan's `-2s` tolerance.

Evidence is under ignored `build/job-proof-phase4-evidence/20260720-sv_ogXka9LXd5JdB-final` and `build/job-proof-phase4-evidence/20260720-sv_ogXka9LXd5JdB-timing`; their checksum-manifest digests are `9a2dbc327bf67fe570d65959089aabf711492079a3db720fe062a4997f673dc8` and `c5e18f720ca6fd3d84e32a24db2506532f10685db8aa15f4fe012449f61105f4`. Exact-token/private-key scans passed, final owned Incus inventory was empty, the remote and local ephemeral private keys were removed, and Latitude deletion was confirmed by 404 lookup plus an empty project server list. Phase 4 is operationally proven; PR #40 merge remains approval-gated, and Phase 5 TPM-backed credentials remain deferred.

## 2026-07-20 19:53 — Close
Maintainer approval landed PR #40 from the exact reviewed head `ec829c34cfb4e52d301b37147ad7368a8d903555` as squash merge `e1d5979b9b9dd944a610496ece42b698c0d16f0f`. Every substantive hosted CI, CodeQL, Pages-build, and Kusari check was green; the release dry-run and Pages deployment jobs were intentionally skipped by repository gates. Local `master` was fast-forwarded to the merge, and the integrated `feat/job-proof-live-contract` Worktrunk and branch were removed.

Session 019 closes with Phase 4 fully implemented and operationally proven through PRs #39 and #40. The bounded evidence, checksum manifests, clean secret scan, destroyed disposable host, and zero owned Incus inventory remain recorded above and in `SUMMARY.md`. Phase 5 TPM-bound credential validation is deliberately deferred.

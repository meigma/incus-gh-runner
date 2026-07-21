---
id: 019
title: Implement job machine proof phase 4
date: 2026-07-20
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [014, 015, 017, 018]
---

## Goal

Review Session 014's job machine-proof design and implementation plan, then
execute Phase 4's genuine GitHub/Incus proof-consumption gate without expanding
into TPM-backed credentials or additional provenance claims.

## Outcome

The goal was met. The file-backed proof deployment and workflow consumption
surface landed through PR #39 as `173381f`, and live GitHub jobs exposed two
contract assumptions that were corrected and landed through PR #40 as
`e1d5979`. The final matrix proved hot-standby and scale-from-zero receipts,
proof-disabled compatibility, fail-closed missing-proof behavior, external
payload agreement, negative verification cases, bounded receipt timing, secret
cleanup, and zero final owned inventory.

## Key Decisions

- Treat a zero GitHub `runnerRequestId` as unreported while rejecting negative
  values -> genuine scale-set `JobStarted` messages used zero, so requiring a
  positive value rejected authenticated live events without adding identity.
- Exclude Incus-managed `image.*` audit properties from launch-digest
  reconstruction and reserve those profile keys -> Incus adds them after the
  create request, while controller-authored config and device drift must remain
  fail-closed.
- Keep proof retrieval as the first optional workflow step and use the same
  file-backed PKCS#8 credential format as Phase 1 -> Phase 4 proves the complete
  deployed claim without coupling it to Phase 5 storage binding.
- Preserve the first live failures as design feedback -> the implementation was
  corrected against real upstream behavior rather than weakening the acceptance
  matrix or treating the Session 014 plan as immutable.

## Changes

- `.github/workflows/runner-functional.yml` - optionally retrieves a machine
  proof before normal work and retains the receipt as a one-day artifact.
- `deploy/systemd/credentials-job-proof-file.conf` and supporting deployment
  examples/checks - install the file-backed signing key as a systemd credential.
- `docs/docs/how-to/deploy.md` and
  `docs/docs/reference/configuration.md` - document key enrollment, ownership,
  installation, and workflow behavior.
- `internal/provenance/*` - accept an unreported zero runner request ID while
  retaining strict validation and coverage for negative values.
- `internal/adapters/incus/backend.go` and tests - omit server-added `image.*`
  audit metadata from the pre-metadata launch contract and reject collisions in
  source profiles.

## Open Threads

- Phase 5 remains deferred: validate the same PKCS#8 Ed25519 key through a
  systemd-250+ TPM-bound credential. It remains storage binding, not TPM-native
  signing or measured boot.
- Session 014's positive-only `runnerRequestId` assumption is superseded by the
  live contract recorded here and by the merged validation code.

## Lessons

- Genuine upstream events and a real Incus create/read cycle were necessary to
  discover contract behavior that deterministic tests and SDK types did not
  reveal.
- A small negative host-to-guest timing delta can be clock skew; the observed
  `-1.800874s` remained within the plan's explicit `-2s` tolerance.

## References

- PR #39: https://github.com/meigma/incus-gh-runner/pull/39
- PR #40: https://github.com/meigma/incus-gh-runner/pull/40
- Phase 4 merge commits: `173381ff18c220177fd75bda2fb920e38a757990`
  and `e1d5979b9b9dd944a610496ece42b698c0d16f0f`
- Acceptance runs: `29795963776`, `29796083069`, `29796167521`, and
  `29796238603`; timing repeats: `29796424995` and `29796509530`.
- Preserved ignored evidence:
  `build/job-proof-phase4-evidence/20260720-sv_ogXka9LXd5JdB-final` and
  `build/job-proof-phase4-evidence/20260720-sv_ogXka9LXd5JdB-timing`.
- Evidence checksum-manifest SHA-256 digests:
  `9a2dbc327bf67fe570d65959089aabf711492079a3db720fe062a4997f673dc8` and
  `c5e18f720ca6fd3d84e32a24db2506532f10685db8aa15f4fe012449f61105f4`.
- Design and plan: `.journal/014/JOB_MACHINE_PROOF_DESIGN.md` and
  `.journal/014/JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md`.

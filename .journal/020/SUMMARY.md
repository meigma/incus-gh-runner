---
id: 020
title: Implement job machine proof phase 5
date: 2026-07-21
status: complete
repos_touched: [incus-gh-runner, catalyst-infra]
related_sessions: [014, 019]
---

## Goal
Review Session 014's job machine-proof design and plan, then implement and validate Phase 5: protect the existing PKCS#8 Ed25519 proof key with a systemd TPM-bound credential without changing the proof protocol or controller sandbox.

## Outcome
The goal was met. PR #41 landed TPM-bound systemd credential packaging, verification, and operator documentation. A physical TPM on `sre@ci` passed seal/unseal, PID 1 loading, rotation, normal reboot, and a genuine GitHub job proof whose repository, workflow, runner, scale set, Incus UUID, image, profile, and launch identities all matched independent records. External verification passed, negative checks failed closed, and all disposable capacity was removed.

## Key Decisions
- Keep the existing software Ed25519 key and runtime credential contract, changing only systemd storage -> file-backed and TPM-bound deployments retain identical proof schemas, key IDs, verifier behavior, and `PrivateDevices=yes` sandboxing.
- Use systemd's explicit empty PCR set -> normal firmware, kernel, and bootloader updates should not lock out the controller; this is TPM-bound storage, not measured boot or TPM-native signing.
- Treat the encryption attempt as the portable capability test and require the complete TSS2 runtime -> the target systemd range does not share one TPM probe command, and partial TSS2 installations fail misleadingly.
- Use the intentionally scoped Catalyst Infra GitHub App and a disposable exact-label workflow -> the App cannot access `meigma/incus-gh-runner`, while `cardano-foundation/catalyst-infra` was authorized and safe for a temporary proof run.
- Preserve the authorized root-only GitHub App key and TPM userspace packages on `sre@ci`, but remove the test enrollment and controller deployment -> the server keeps the approved production prerequisite without retaining Phase 5 test state.

## Changes
- `deploy/systemd/credentials-job-proof-tpm.conf` - added the TPM-encrypted proof-key loader with the same runtime credential name and environment variable as file-backed mode.
- `deploy/systemd/verify.sh` - covered every GitHub App/PAT and file/TPM drop-in combination, installed credential ownership and modes, and continued `PrivateDevices=yes` behavior.
- `deploy/systemd/config.example.yaml` - clarified the TPM-bound proof-key configuration surface.
- `docs/docs/how-to/deploy.md` - documented enrollment, dependencies, empty-PCR policy, rotation, reboot, recovery, escrow, cross-host testing, and external verification.
- `docs/docs/explanation/how-it-works.md` and `docs/docs/reference/configuration.md` - documented the assurance boundary and configuration behavior without TPM-native or measured-boot claims.
- `cardano-foundation/catalyst-infra` PR #180 - temporarily added the exact-label proof workflow, then closed it without merge and deleted its branch/worktree after successful run `29867201183`.

## Open Threads
- Cross-host decryption remains an explicitly untested optional evidence gap because a second TPM host was not used.
- `sre@ci` retains the approved GitHub App private key and `tpm2-tools`, but no controller service, config, TPM proof enrollment, imported test image, scale set, or runner VM; production enrollment and deployment remain separate operator work.

## Lessons
- systemd's generic `AES-128-CFB missing?` error can mask a partial TSS2 runtime. Debug logging showed `libtss2-esys.so.0` present but `libtss2-rc.so.0` absent; installing `tpm2-tools` supplied the complete required library set.
- A genuine proof should be checked against both GitHub job metadata and the pre-job Incus instance snapshot; signature verification alone does not enforce consumer policy.
- Selected-repository GitHub App scope fails closed before capacity creation when the configured repository is not installed, which made the initial Meigma target rejection safe and observable.

## References
- [PR #41](https://github.com/meigma/incus-gh-runner/pull/41)
- [Genuine TPM-backed workflow run](https://github.com/cardano-foundation/catalyst-infra/actions/runs/29867201183)
- [Disposable Catalyst Infra PR #180](https://github.com/cardano-foundation/catalyst-infra/pull/180)
- `.journal/014/JOB_MACHINE_PROOF_DESIGN.md`
- `.journal/014/JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md`
- `build/job-proof-phase5-evidence/20260721-ci-tpm/`

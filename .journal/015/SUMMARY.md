---
id: 015
title: Implement job machine proof phase 1
date: 2026-07-20
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [014]
---

# Session 015 Summary

## Goal

Review Session 014's job-bound machine-proof design and implementation plan, then implement the first locally verifiable phase.

## Outcome

Phase 1 was implemented, verified, reviewed, and squash-merged through PR #36 as `ce7c89c`. The repository now has strict version 1 proof primitives, bounded file adapters, optional startup configuration, a configuration-independent verifier command, and operator documentation. Phase 2 was not started.

## Key Decisions

- Keep the proof model, digest construction, and cryptography in a pure `internal/provenance` core, with filesystem and CLI behavior in adapters.
- Require exactly one Ed25519 DSSE signature and derive stable key IDs from the public key's SPKI encoding rather than a library-specific OpenSSH representation.
- Make proof configuration an optional pair and load the signing key once at startup, preserving existing behavior when proof generation is disabled.
- Share profile-digest construction between provenance and the live Incus adapter so the golden vector protects the format used by real runner instances.
- Keep `proof verify` policy-neutral and write a payload to stdout only after complete cryptographic and structural verification.

## Changes

- Added strict payload, launch digest, profile digest, key ID, DSSE signing, and verification logic under `internal/provenance`.
- Added bounded PKCS#8 private-key and SPKI public-key file loading under `internal/adapters/provenancefile`.
- Added paired `job_proof` configuration and one-time startup key loading in the command and runtime layers.
- Added `incus-gh-runner proof verify` and adversarial coverage for malformed, oversized, tampered, multi-signature, wrong-key, and wrong-host inputs.
- Documented enrollment, rotation, verification, and disabled-by-default behavior in the configuration reference.

## Open Threads

- Begin Phase 2 from Session 014's implementation plan: bind authenticated GitHub job-start data to the exact JIT registration and deliver the signed proof from host to VM.
- GitHub correlation, live end-to-end proof, and TPM-bound credential validation remain later phases; the overall job machine-proof feature is not yet complete.

## References

- PR #36: https://github.com/meigma/incus-gh-runner/pull/36
- Merge commit: `ce7c89c920ac16cf0422bb8e554498b0549524cf`
- Design: `.journal/014/JOB_MACHINE_PROOF_DESIGN.md`
- Implementation plan: `.journal/014/JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md`
- Hosted CI run: `29784017360`
- Hosted CodeQL runs: `29784015784`, `29784015431`

## Lessons

- The library's OpenSSH-oriented key identifier did not match the design's SPKI contract, so explicit SPKI derivation made the compatibility boundary unambiguous.
- Sharing the profile adapter with the provenance core prevents a passing proof fixture from drifting away from the format used in live Incus configuration.

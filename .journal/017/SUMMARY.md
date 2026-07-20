---
id: 017
title: Implement job machine proof phase 2
date: 2026-07-20
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [014, 015]
---

## Goal

Review Session 014's job machine-proof design and plan, then implement and
prove the Phase 2 host-to-VM delivery slice without entering GitHub job
correlation or TPM work.

## Outcome

The goal was met. Phase 2 was implemented, verified locally, proven against a
hosted reference image on Incus 7.2, reviewed, and squash-merged through PR
#37 as `ea7e504`. The repository now has an ownership-fenced Incus proof sink,
an immutable guest commit protocol, an unprivileged retrieval helper, contract
documentation, and deterministic plus real-Incus functional coverage.

The live proof used a disposable Latitude server and project. The image
contract validator passed, `TestProofSinkFunctional` passed in 32.49 seconds,
and guest-agent proof delivery took 78.13291 ms. The test verified exact bytes,
ownership, permissions, readiness behavior, and VM cleanup. The imported image
and server were deleted, and the final Latitude project server list was empty.

## Key Decisions

- Keep machine proofs in a separate `0755` public staging directory -> the
  existing root-only JIT payload boundary remains unchanged.
- Write `job-proof.dsse.json` before `job-proof.ready` and make both `0444` ->
  the marker is an unambiguous commit point and committed proofs are immutable.
- Verify the signed envelope and exact owned instance UUID before the proof
  write, then recheck the target before the marker write -> instance
  replacement, ownership drift, and teardown races fail closed.
- Accept a committed duplicate only when its signed job, runner, and instance
  tuple matches -> retries are idempotent without allowing proof replacement.
- Let the guest helper validate shape and copy to a caller-owned `0600` file,
  but leave signature and policy verification external -> the helper needs no
  enrolled host key or additional guest credential.
- Stop at the Phase 2 gate -> GitHub event authority, JIT correlation,
  coordinator lifecycle, and TPM storage remain separate reviewable slices.

## Changes

- `internal/provenance/delivery.go` - narrow machine-target, verifier, and
  proof-sink ports.
- `internal/provenance/crypto.go` - reusable verified-payload decoding for the
  delivery adapter.
- `internal/adapters/incus/proof_sink.go` - verified, ownership-scoped,
  race-fenced, immutable Incus guest-agent delivery.
- `internal/adapters/incus/proof_sink_test.go` - adversarial unit coverage for
  target drift, replacement, duplicate, marker, and write failures.
- `internal/adapters/incus/proof_sink_functional_test.go` - opt-in real Incus
  VM proof-delivery harness.
- `image/guest/incus-gh-runner-proof` - bounded wait, shape check, and safe
  unprivileged proof copy.
- `image/image.yaml`, `image/guest/incus-gh-runner.conf`, and
  `image/tests/guest-entrypoint-test.sh` - install the proof contract and test
  its permissions and behavior in the reference image.
- `docs/docs/reference/guest-contract.md` - operator-facing paths,
  permissions, commit semantics, and helper usage.

## Open Threads

- Phase 3 must bind authenticated GitHub `JobStarted` data to the exact JIT
  registration and owned VM, persist and fence the JIT reference, and add the
  supervised signing/delivery coordinator.
- Phase 4 must prove the complete file-backed claim with genuine GitHub jobs;
  the Phase 2 live test intentionally exercised only the delivery channel.
- TPM-bound systemd credential validation remains Phase 5.
- The `actions/scaleset` acknowledgement-before-callback availability gap from
  the design remains intentional; proof-required work fails through helper
  timeout if a crash loses the event.

## Lessons

- The fixed host-push channel is operationally sound on Incus 7.2; 78.13291 ms
  delivery is comfortably inside the helper's 60-second default.
- `incus admin init --minimal` may leave the default profile without a root
  disk, and remote Incus CLI invocations should close SSH stdin. These were
  disposable-host setup issues, not proof-channel defects.
- Building and validating the exact hosted image before the functional proof
  caught the real guest contract while keeping the paid hardware window
  bounded.

## References

- PR #37: https://github.com/meigma/incus-gh-runner/pull/37
- Merge commit: `ea7e504087fd7e5d7b49782a5093fc9c48021e79`
- PR Reference Image run: https://github.com/meigma/incus-gh-runner/actions/runs/29787697737
- Pre-PR exact-head Reference Image run: https://github.com/meigma/incus-gh-runner/actions/runs/29786457401
- Preserved local evidence: `build/live-phase2-evidence/20260720T231117Z/`
- Evidence checksum index SHA-256: `4eeab43c443948c8b28b94627a4ab9490854baab3d30d85a3624ebfee267fd14`
- Design and plan: `.journal/014/JOB_MACHINE_PROOF_DESIGN.md` and
  `.journal/014/JOB_MACHINE_PROOF_IMPLEMENTATION_PLAN.md`
- Phase 1: `.journal/015/SUMMARY.md`

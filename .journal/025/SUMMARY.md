---
id: 025
title: Investigate machine-proof availability delay
date: 2026-07-25
status: complete
repos_touched: [incus-gh-runner, meigma/packages]
related_sessions: [014, 019]
---

## Goal

Identify why Phoenix jobs could begin execution 26–46 seconds before the
job-bound machine proof became available, determine whether the Catalyst colo
or Incus infrastructure caused the delay, and deliver the smallest safe
resolution without weakening proof semantics.

## Outcome

The goal was met. A disposable Latitude reproduction showed the same
approximately 50-second delay outside Catalyst, while proof issuance remained
within about 6 milliseconds of the authenticated `JobStarted` callback. A
controlled hot-standby A/B canary then reduced the client-visible poll boundary
from approximately 50 seconds to 5 seconds and reduced proof wait from
47.84 seconds to 2.09 seconds. The immediately following poll received
`JobStarted` in about 0.13–0.15 seconds in both cases, isolating the delay to the
GitHub scale-set message service's handling of an already in-flight
`GetMessage` request (or undocumented endpoint semantics), not Catalyst,
Incus, the guest, or local proof generation.

The behavior was reported upstream as
[actions/scaleset#122](https://github.com/actions/scaleset/issues/122). PR #58
landed an opt-in, minimum-five-second `github.message_poll_timeout` workaround
with default behavior unchanged. PR #59 released the change as immutable
`v1.3.0` at `b061576`. The tag workflow, all release payload checksums and
attestations, SBOMs, multi-architecture image provenance and Cosign signature,
and downstream DEB/RPM publication and clean installs all passed. No Catalyst
or Phoenix production configuration was changed during the session.

## Key Decisions

- Reproduce on a disposable Latitude host before changing code -> the matching
  50-second boundary proved Catalyst colo networking and Incus hosting were not
  required for the symptom.
- Use a five-second A/B canary against the same hot-standby path -> the
  approximately 45-second causal shift isolated the current long poll rather
  than merely correlating the delay with proof retrieval.
- File the upstream issue before shipping a workaround -> preserves the
  distinction between the observed endpoint behavior and the controller-side
  mitigation.
- Keep the workaround opt-in with a `0s` default and a `5s` minimum -> existing
  deployments retain upstream behavior, while operators can consciously trade
  more idle requests for lower proof latency.
- Translate only the controller-owned child deadline into an empty poll ->
  parent cancellation, transport failures, and upstream errors continue to
  fail normally instead of being hidden.
- Preserve staging-before-production package publication -> the stale R2
  staging signature failed closed; production stayed untouched until a newly
  scoped credential passed both allowed-prefix and outside-prefix tests.

## Changes

- `internal/config/` - added typed YAML/environment loading and validation for
  `github.message_poll_timeout` /
  `INCUS_GH_RUNNER_GITHUB_MESSAGE_POLL_TIMEOUT`.
- `internal/adapters/github/` - added sanitized message-poll timing and the
  bounded child-context reset while preserving cancellation and error
  semantics.
- `internal/runtime/` - wired the configured timeout and emitted a startup
  warning when the workaround is active.
- `deploy/systemd/config.example.yaml` and
  `docs/docs/reference/configuration.md` - documented the opt-in setting, issue
  reference, and idle request-rate tradeoff.
- Release metadata in `.release-please-manifest.json`, `CHANGELOG.md`,
  `apko.yaml`, and `melange.yaml` advanced to `1.3.0`.
- The `meigma/packages` staging R2 credential was reissued from the current
  parent, confined to `meigma-packages/_staging/`, stored in 1Password and the
  protected GitHub environment, and set to expire
  `2027-07-26T03:06:57Z`; no package-repository source changed.

## Open Threads

- Track [actions/scaleset#122](https://github.com/actions/scaleset/issues/122).
  If GitHub or the upstream client supplies correct in-flight delivery or a
  supported poll-duration control, revalidate and remove the controller
  workaround.
- Enabling `github.message_poll_timeout: 5s` in Catalyst/Phoenix production is
  a separate operator decision; this session did not deploy or configure it.
- Renew or replace the protected staging R2 credential before
  `2027-07-26T03:06:57Z`; automated renewal or an OIDC broker remains preferable
  to another manual rotation.

## Lessons

- Workflow-step timing alone was insufficient: the decisive evidence combined
  `runnerAssignTime`, poll start/completion, callback entry, and signed
  `issued_at` timestamps.
- An in-flight empty long poll can hide a message that the immediately
  following request receives, so moving proof retrieval cannot solve this
  boundary; shortening or fixing the poll does.
- Do not inspect live runner processes with commands that print arguments or
  environment: one discarded canary exposed ephemeral JIT material and had to
  be canceled, deregistered, and destroyed immediately.
- Canary artifacts must be built for the target OS and include the exact guest
  proof helper before any timing result is accepted.

## References

- [PR #58](https://github.com/meigma/incus-gh-runner/pull/58), merge commit
  `9b0f2a52b7e14424640d1b5dd6d4715c8cbab362`.
- [PR #59](https://github.com/meigma/incus-gh-runner/pull/59), release commit
  `b061576671c82ce4e8513d87df7b94aff680b18b`.
- [v1.3.0 release](https://github.com/meigma/incus-gh-runner/releases/tag/v1.3.0)
  and [release run 30185020175](https://github.com/meigma/incus-gh-runner/actions/runs/30185020175).
- [Package publication run 30185235050](https://github.com/meigma/packages/actions/runs/30185235050),
  successful on attempt 3 after staging credential recovery.
- Latitude baseline run `30165004574`; A/B runs `30166003024` and
  `30166076210`.
- Preserved ignored evidence:
  `build/job-proof-latitude-delay-20260725/evidence/` with checksum-manifest
  SHA-256 `b83c6ff22a19072abcd0cddcc4a72b97d270d70e258003bb329b6c7213757ce7`,
  and `build/job-proof-short-poll-20260725/evidence/` with checksum-manifest
  SHA-256 `942081949ac362fccb12b563700138b93625f2d43d10f90ec31a646abd514db9`.
- `.journal/014/SUMMARY.md` and `.journal/019/SUMMARY.md` for the machine-proof
  design and live proof-consumption context.

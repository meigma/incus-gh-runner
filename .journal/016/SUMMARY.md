---
id: 016
title: Assess bootc image migration
date: 2026-07-20
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [004, 010, 014]
---

## Goal

Evaluate replacing the distrobuilder-based Ubuntu reference VM with a bootc
workflow so OCI could become the authored image format, validate the idea on a
real x86_64/KVM Incus host, and preserve the maintainer's final decision.

## Outcome

The assessment goal was met and the migration proposal was rejected. A Fedora
44 bootc prototype passed the repository's complete Incus guest-contract
validator on a disposable Latitude bare-metal server, proving the path was
technically feasible. The experiment also exposed enough integration and
provenance costs that the maintainer chose to retain the original
distrobuilder-based Ubuntu 24.04 plan.

No implementation was merged. The prototype remained uncommitted, no PR was
opened, and `feat/bootc-image-experiment` was removed with its Worktrunk after
the assessment was recorded.

## Key Decisions

- Retain distrobuilder -> bootc added an OCI-to-qcow2-to-Incus conversion bridge,
  compatibility work, a guest-distribution change, and a derived-artifact
  provenance boundary without enough project-specific benefit.
- Treat the successful Fedora prototype as evidence, not a pending migration ->
  it passed the full guest contract, but feasibility did not justify adoption.
- Reject CentOS Stream 10 for this path -> its kernel disabled the 9p support
  Incus 7.0.1 requires for the VM agent.
- Do not merge or preserve the prototype as product code -> the proposal was
  rejected, so historical findings belong in the journal rather than `master`.

## Changes

- `.journal/016/BOOTC_ASSESSMENT.md` - complete experiment environment,
  measurements, compatibility findings, attestation analysis, decision, and
  cleanup record.
- `.journal/016/NOTES.md` - kickoff, assessment checkpoint, and final handoff.
- `.journal/TECH_NOTES.md` - durable direction to retain distrobuilder and avoid
  treating bootc as an open migration thread.
- `feat/bootc-image-experiment` - uncommitted prototype abandoned and its
  Worktrunk/branch removed; no production files changed.

## Open Threads

- None. Bootc is not a pending migration. Reopen the decision only if a future
  maintainer has materially different requirements or the tooling eliminates
  the observed conversion and compatibility costs.

## Lessons

- An OCI-authored bootable image does not remove Incus' native delivery format;
  the qcow2/Incus archive conversion still needs provenance linking its checksum
  back to the OCI digest.
- Bootc's immutable filesystem model changes runner layout: writable state under
  `/opt/actions-runner` must be redirected into `/var`.
- A raw KVM boot is insufficient acceptance evidence. CentOS booted directly,
  but failed the real Incus agent contract because its kernel lacked 9p support.

## References

- `.journal/016/BOOTC_ASSESSMENT.md` - primary historical assessment.
- `.journal/004/SUMMARY.md` - original distrobuilder reference-image work.
- `.journal/010/SUMMARY.md` - reference-image release and attestation context.
- `.journal/014/SUMMARY.md` - job-bound machine-proof scope and provenance context.
- [bootc filesystem model](https://bootc.dev/bootc/filesystem.html)
- [osbuild bootc-image-builder deprecation notice](https://osbuild.org/docs/bootc/deprecation-notice/)
- [Incus image format](https://linuxcontainers.org/incus/docs/main/reference/image_format/)


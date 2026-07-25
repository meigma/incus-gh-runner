---
id: 025
title: Investigate machine-proof availability delay
started: 2026-07-25
---

## 2026-07-25 08:50 — Kickoff
Goal for the session: Investigate the unusually consistent 26–46 second delay between a Phoenix GitHub Actions job beginning execution and its signed machine proof becoming available inside the runner VM.
Current state of the world: The attached investigation brief pins production to incus-gh-runner v1.2.0 at commit 5cabfebcf6c6636fa4e117034a6850f974b883a7 and actions/scaleset v0.4.0, provides two exact corroborating timelines plus a 13-run distribution, and indicates proof issuance completes within milliseconds after the authenticated JobStarted callback. The prompt has been reviewed, but no investigation, production inspection, code change, deployment, upstream submission, or other substantive action has begun.
Plan: Wait for explicit approval to begin, then validate the supplied evidence, inspect the pinned local and upstream source paths, narrow the fault boundary, and report the smallest safe resolution without mutating production.

## 2026-07-25 09:13 — Latitude canary reproduced the hot-standby delay
The maintainer authorized a disposable Latitude canary to determine whether the proof wait depends on the Catalyst colo. One hourly `c3-small-x86` host in MEX2 (`sv_7vRENk9bgadPy`) ran Incus 7.0.1, the exact deployed controller source/version (`v1.2.0`, commit `5cabfebcf6c6636fa4e117034a6850f974b883a7`), and the proof-capable VM archive from the earlier Phase 4 live manifest. The guest used Actions Runner `2.335.1`, one patch behind the Phoenix observation of `2.336.0`.

The valid hot-standby workflow was run `30165004574`, job `89696403256`, on an already connected idle VM. The proof step began at `16:07:53.4147511Z` and the following artifact step began at `16:08:42.2141434Z`, an observed wait of `48.7993923s` (49 seconds in the Jobs API). The controller opened `GetMessage` at `16:07:51.493732504Z`; the same `lastMessageID=100000041` poll ended at `16:08:41.618608928Z` after `50.124876424s`. The next poll received `JobStarted` at `16:08:42.044198141Z`, `425.589213ms` later. The signed proof's `issued_at` was `16:08:42.050013619Z`, only `5.815478ms` after callback entry. External signature/host verification passed, and the receipt identified controller `1.2.0`/`5cabfeb`, runner ID `69`, the exact Incus VM UUID, and image fingerprint.

This independently reproduces the full hot-standby delay outside Catalyst: a Latitude controller and runner exposed essentially the same 50-second empty long-poll boundary and millisecond callback-to-proof time. The Catalyst colo/Incus host/network path is therefore not required for the symptom. The narrow behavior remains in the GitHub scale-set message path or pinned client semantics, not local proof generation.

Two setup attempts were excluded from evidence. The first controller artifact was accidentally built for macOS and systemd rejected it before capacity existed; it was replaced with the pinned Linux/amd64 build. The first VM archive lacked `incus-gh-runner-proof`, so run `30164839777` failed immediately with command-not-found; it was cleaned up and replaced by the exact proof-capable Phase 4 archive.

A scale-from-zero comparison (`30165064309`) was canceled without executing: its JIT runner connected and logged `Listening for Jobs`, but GitHub reported it offline and left the job queued. During diagnosis, a process-status command printed that disposable runner's ephemeral JIT configuration in tool output, violating the test's secret-handling constraint. The run was canceled, runner registration and owned VM were deleted, remote credentials were removed, and the host was immediately destroyed, invalidating the exposed JIT material. Teardown was verified by a 404 server lookup, an empty Latitude project server list, zero remaining test runner registrations, and removal of the local ephemeral private key.

Ignored evidence is under `build/job-proof-latitude-delay-20260725/evidence/`; its checksum-manifest SHA-256 is `b83c6ff22a19072abcd0cddcc4a72b97d270d70e258003bb329b6c7213757ce7`. A targeted sensitive-pattern scan of the preserved evidence passed.

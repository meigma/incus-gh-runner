---
id: 025
title: Investigate machine-proof availability delay
started: 2026-07-25
---

## 2026-07-25 08:50 — Kickoff
Goal for the session: Investigate the unusually consistent 26–46 second delay between a Phoenix GitHub Actions job beginning execution and its signed machine proof becoming available inside the runner VM.
Current state of the world: The attached investigation brief pins production to incus-gh-runner v1.2.0 at commit 5cabfebcf6c6636fa4e117034a6850f974b883a7 and actions/scaleset v0.4.0, provides two exact corroborating timelines plus a 13-run distribution, and indicates proof issuance completes within milliseconds after the authenticated JobStarted callback. The prompt has been reviewed, but no investigation, production inspection, code change, deployment, upstream submission, or other substantive action has begun.
Plan: Wait for explicit approval to begin, then validate the supplied evidence, inspect the pinned local and upstream source paths, narrow the fault boundary, and report the smallest safe resolution without mutating production.

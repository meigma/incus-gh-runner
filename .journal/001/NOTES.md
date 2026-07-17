---
id: 001
title: Incus runner kickoff
started: 2026-07-17
---

## 2026-07-17 13:53 — Kickoff
Goal for the session: Begin the initial work on `incus-gh-runner`; the substantive implementation goal is pending the user's next request.
Current state of the world: The private repository was created from `meigma/template-go`, `master` is clean, and the personal `journal/jmgilman` worktree is initialized and published.
Plan: Wait for the user's actual request, then inspect the relevant repository surface and proceed incrementally from working behavior.

## 2026-07-17 14:00 — Ephemeral runner architecture research
Goal clarified: Teach the expected architecture and lifecycle of self-hosted ephemeral GitHub Actions runners, with emphasis on the provisioner contract that will inform `incus-gh-runner`.
Current findings: GitHub schedules jobs by `runs-on`; capacity demand reaches a custom provisioner either through the scale-set message API/long-poll listener or `workflow_job` webhooks. The provisioner creates clean capacity, obtains a per-runner JIT configuration, starts `run.sh --jitconfig`, observes execution, exports diagnostics, and destroys the Incus instance after the one-job runner exits. GitHub automatically deregisters a successfully used ephemeral runner, while reconciliation must clean up failed starts, stale runner records, and orphaned instances.
Design direction to explore: Prefer the official Go `actions/scaleset` client for a prototype because it exposes current demand statistics, acknowledgments, JIT configuration generation, and max-capacity reporting without requiring Kubernetes; keep a webhook-driven implementation as a simpler alternative with more reconciliation burden.

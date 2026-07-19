---
id: 012
title: Support GitHub App and PAT authentication
started: 2026-07-18
---

## 2026-07-18 19:08 — Kickoff
Goal for the session: Cleanly support both GitHub App and personal access token authentication for repository-scoped production runner deployments, then publish the change for review.
Current state of the world: The binary already accepts an environment-only PAT and repository config URL, but product documentation labels PAT use as local-only and the packaged systemd unit unconditionally loads a GitHub App private key. GitHub supports repository-scoped runner scale sets with a fine-grained PAT restricted to Administration read/write on the target repository.
Plan: Create an isolated branch, make the smallest secure systemd/configuration and documentation change that treats both credentials as production options, add focused verification, run the repository gates, and open a pull request.

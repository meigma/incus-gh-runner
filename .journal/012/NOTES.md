---
id: 012
title: Support GitHub App and PAT authentication
started: 2026-07-18
---

## 2026-07-18 19:08 — Kickoff
Goal for the session: Cleanly support both GitHub App and personal access token authentication for repository-scoped production runner deployments, then publish the change for review.
Current state of the world: The binary already accepts an environment-only PAT and repository config URL, but product documentation labels PAT use as local-only and the packaged systemd unit unconditionally loads a GitHub App private key. GitHub supports repository-scoped runner scale sets with a fine-grained PAT restricted to Administration read/write on the target repository.
Plan: Create an isolated branch, make the smallest secure systemd/configuration and documentation change that treats both credentials as production options, add focused verification, run the repository gates, and open a pull request.

## 2026-07-18 19:19 — Production PAT support opened for review
Implemented the bounded credential slice on `feat/production-pat-auth` at `b3f8776`. The controller now accepts `github.token_file` / `INCUS_GH_RUNNER_GITHUB_TOKEN_FILE`, reads and trims the protected PAT once during runtime startup, rejects empty or conflicting credential sources, and retains the existing raw environment PAT path.

Made the base systemd unit credential-neutral and added explicit GitHub App and PAT drop-ins. The deployment guide now covers repository-scoped pairing, explains why GitHub's one-time registration token cannot drive the ephemeral controller, documents current App/PAT permissions, and gives secure installation commands for both methods.

Verification passed with `mise exec -- moon run root:check`; hosted PR checks also passed, including Linux `systemd-analyze`, CodeQL, CI, GitHub Pages build, and Kusari Inspector. Opened PR #26 at https://github.com/meigma/incus-gh-runner/pull/26. The PR is ready for human review and has not been merged.

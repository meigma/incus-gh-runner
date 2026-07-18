---
id: 007
title: Package hosting proposal for .deb/.rpm distribution
started: 2026-07-17
---

## 2026-07-17 20:55 — Kickoff
Goal for the session: capture and iterate on the meigma-wide Linux package
hosting proposal (signed apt/yum repos for incus-gh-runner and future
projects) produced from today's research, and move the draft out of the
master checkout into this session's journal folder.

Current state of the world: a three-agent research workflow compared hosted
services (Cloudsmith, Gemfury, Buildkite/packagecloud, JFrog, GitHub
Packages), distro-native build services (OBS, COPR, Launchpad PPA), and
self-hosted static approaches (GitHub Pages, Cloudflare R2, S3). The chosen
direction is static signed apt+yum repos on Cloudflare R2 behind
`pkgs.meigma.dev` (the user already runs meigma.dev on Cloudflare as
registrar and DNS), with GitHub Releases as the artifact of record and a
shared publish pipeline in a future `meigma/packages` repo. A full proposal
draft was written to `docs/proposals/2026-07-package-hosting.md` in the
master checkout (untracked) and is being relocated to this session folder as
`PACKAGE_HOSTING_PROPOSAL.md` for review.

Plan: move the draft into `.journal/007/`, commit and push the journal
mutation, then await the user's review feedback on the proposal's open
questions (hostname, apt component layout, retention count, dispatch auth,
package-level signing, smoke-test timing).

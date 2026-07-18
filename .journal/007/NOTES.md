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

## 2026-07-17 21:05 — Jumpstart doc for meigma/packages agent
Wrote PACKAGES_REPO_JUMPSTART.md: a self-contained design contract for a
dedicated agent to build meigma/packages from a bare repo. Converted the
proposal's open questions into adopted defaults (pkgs.meigma.dev single
hostname, component-per-project apt layout, retention N=5, metadata-only
signing in v1, PAT-based dispatch, smoke test in v1) and split the remainder
into an explicit escalation list. Included invariants (owned-domain-only
URLs, signed metadata, idempotent + serialized publishes, SHA-pinned
actions, full-rebuild-from-Releases), deliverables (publish/rebuild/smoke
workflows, project registry, shared scripts, docs, secretless PR-level
pipeline test), Josh's external provisioning checklist (R2 bucket + domain,
scoped token, GPG keypair, dispatch PATs), and acceptance criteria ending in
real apt/dnf installs in clean containers. Next: Josh reviews jumpstart +
proposal; dedicated agent executes.

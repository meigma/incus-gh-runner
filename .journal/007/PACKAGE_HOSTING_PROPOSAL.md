# Proposal: Linux Package Hosting for Meigma Projects

- **Status:** Draft for review
- **Date:** 2026-07-17
- **Scope:** meigma-wide infrastructure; incus-gh-runner is the first consumer
- **Decision drivers:** zero/near-zero cost, long-term sustainability, no
  vendor able to break installed users, amortizes across dozens of projects

## Summary

Host signed apt and yum/dnf repositories as static files on Cloudflare R2,
served through a meigma-owned domain (`pkgs.meigma.dev`). Packages are built
by each project's existing GoReleaser/nfpm release flow and attached to
GitHub Releases as the artifact of record. A single shared publish pipeline
in a new `meigma/packages` repository pulls release assets, regenerates repo
metadata, signs it with a meigma GPG key, and syncs the tree to R2.

End users get a normal experience:

```sh
# Debian/Ubuntu
sudo apt install incus-gh-runner   # after one-time repo setup

# Fedora/RHEL-family
sudo dnf install incus-gh-runner   # after one-time repo setup
```

Expected cost: $0 at any realistic scale (R2 free tier: 10 GB storage,
unlimited egress; the domain and Cloudflare account already exist).

## Problem

incus-gh-runner must be installable as a system package (.deb and .rpm) so
it can ship a default config and systemd unit, and so users receive updates
through their package manager. This is the first meigma project with this
requirement, but likely not the last — the solution should be a shared
capability, not a per-project one-off.

Constraints:

1. **No meaningful recurring cost.** These are OSS side projects.
2. **Sustainable.** No dependence on a vendor free tier that can be
   withdrawn. The package-hosting space has a track record here: JFrog
   discontinued its free tier and deactivated existing OSS accounts;
   packagecloud killed free private repos post-acquisition and is now closed
   to new projects.
3. **Users must never be stranded.** The repo URL baked into thousands of
   sources.list / .repo files is the real migration cost. It must be a
   domain meigma controls.
4. **Keep the existing release flow.** GoReleaser + nfpm already produce
   .deb/.rpm artifacts; the hosting layer should consume them, not replace
   them (this rules out source-build services like OBS/COPR/PPA as the
   primary channel — they cannot host prebuilt packages).

## Options Considered

| Option | Verdict |
|---|---|
| **Cloudflare R2 + custom domain (proposed)** | Static signed repos, $0, unlimited egress, no vendor terms that can reach users. We own ~100 lines of publish workflow and a GPG key. |
| GitHub Pages static repo | Same pattern, zero new vendors, but 1 GB site cap + 100 GB/mo soft bandwidth limit, and Cloudflare is already in our trust boundary (registrar + DNS for meigma.dev). R2 is the no-ceilings end state; starting there skips a predicted migration. |
| Cloudsmith OSS program | Best turnkey option: free 50 GB / 200 GB/mo, signed apt+yum, self-service. Rejected as primary because users' sources would point at `dl.cloudsmith.io` — a future policy change breaks installed users. Remains the named fallback. |
| Gemfury free tier | Free and simple, but no formal OSS commitment, and signed apt metadata requires uploading our private key to them. |
| Buildkite Packages (ex-packagecloud) | 1 GB free tier; OSS terms require a sales conversation. Acquisition/repricing history is the instability we're avoiding. |
| JFrog Artifactory | Free tier discontinued; deactivated existing free OSS accounts. |
| GitHub Packages | Does not support apt or yum repositories at all. |
| OBS / COPR / Launchpad PPA | Free and durable, but build from source — nfpm artifacts unusable; requires maintaining RPM spec and/or debian/ packaging in parallel. Possible *additive* channels later, not the primary. |
| Official Debian/Fedora repos | Not realistic for an early-stage niche tool; Fedora via Packit is a plausible 12–18 month follow-up once interfaces stabilize. |

## Proposed Architecture

```
project repo (e.g. incus-gh-runner)
  └─ GoReleaser + nfpm on tag
       ├─ .deb/.rpm attached to GitHub Release   (artifact of record)
       └─ dispatch event ──► meigma/packages repo
                                └─ publish workflow
                                     1. gh release download (new assets)
                                     2. rclone sync R2 → runner (current tree)
                                     3. regenerate apt metadata (apt-ftparchive)
                                        + sign Release/InRelease (GPG)
                                     4. regenerate rpm metadata (createrepo_c)
                                        + sign repomd.xml (GPG)
                                     5. prune per retention policy
                                     6. rclone sync runner → R2
                                                │
                              Cloudflare R2 bucket (meigma-packages)
                                                │
                              custom domain: pkgs.meigma.dev (Cloudflare CDN)
                                                │
                                        apt / dnf clients
```

### Components

- **`meigma/packages` repository** — owns the publish workflow, repo layout
  configuration, retention policy, install documentation, and the public
  signing key. Projects onboard by (a) producing .deb/.rpm release assets
  and (b) sending a dispatch event. The signing key secret is scoped to this
  one repo, and every project gets the same contract: *publish a GitHub
  Release; packages appear.*
- **R2 bucket `meigma-packages`** with custom domain `pkgs.meigma.dev`.
  r2.dev subdomains are rate-limited and not production-appropriate; the
  custom domain is mandatory and also provides CDN caching. A single
  hostname serves both ecosystems (`/apt/...`, `/rpm/...`).
- **GitHub Releases remain canonical.** The R2 tree is a disposable,
  reproducible projection of release assets. Losing the bucket loses
  nothing; the publish workflow can rebuild it from Releases.

### Repository layout

```
pkgs.meigma.dev/
├── meigma.asc                      # armored public signing key (stable URL)
├── apt/
│   ├── dists/stable/
│   │   ├── InRelease / Release / Release.gpg
│   │   └── <component per project>/binary-{amd64,arm64}/Packages{,.gz}
│   │       # components: incus-gh-runner, <future-project>, ...
│   └── pool/<project>/*.deb
└── rpm/
    └── <project>/
        ├── meigma.repo             # ready-to-download .repo file
        ├── *.rpm
        └── repodata/ (repomd.xml + repomd.xml.asc)
```

One apt suite (`stable`) with **one component per project** keeps a single
signed Release file and lets users opt into exactly the projects they want.
The rpm side uses one repo directory per project (yum has no component
concept).

### User install experience

Debian/Ubuntu (deb822 format):

```sh
curl -fsSL https://pkgs.meigma.dev/meigma.asc \
  | sudo tee /etc/apt/keyrings/meigma.asc > /dev/null

sudo tee /etc/apt/sources.list.d/meigma.sources <<'EOF'
Types: deb
URIs: https://pkgs.meigma.dev/apt
Suites: stable
Components: incus-gh-runner
Signed-By: /etc/apt/keyrings/meigma.asc
EOF

sudo apt update && sudo apt install incus-gh-runner
```

Fedora/RHEL-family:

```sh
sudo dnf config-manager addrepo \
  --from-repofile=https://pkgs.meigma.dev/rpm/incus-gh-runner/meigma.repo
sudo dnf install incus-gh-runner
```

### Signing and key management

- Generate an ed25519 GPG keypair for `Meigma Packages <packages@meigma.dev>`
  (or similar). The **primary key stays offline** (password manager /
  hardware token); a **signing-only subkey** is exported into the
  `meigma/packages` repo as an environment-scoped Actions secret.
- apt: sign `Release` (detached `Release.gpg`) and inline `InRelease`.
- rpm: sign `repomd.xml` (detached `.asc`); `.repo` files set
  `repo_gpgcheck=1`.
- Publish the public key at `https://pkgs.meigma.dev/meigma.asc` *and* in
  the `meigma/packages` git repo (two independent trust paths).
- Compromise recovery: revoke the subkey with the offline primary, mint a
  new subkey, re-sign, re-upload. User keyrings hold the primary's identity,
  so no client-side changes are needed.
- Optional hardening (later): also sign individual packages via nfpm so
  `gpgcheck=1` and `debsig` verification work in addition to metadata
  signing.

### Publish workflow mechanics

- **Trigger:** `repository_dispatch` from each project's release workflow,
  authenticated with a fine-grained PAT or (preferably) a small GitHub App
  installed on meigma repos. Payload: `{project, tag}`. A manual
  `workflow_dispatch` fallback allows re-publishing any release.
- **Concurrency:** a single concurrency group serializes publishes — the
  workflow does read-modify-write against the bucket, so overlapping runs
  must queue.
- **Secrets** (environment-scoped in `meigma/packages` only): GPG signing
  subkey + passphrase; R2 API token scoped to object read/write on the one
  bucket. Third-party actions pinned by SHA.
- **Tooling:** `apt-ftparchive` + `createrepo_c` on ubuntu-latest runners —
  both distro-maintained, both proven in this exact pattern (the NoPorts /
  Atsign pipeline runs this on GitHub Pages today; Cloudflare's own blog
  documents the apt/yum-on-R2 pattern).
- **Retention:** keep the newest N versions per project per architecture
  (proposed N=5) in the repo tree. Older versions remain downloadable
  forever from GitHub Releases.

## Costs and limits

| Item | Cost |
|---|---|
| R2 storage | Free ≤ 10 GB (a ~10 MB daemon × 2 arches × 5 retained versions × dozens of projects ≈ low single-digit GB) |
| R2 egress | $0, unlimited — no bandwidth cliff regardless of adoption |
| R2 operations | Free tier: 1M writes / 10M reads per month — far above need |
| Domain / DNS / CDN | Already owned (meigma.dev on Cloudflare) |
| GitHub Actions | Free for public repos |
| **Total** | **$0 expected; worst case pennies/month for storage overage** |

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| GPG subkey exfiltrated from Actions | Signing-only subkey; offline primary enables revocation without re-keying users; environment-scoped secrets; SHA-pinned actions |
| R2 write token leaked | Attacker can replace objects but cannot produce validly signed metadata; clients reject tampering. Scope token to the one bucket; rotate on suspicion |
| Cloudflare changes R2 free tier | Static files + owned domain: rsync tree to any host (GitHub Pages, S3, a VPS) and repoint DNS; users notice nothing. Zero-egress R2 has held since 2022 and is core to Cloudflare's positioning |
| Read-modify-write races between releases | Workflow concurrency group serializes publishes |
| Bucket corruption / accidental deletion | Tree is a projection of GitHub Releases; a full-rebuild workflow restores it from scratch |
| Key/metadata bugs strand users on stale versions silently | Add a scheduled smoke test: `apt update`/`dnf check-update` against the live repo in CI, alert on failure |

## What this does NOT cover (deliberately)

- **Distro-native channels** (COPR, PPA, OBS, official Debian/Fedora):
  additive later if demand appears; COPR via Packit is the most plausible
  next step for Fedora users.
- **Windows/macOS packaging** (winget, Homebrew): separate concerns,
  GoReleaser already has OSS paths for these.
- **Private packages:** everything here is public-only.

## Fallback

If this approach proves more burdensome than expected, the named fallback
is **Cloudsmith's OSS program** (free 50 GB / 200 GB/mo, self-service,
signed apt+yum, attribution badge required). The main loss would be URL
ownership — mitigated only partially by documentation.

## Open questions for review

1. **Hostname:** single `pkgs.meigma.dev` (proposed) vs split
   `apt.meigma.dev` / `rpm.meigma.dev`?
2. **Retention N=5** per project/arch — right number?
3. **Dispatch auth:** fine-grained PAT (simpler) vs GitHub App (cleaner,
   no expiry churn) for `repository_dispatch`?
4. **apt component-per-project** (proposed) vs one flat `main` component —
   flat is simpler for users installing multiple meigma tools; components
   are cleaner isolation.
5. **Package-level signing** (nfpm rpm/deb signatures) in v1, or metadata
   signing only (proposed) with package signing as a follow-up?
6. Should the smoke-test cron live in `meigma/packages` from day one?

## Rollout plan

1. Create `meigma/packages` repo; generate keypair; store subkey secret.
2. Create R2 bucket + `pkgs.meigma.dev` custom domain; scoped API token.
3. Build publish workflow; validate end-to-end with a throwaway test
   package against a staging prefix in the bucket.
4. Add nfpm config (default config file, systemd unit, postinstall scripts)
   to incus-gh-runner's GoReleaser setup, plus the release-time dispatch.
5. Publish first real release; verify install/upgrade on Debian stable,
   Ubuntu LTS, and Fedora VMs (or containers where systemd isn't needed).
6. Write user-facing install docs in the mkdocs site; add the repo-health
   smoke test.

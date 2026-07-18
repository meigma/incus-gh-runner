# Jumpstart: `meigma/packages`

You are building `meigma/packages` from a bare repository. This document is
self-contained; treat it as the design contract. Where it says "default," adopt
the default without asking. Where it says "escalate," stop and ask Josh.

## Mission

Build the shared publishing pipeline that turns GitHub Release assets
(.deb/.rpm built by GoReleaser/nfpm in individual meigma project repos) into
signed, static apt and yum/dnf repositories served from Cloudflare R2 behind
`pkgs.meigma.dev`. First consumer: `meigma/incus-gh-runner`. Design for dozens
of projects: onboarding a new project must be a config entry, not new
infrastructure.

## Why this shape (compressed rationale)

- Hosted package services have rug-pull history (JFrog killed its free tier
  and deactivated OSS accounts; packagecloud closed to new projects
  post-acquisition). A static repo on object storage behind a meigma-owned
  domain is the only architecture where no vendor decision can strand
  installed users — worst case, the tree rsyncs elsewhere and DNS moves.
- Cloudflare already holds meigma.dev (registrar + DNS), and R2 has zero
  egress fees and a 10 GB free tier. Expected steady-state cost: $0.
- GitHub Releases stay the artifact of record. The R2 tree is a disposable
  projection: it must always be fully reconstructable from Releases alone.
- Packages are static Go binaries packaged by nfpm — distro-agnostic, so one
  apt suite and one rpm repo per project serve all deb/rpm distros. No
  per-distro builds.

## Invariants (do not trade these away)

1. Users' sources reference `pkgs.meigma.dev` only. No vendor hostname ever
   appears in user-facing config or docs.
2. All repo metadata is GPG-signed. The signing key never leaves GitHub
   Actions secrets; the primary key is offline (Josh holds it — the CI key is
   a signing-only subkey).
3. Publishes are idempotent: re-publishing the same (project, tag) must
   converge to the same repo state without error.
4. Publishes are serialized: one concurrency group, queued not cancelled
   (a cancelled half-written publish must never be possible; late-arriving
   runs must still publish their release).
5. Every third-party action pinned by commit SHA. Workflow `permissions:`
   blocks are minimal and explicit.
6. A full-rebuild path exists that reconstructs the entire bucket from
   GitHub Releases with an empty starting state.

## Architecture

```
project repo release workflow
  └─ repository_dispatch → meigma/packages  (event: publish-package)
       payload: { "project": "<repo name>", "tag": "vX.Y.Z" }

meigma/packages publish workflow:
  1. gh release download (deb+rpm assets, checksums) from meigma/<project>@<tag>
  2. rclone sync bucket → workspace          (current repo state)
  3. place packages into pool/project dirs; apply retention
  4. regenerate apt metadata (apt-ftparchive) → sign Release.gpg + InRelease
  5. regenerate rpm metadata (createrepo_c)  → sign repomd.xml.asc
  6. rclone sync workspace → bucket          (checksum-based, delete extraneous)

R2 bucket `meigma-packages` ← custom domain https://pkgs.meigma.dev (CDN-cached)
```

## Bucket / URL layout

```
pkgs.meigma.dev/
├── index.html                     # minimal landing page: what this is, links
├── meigma.asc                     # armored public signing key (stable URL, never moves)
├── apt/
│   ├── dists/stable/
│   │   ├── InRelease, Release, Release.gpg
│   │   └── <project>/binary-{amd64,arm64}/Packages{,.gz}   # component per project
│   └── pool/<project>/*.deb
└── rpm/<project>/
    ├── meigma.repo                # ready-to-use repo file
    ├── x86_64/*.rpm  aarch64/*.rpm
    └── repodata/                  # repomd.xml + repomd.xml.asc
```

Notes:
- apt: single suite `stable`, one component per project. Architectures:
  amd64, arm64. Set proper Origin/Label/Suite/Codename/Components/
  Architectures and SHA256 fields in Release; ship both `Release` +
  detached `Release.gpg` and clearsigned `InRelease`.
- rpm: one repo per project (yum has no components). Decide repodata
  placement relative to arch dirs however createrepo_c makes cleanest;
  the `.repo` file is generated from a template and must set
  `repo_gpgcheck=1`, `gpgcheck=0` (package-level signing is a later phase),
  `gpgkey=https://pkgs.meigma.dev/meigma.asc`.

## User-facing install contract (must match docs you write)

```sh
# Debian/Ubuntu
curl -fsSL https://pkgs.meigma.dev/meigma.asc | sudo tee /etc/apt/keyrings/meigma.asc >/dev/null
sudo tee /etc/apt/sources.list.d/meigma.sources <<'EOF'
Types: deb
URIs: https://pkgs.meigma.dev/apt
Suites: stable
Components: incus-gh-runner
Signed-By: /etc/apt/keyrings/meigma.asc
EOF
sudo apt update && sudo apt install incus-gh-runner

# Fedora/RHEL-family
sudo dnf config-manager addrepo --from-repofile=https://pkgs.meigma.dev/rpm/incus-gh-runner/meigma.repo
sudo dnf install incus-gh-runner
```

## Repository deliverables

1. **`publish.yml`** — the workflow above. Triggers: `repository_dispatch`
   (type `publish-package`) + `workflow_dispatch` with the same inputs for
   manual/re-publish. Validates payload against the project registry before
   touching anything.
2. **`rebuild.yml`** — `workflow_dispatch` only. Rebuilds the full tree from
   scratch by iterating the registry and every qualifying Release per
   project. Same metadata/signing/sync code path as publish (shared scripts —
   do not fork the logic).
3. **`smoke-test.yml`** — scheduled (daily) + after each publish. In Debian
   and Fedora containers: install the key, add the repo, `apt-get update` /
   `dnf makecache` with GPG checking enforced, install the newest package.
   Fails loudly (issue creation or repo failure notification) — a signing or
   metadata regression must not be silent.
4. **`projects.yml` (registry)** — single config file listing onboarded
   projects: repo name, asset patterns for deb/rpm, retention override.
   Publish/rebuild read only from this registry; unknown dispatch payloads
   are rejected.
5. **Scripts** (`scripts/`) — the metadata build + sign + sync logic, written
   to run identically in CI and locally (dry-run mode against a local dir
   instead of R2). Bash or Go, your call; favor testability. Shellcheck if
   bash.
6. **Docs** — README (what this repo is, architecture sketch, how publishing
   works, disaster recovery = run rebuild.yml), `docs/onboarding.md` (the
   exact contract a meigma project adds: nfpm expectations + the ~10-line
   dispatch step for its release workflow, copy-pasteable), and
   `docs/install.md` (user-facing snippets above, per project).
7. **CI for the repo itself** — lint workflows/scripts; a PR-level test that
   runs the full publish pipeline against a fixture .deb/.rpm into a temp
   directory (no R2, no real key — a throwaway test key) and asserts the
   metadata verifies with `gpgv` / `apt-ftparchive`-consistency /
   `createrepo_c --checkts`-style checks.

## Retention

Default: keep newest 5 versions per project per architecture in the repo
tree (older remain on GitHub Releases). Retention applies at publish time;
registry may override per project.

## Adopted defaults (previously open questions — do not re-litigate)

- Single hostname `pkgs.meigma.dev`, paths `/apt` and `/rpm`.
- apt: component-per-project, single `stable` suite.
- Retention N=5.
- v1 signs metadata only; per-package signing (nfpm rpm signatures, debsig)
  is a documented follow-up, not in scope.
- Dispatch auth: fine-grained PAT (`MEIGMA_PACKAGES_DISPATCH` secret in each
  project repo, scoped to repository_dispatch on meigma/packages). Note the
  GitHub App upgrade path in the README; do not build it now.
- Smoke test ships in v1.

## Escalate to Josh (do not decide unilaterally)

- Anything requiring new spend or a new external account.
- Changing the URL layout after first publish (it's a user-facing contract).
- Key algorithm/structure choices beyond "ed25519 primary offline,
  signing-only subkey in CI" if some tool in the chain can't handle ed25519
  (older RPM stacks can be picky — verify early; RSA-4096 fallback is
  pre-approved if you hit real incompatibility, but say so).
- Any deviation from the invariants section.

## External provisioning (Josh does these; write the checklist into the README and issue precise asks)

1. Create R2 bucket `meigma-packages`; attach custom domain `pkgs.meigma.dev`.
2. Create scoped R2 API token (object read/write, that bucket only) →
   `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` (+ account ID) as Actions
   secrets, environment-scoped.
3. Generate the GPG keypair per the signing design (you provide the exact
   commands + subkey export steps in a runbook doc; assume Josh runs them
   locally and stores the primary offline) → `GPG_SIGNING_SUBKEY` +
   `GPG_PASSPHRASE` secrets.
4. Add the dispatch PAT to consumer repos.

Sequence your work so everything buildable-and-testable-without-secrets lands
first (scripts, fixtures, CI, docs); the secrets-dependent end-to-end run is
the last mile.

## Acceptance criteria

- PR-level pipeline test passes with fixture packages and a throwaway key.
- After provisioning: a staged end-to-end publish (use a `staging/` bucket
  prefix or separate staging bucket — your call) followed by real
  `apt install` and `dnf install` of incus-gh-runner in clean Debian stable,
  Ubuntu LTS, and Fedora containers, with GPG verification enforced, no
  `[trusted=yes]`, no `--nogpgcheck`.
- Re-running publish for the same tag is a no-op (bucket state converges,
  workflow green).
- `rebuild.yml` from an emptied staging prefix reproduces the tree.
- Onboarding doc validated by actually wiring incus-gh-runner's release
  workflow (that change lands in the incus-gh-runner repo — prepare it as a
  ready-to-apply patch/PR).

## Non-goals

Private packages; Windows/macOS packaging; distro-native channels (COPR/PPA/
OBS — possible later, different architecture); package-level signatures (v2);
GitHub App dispatch auth (documented upgrade path only).

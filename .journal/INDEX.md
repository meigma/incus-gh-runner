# Session Journal

| ID  | Date       | Title | Status | Summary |
|-----|------------|-------|--------|---------|
| 001 | 2026-07-17 | Incus runner v1 design | complete | Defined the controller, optional reference image, and evidence-based implementation plan for the full v1 slice. |
| 002 | 2026-07-17 | Establish repository foundation | complete | Renamed and secured the repository foundation, pinned upstream client adapters, and landed phase 0 through PR #7. |
| 003 | 2026-07-17 | Continue phase 1 controller core | complete | Delivered the typed, signal-aware controller core and fake-demand convergence proof through PR #8. |
| 004 | 2026-07-17 | Continue phase 2 guest image work | complete | Landed the reproducible reference VM, one-shot guest contract, hosted offline proof, and live Incus validator through PR #9. |
| 005 | 2026-07-17 | Continue phase 3 Incus lifecycle | complete | Landed the ownership-scoped real Incus lifecycle, periodic inventory, restart safety, and disposable live-test harness through PR #10. |
| 006 | 2026-07-17 | Continue phase 4 GitHub lifecycle | complete | Landed the real GitHub scale-set/JIT lifecycle and proved one genuine job through a disposable Incus 7 VM with exact cleanup. |
| 007 | 2026-07-17 | Package hosting proposal | in-progress | Draft and review the meigma-wide signed apt/yum package hosting proposal (Cloudflare R2 + pkgs.meigma.dev) for incus-gh-runner and future projects. |
| 008 | 2026-07-18 | Continue phase 5 hot pool recovery | complete | Proved live hot-standby replacement and restart safety while carrying bounded concurrent and edge-state live gates forward. |
| 009 | 2026-07-18 | Continue phase 6 service hardening | complete | Landed and proved predictable systemd, timeout, outage, signal, credential, ownership, and operation-retry behavior. |
| 010 | 2026-07-18 | Continue phase 7 release readiness | complete | Landed and proved release-ready controller and reference-image automation while deliberately leaving `v1.0.0` unpublished. |
| 011 | 2026-07-18 | Pre-release language cleanup, docs overhaul, and licensing | complete | Removed development-process language, shipped the operator Diátaxis docs set, rewrote the README, and dual-licensed the repo under Apache-2.0/MIT. |
| 012 | 2026-07-18 | Support GitHub App and PAT authentication | in-progress | Make GitHub App and repository-scoped PAT authentication clean production deployment options. |
| 013 | 2026-07-18 | Plan SLSA security remediation | in-progress | Draft a reviewable plan to address every controller, runner-image, release, repository, and Incus security finding. |
| 014 | 2026-07-20 | Review builder attestation architecture | complete | Produced the reviewed job-bound machine-proof design and five-phase plan for file-backed or TPM-bound systemd credentials. |
| 015 | 2026-07-20 | Implement job machine proof phase 1 | complete | Implemented and merged the locally verifiable job machine proof format, signer, verifier, configuration, and documentation through PR #36. |
| 016 | 2026-07-20 | Assess bootc image migration | complete | Proved bootc feasible on x86_64/KVM, rejected the migration, and retained the distrobuilder-based Ubuntu reference-image plan. |
| 017 | 2026-07-20 | Implement job machine proof phase 2 | complete | Landed and proved the ownership-fenced host-to-VM proof channel through PR #37 on Incus 7.2. |
| 018 | 2026-07-20 | Implement job machine proof phase 3 | complete | Landed GitHub job/JIT-to-VM correlation and supervised proof signing and delivery through PR #38, with the Phase 4 live proof deferred. |
| 019 | 2026-07-20 | Implement job machine proof phase 4 | complete | Landed and proved genuine file-backed machine-proof consumption through PRs #39 and #40, including live contract corrections and fail-closed evidence. |
| 020 | 2026-07-20 | Implement job machine proof phase 5 | complete | Landed and proved TPM-bound job-proof key storage through PR #41 on the upgraded Hetzner host. |
| 021 | 2026-07-21 | Start new work session | in-progress | Opened a fresh session and awaited the substantive work request. |
| 022 | 2026-07-21 | Remove bash test-script debt | complete | Removed all non-product bash test/validation scripts and live harnesses with their CI tasks and docs references through PR #44. |
| 023 | 2026-07-21 | Remove the reference image surface | complete | Removed the reference image, distrobuilder toolchain, and their CI/docs surface through PR #45, keeping `guest/` as product code and adding a single image-hardening how-to. |

---
id: 020
title: Implement job machine proof phase 5
started: 2026-07-20
---

## 2026-07-20 19:56 — Kickoff
Goal for the session: Review Session 014's job machine-proof design and plan, then begin Phase 5 TPM-bound credential validation.
Current state of the world: Phases 1 through 4 are merged on `master` at `e1d5979`; genuine file-backed proof consumption is complete, and Phase 5 remains the bounded validation of the same PKCS#8 Ed25519 key through systemd-250+ TPM-bound credential storage.
Plan: Re-read the Session 014 design and plan, inspect the current deployment and test surfaces, then implement the smallest evidence-producing Phase 5 slice in an isolated Worktrunk branch.

## 2026-07-20 20:06 — Local TPM credential slice
Reviewed Session 014's design and five-phase plan against the Phase 4 implementation. The bounded Phase 5 contract remains correct: use the same PKCS#8 Ed25519 key and runtime path, change only systemd storage, retain `PrivateDevices=yes`, and make no TPM-native signing or measured-boot claim. The three pinned Go modules remain their latest published versions.

Created `feat/job-proof-phase-5-tpm` from fetched `origin/master` and committed `143b940` (`feat(provenance): add TPM-bound proof key storage`). The slice adds the `LoadCredentialEncrypted=` drop-in, exercises all four GitHub App/PAT with file/TPM proof-key combinations, adds installed-host ownership/mode/presence checks, and documents encryption, empty-PCR policy, origin and cross-host checks, rotation, escrow, replacement, reboot, and external proof verification.

Verification passed: Ubuntu 24.04 sandbox matrix; installed file and TPM verifier modes; full serial `root:check`; and explicit docs build. The first parallel full check hit a stale golangci-lint cache referencing a deleted Worktrunk path plus a concurrently killed isolation fixture; cleaning the linter cache and rerunning the affected checks serially passed. The live TPM host reboot, genuine proof, rotation, and optional second-host binding gates remain open.

## 2026-07-20 20:07 — Draft review gate
Pushed `feat/job-proof-phase-5-tpm` and opened draft PR #41: https://github.com/meigma/incus-gh-runner/pull/41. The PR explicitly remains draft until the enrolled-TPM host, reboot, genuine proof, rotation, and optional second-host binding evidence is complete.

## 2026-07-20 20:09 — Hosted checks
Confirmed draft PR #41 at exact head `143b9409ac9ec95c2341510d2c00ae5b4a36ff1f`. Hosted CI, CodeQL for Go and Actions, GitHub Pages, and Kusari Inspector passed; release dry-run jobs skipped by the draft/non-release path as expected.

## 2026-07-20 21:57 — Hetzner TPM experiment
The maintainer supplied `sre@ci` as the preferred bare-metal TPM target. Read-only preflight found Ubuntu 22.04, TPM 2.0 at `/dev/tpm0` and `/dev/tpmrm0`, passwordless sudo, Incus 7.0.1, an inactive/uninstalled `incus-gh-runner.service`, and no existing proof credential. The blocking fact is systemd 249: `systemd-creds` is absent, the installed PID 1 lacks `LoadCredentialEncrypted=`, and apt offers no newer systemd on this release.

Used Docker only for a disposable physical-TPM experiment without mounting host files. Ubuntu 24.04 systemd 255 reported `+TPM2` but initially failed because the optional TSS2 runtime was absent; installing `tpm2-tools` supplied it. The real TPM then encrypted and decrypted the same temporary PKCS#8 Ed25519 key with `--with-key=tpm2 --tpm2-device=/dev/tpmrm0 --tpm2-pcrs=`, mode `0600`, and the derived public keys matched. A nested systemd-container service also reached successful TPM unsealing with the packaged drop-in, but Docker's mount namespace hid the generated `/run/credentials/...` path, so this is not service or reboot acceptance evidence. Removed the exact test container, derived image, and pulled Ubuntu test image; the pre-existing Earthly containers were unchanged.

Updated the operator docs with the discovered TSS2 runtime prerequisite and pushed commit `4d8d345` to draft PR #41. The full Phase 5 gate still requires bare-metal systemd 250+; either `ci` must receive an explicitly approved Ubuntu 24.04 upgrade or another compatible TPM host must be selected.

## 2026-07-20 21:59 — Experiment follow-up checks
Updated draft PR #41 with the Hetzner TPM evidence and exact remaining gate. Confirmed hosted CI, CodeQL for Go and Actions, GitHub Pages, and Kusari Inspector pass at head `4d8d345ff3496457b9f7404fa1f8bb18be5ae2b4`; release dry-run jobs remain skipped as expected.

## 2026-07-21 13:12 — Bare-metal PID 1 acceptance
After the maintainer upgraded `sre@ci`, re-fingerprinted the host as Ubuntu 26.04 with systemd 259, systemd-creds present, TPM 2.0, Incus 7.0.1, no runner instances, and no installed runner controller or credentials. The first direct encryption failed with systemd's generic `AES-128-CFB missing?` message. Debug logging identified the actual cause: `libtss2-esys.so.0` loaded but `libtss2-rc.so.0` was absent. Removed the temporary private/public key artifacts; no encrypted credential had been created.

Installed the documented `tpm2-tools` dependency, which supplied the missing TSS2 runtime. A fresh disposable Ed25519 key then sealed and unsealed through the physical TPM using the required empty PCR set, and its derived public keys matched. Removed both plaintext copies immediately after the check.

Created a runtime systemd probe and installed the repository's exact `credentials-job-proof-tpm.conf` as its drop-in. PID 1 expanded `%d`, loaded the encrypted credential, exposed a readable PKCS#8 key, reproduced the enrolled public key, and reported `PrivateDevices=yes`, `Result=success`, and `ExecMainStatus=0`. The encrypted disposable test credential and public key remain only to support the pending reboot-and-retry gate; the runtime unit itself will disappear on reboot. No controller or GitHub credential was installed.

Generalized the Ubuntu TSS2 dependency guidance and documented how to distinguish the misleading AES error from a missing library. Docs build passed and commit `98d3015` (`docs(provenance): clarify TPM library failures`) was pushed to draft PR #41. Remaining gates are reboot-and-retry, a genuine GitHub job proof, rotation, and optional second-host binding.

## 2026-07-21 13:16 — TPM credential rotation
Rotated the disposable proof key in place: generated a distinct Ed25519 key under root-only `/run`, sealed it to the same physical TPM and empty PCR set, decrypted it once to verify the enrolled public key, atomically replaced the encrypted credential, and restarted the PID 1 probe. The restarted service reproduced the new public key and the new key was confirmed distinct from the previous enrollment. Removed every plaintext key/check file and the superseded public key. Only the current encrypted credential (`root:root` `0600`) and public key (`root:root` `0644`) remain persistently for the reboot gate. Storage rotation is now accepted; external consumer overlap still belongs to the genuine-proof rollout.

## 2026-07-21 13:22 — Reboot acceptance
With explicit maintainer approval, rebooted `sre@ci`. The boot ID changed from `1c04e845-f99a-4920-85e3-617db59af4a5` to `6962f057-43b4-40af-9ddc-bfd8c68d9aa2`; Incus returned active with no runner instances, the complete TSS2 runtime remained available, the runtime probe correctly disappeared, and the encrypted credential plus enrolled public key persisted with their expected ownership and modes.

Recreated the runtime service and installed the repository's exact TPM proof-key drop-in. On the fresh PID 1, the service again expanded `%d`, unsealed the TPM credential, reproduced the enrolled public key, retained `PrivateDevices=yes`, and completed with `Result=success` and `ExecMainStatus=0`. Reboot-and-retry is accepted.

Stopped and removed the runtime service, encrypted disposable credential, public key, and derived public output. No test enrollment or plaintext key remains on the host. Kept `tpm2-tools` and its TSS2 runtime dependencies because they are the documented prerequisite for the eventual production enrollment. The remaining required Phase 5 gate is a genuine externally verified GitHub job proof; second-host binding remains optional.

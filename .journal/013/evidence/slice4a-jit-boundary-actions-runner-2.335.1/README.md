# Slice 4A JIT-boundary spike

## Scope

This is retained, secret-free evidence from the required stock Actions Runner
boundary spike. The probe source was temporary and does not appear in PR #34.
No encoded JIT configuration, materialized runner credential, raw runner
diagnostic log, PAT, or private key is retained here.

## Exact inputs

- Product base after Slice 3D: `e2138adea423b48ee798e9f909d780895dbbefa3`
- Temporary workflow head: `8e2e38b6b5ee41e8cb1725f964b856847e734227`
- Final product head: `8351cd84e610ff8adc3c240754392e46b803128f`
- Actions Runner: `2.335.1`
- Exact upstream tag commit inspected: `7d737449ef346f6524f75688d0c9c95fa10ba10a`
- Reference image workflow run: `29708275005`
- Reference image fingerprint and archive SHA-256:
  `d31a6f9cbdfbf48c31843ead51eb2365e0ccdf222b8bdb2b7faa92145550ad64`
- Controller binary SHA-256:
  `00901a2d21c5bab13de92dba9412b2fe56274bbcafa4355737985b677682dece`
- Live workflow run: <https://github.com/meigma/incus-gh-runner/actions/runs/29754484647>
- Disposable host: Latitude `sv_y9815XYwxavEk`, Ubuntu 24.04, Incus 7.0.1,
  KVM available

## Source inspection result

Stock `Runner.Listener` decodes `--jitconfig`, writes the supplied runner files
into the runner root, and launches `Runner.Worker` with an ordinary child
process. The process launcher has no supported username-switch setting. The
listener, worker, and job therefore share the `actions-runner` UID. A clean UID
split would require a privileged wrapper or maintained fork, so this spike did
not propose that architecture.

## Live result

The adversarial job completed successfully and emitted only these safe facts:

```text
listener_worker_same_uid=true
jit_command_line_readable=true
jit_files_readable=true
renewable_controller_credential_absent=true
```

The initial detached replay classifier incorrectly called exit code `0`
"accepted." A bounded manual replay then resolved the ambiguity without
exporting the raw log or credential:

```text
manual_exit=0
manual_elapsed=3
connected=true
listening=false
registration_deleted=true
session_conflict=false
```

Captured JIT material can therefore authenticate far enough to report
`Connected to GitHub` after the job, but GitHub immediately reports that the
runner registration was deleted and the replay never reaches `Listening for
Jobs`. It cannot accept a second job. The correct boundary is that the job can
read and interfere with its own live JIT/session material, while the renewable
controller credential stays on the host and the VM is destroyed before any
cross-job reuse.

## Cleanup

- Stopped the controller and deleted its temporary PAT before each job ran.
- Deleted runner scale set ID `4` with the upstream scale-set client.
- Deleted the disposable Incus instances, project, image, and transferred
  material by destroying the exact paid host.
- Destroyed exact Latitude server `sv_y9815XYwxavEk`; exact-ID lookup returns
  `404 NotFound` and the provider project server list is empty.
- Removed the temporary workflow probe from the final branch.

## Product consequence

PR #34 documents the same-UID JIT boundary and corrects the reference image's
false offline/reproducible wording. The image build is networked and
non-hermetic: the runner archive and `distrobuilder` are pinned, but Ubuntu
repository snapshots and the complete resolved package set are not yet pinned
or recorded. Final-rootfs SBOMs, vulnerability scans, runner freshness policy,
and stronger provenance remain later Slice 4 work.

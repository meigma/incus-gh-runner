# Slice 3A agent-outage proof — Incus 7.0.1

Result: **pass**.

## Reviewed inputs

- PR: <https://github.com/meigma/incus-gh-runner/pull/30>
- exact commit: `fa729feb8661de8ad85f008bfaae4954e2a022f0`
- exact-head reference-image workflow:
  <https://github.com/meigma/incus-gh-runner/actions/runs/29706057178>
- image fingerprint/SHA-256:
  `a420b1d0a88bad98f79533baf36ce0c17e4e7f848b4604747ea94d0a6fac92e7`
- controller SHA-256:
  `773a19f0f472bb086c614bb3ecbdd919d5dcc81053efeb2faa9d777472167617`
- runtime config SHA-256:
  `04c99f9e46bc16332c4b4f8f4fe1375b06a9cedfa68bb834032a9f1f69ae8fb7`
- Incus client/server: `7.0.1` / `7.0.1`
- disposable Latitude host: `sv_dexA0qAO80lQV`, MEX2,
  `c3-small-x86`, Ubuntu 24.04

The controller version output named the exact commit. The image archive and
controller binary checksums were verified before use. The GitHub token was
delivered only through the systemd manager environment, never written to disk,
and verified absent before teardown.

## Live acceptance

Two held GitHub jobs were active on two owned VMs whose creation metadata had
been backdated beyond the five-minute bootstrap timeout. Both guest
`status.json` documents reported `running` before the outage:

- [run 29706713014](https://github.com/meigma/incus-gh-runner/actions/runs/29706713014),
  exact head `fa729feb8661de8ad85f008bfaae4954e2a022f0`, success,
  job interval `2026-07-19T22:45:54Z`–`2026-07-19T22:48:57Z`;
- [run 29706716011](https://github.com/meigma/incus-gh-runner/actions/runs/29706716011),
  exact head `fa729feb8661de8ad85f008bfaae4954e2a022f0`, success,
  job interval `2026-07-19T22:46:10Z`–`2026-07-19T22:49:14Z`.

After both Incus guest agents were unavailable, ten samples spanning 46
seconds (`22:47:58Z`–`22:48:44Z`) retained the same observations every time:

| Runner | State | QEMU PID | Guest status |
| --- | --- | ---: | --- |
| `incus-gh-runner-99c2acc8-dd07-4478-971d-091643d486f2` | `Running` | `546449` | unavailable |
| `incus-gh-runner-f24cd24d-a3ed-4ba5-bc26-395ae381c27e` | `Running` | `546740` | unavailable |

The controller emitted whole-inventory failures, including one read failure
for the later runner and then failures for both runners. It emitted no owned
runner delete while either job was active. Both workflows completed
successfully. Once both VMs became `Stopped`, a fresh inventory succeeded and
the controller deleted the two owned VMs at `22:50:17Z`; the unowned sentinel
remained present. Final owned inventory was zero.

The temporary harness initially checked one asynchronous guest-agent shutdown
too early and stopped its orchestration. This was a harness timing error, not a
production assertion failure; the same two jobs and VMs were retained for the
46-second proof rather than dispatching replacements.

## Adjacent restart observation

A controller cold-started while inventory was already uncertain exited
fail-closed, after which the supplied systemd restart policy reached its default
start limit after five retries. Neither active job was stopped or deleted, so
the Slice 3A acceptance condition still passed. Record this as input to Slice
3B's explicit restart work: recovery after a cold-start inventory outage should
not require an operator to reset systemd's start-limit failure.

## Teardown

At `2026-07-19T22:52:29Z`, the exact owner inventory was zero, the sentinel,
image, and `runner-test` project had been deleted, and the GitHub credential was
absent. Latitude server `sv_dexA0qAO80lQV` was destroyed; the exact-ID lookup
then returned provider `404 NotFound`, and the project server list was empty.
The approximately 19-minute lifetime cost about `$0.16` at `$0.52/hour`, before
provider minimums or rounding.

The complete ignored local archive is
`build/slice3a-live-evidence/20260719-sv_dexA0qAO80lQV/`; its manifest verified:

```text
138e667a7c333fd4470238f27f1aea9d2a559126de3d4d70b25359a14bf8ed7f  README.md
bb09eafaf96751ddaeed78f0119639c982a27ff15348124a0a9a25f3e7ad0f87  inventory-before-outage.json
95150c1d16972b7a790b0feb92311c84cb1a3df633e7a74b2288ccbbb6f1eb36  outage-samples.tsv
1ee5010f471c80aae017a430450c850f505ca7b8ed5944e1584727f48b28c9ae  workflows-before-outage.json
```

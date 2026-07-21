---
id: 022
title: Remove bash test-script debt
date: 2026-07-21
status: complete
repos_touched: [incus-gh-runner]
related_sessions: [011]
---

## Goal

Identify every test/validation bash script that landed as a permanent repo
fixture, then — per explicit maintainer decision — remove all of them except
product code, along with their CI wiring and docs references.

## Outcome

The goal was met. The survey found ten tracked `.sh` files plus two
extensionless guest scripts; none had ever been deleted in repo history. The
maintainer kept only product code (`image/guest/incus-gh-runner-{guest,proof}`,
`image/build.sh`, `image/validate-incus.sh`) and directed removal of the rest.
PR #44 squash-merged as `635097d`, deleting 2,288 lines of bash and every
reference: eight scripts, the `cases.cue` CUE test inputs, three moon CI
tasks, three moon file groups, and the deploy/README/CUE-README docs passages
built around them. `moon run root:check` and all hosted checks passed before
and after merge.

## Key Decisions

- Remove rather than rewrite or replace -> the maintainer explicitly declined
  long-term maintenance; the dropped coverage is recorded in the PR body, not
  substituted with new test code.
- Delete `deploy/incus/cue/tests/cases.cue` along with `render-test.sh` -> its
  only consumer was the removed script; the shipped CUE module (schema plus
  examples) stays intact.
- Keep all seven `deploy/systemd/` install artifacts -> `verify.sh` only
  validated them; deploy.md consumes them directly.
- Renumber deploy.md step 8 to 7 after deleting "Validate the unit" -> no
  other page or anchor referenced the old numbering, so the flow stays
  coherent without redirects.

## Changes

- Deleted `scripts/live/` (four live harnesses, including the 808-line
  hostile-isolation harness), `image/tests/` (two contract tests),
  `deploy/incus/cue/tests/`, and `deploy/systemd/verify.sh`.
- `moon.yml` - removed tasks `image-contract-test`,
  `incus-isolation-contract-test`, and `systemd-unit-test`, their `check`
  deps, and the unconsumed `imageSources`/`systemdSources`/
  `incusIsolationSources` file groups.
- `docs/docs/how-to/deploy.md` - removed the hostile-harness preflight block,
  both `verify.sh --installed-job-proof` steps, and the "Validate the unit"
  section; renumbered the final step.
- `README.md` and `deploy/incus/cue/README.md` - removed harness and
  contract-test mentions.

## Open Threads

- Coverage intentionally dropped without replacement: CUE golden/negative
  render tests, systemd unit verification (`systemd-analyze` sandbox and
  security threshold), guest entrypoint/proof-helper contract tests, and the
  live isolation and hot-standby harnesses. Reintroduction would be a new
  maintainer decision, likely as tested Go code rather than bash.
- The docs still document the security *baseline* (`deploy/incus/`), but
  operators no longer have a scripted way to verify live isolation behavior.

## Lessons

- The debt pattern to avoid: each phase-proof session's acceptance harness
  landed in-tree, and CI "contract tests" then asserted on the harnesses'
  source text (grep/sed of quoted lines), coupling CI to byte-level script
  content. Live acceptance tooling should stay in session evidence unless it
  is deliberately adopted as maintained product surface.

## References

- [PR #44](https://github.com/meigma/incus-gh-runner/pull/44), merge commit
  `635097d821fb574d5634aa7ee30dbded503a559f`.
- Survey details and the full reference map: `.journal/022/NOTES.md`.
- `.journal/011/SUMMARY.md` - the session that made the live harnesses
  product-facing.

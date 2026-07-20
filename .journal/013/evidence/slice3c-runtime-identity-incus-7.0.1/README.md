# Slice 3C runtime-identity proof — Incus 7.0.1

Date: 2026-07-20 UTC

## Exact targets

- Pull request: #32
- Controller commit: `ad05ad041ebc17077bab3f2c856862bb1e3c11a0`
- Controller build identity: `incus-gh-runner ad05ad0
  (ad05ad041ebc17077bab3f2c856862bb1e3c11a0) built
  2026-07-20T05:01:29Z`
- Controller binary SHA-256:
  `5d1e6bf5d6a08aeacee36eaeda8f986e27fb48299069b8a2b9d1e13d093f52fb`
- Incus client/server: 7.0.1 / 7.0.1
- Reference image workflow: 29708275005 (image inputs unchanged by Slice 3C)
- Reference image artifact SHA-256 and imported fingerprint:
  `d31a6f9cbdfbf48c31843ead51eb2365e0ccdf222b8bdb2b7faa92145550ad64`
- Alternate valid VM image fingerprint:
  `180ee23f6981ca3a0ea9a342036ef35bceca27495bd4c0a3e311f066c00b87fe`
- Disposable Latitude server: `sv_BDXM5Ekjz0rpk`, MEX2,
  `c3-small-x86`, Ubuntu 24.04

## Probe

A one-use Go test was compiled from the exact PR source, transferred with
SHA-256 verification, and then deleted from the worktree before execution. No
probe or acceptance-framework source remains in the product branch.

The probe performed these operations against project `runner-test`:

1. Preflighted alias `runner-image` and profile `runner`.
2. Changed `limits.cpu` from `2` to `6`; create failed closed with
   `runner profile "runner" changed after preflight` and created no instance.
3. Restored the exact profile state.
4. Retargeted `runner-image` from the approved reference fingerprint to the
   alternate valid VM fingerprint.
5. Created one runner through the already-preflighted backend.
6. Changed the source profile again to CPU `8` and added a drift marker.
7. Re-read the live instance, then deleted it through the production backend.

The exact instance evidence logged by the passing test was:

```json
{"base_image":"d31a6f9cbdfbf48c31843ead51eb2365e0ccdf222b8bdb2b7faa92145550ad64","configured_image":"runner-image","expanded_cpu":"2","pinned_fingerprint":"d31a6f9cbdfbf48c31843ead51eb2365e0ccdf222b8bdb2b7faa92145550ad64","profile_audit":[{"name":"runner","sha256":"4d810cf40b1afef74f693b6a1637cbdc042e2ffed8b3bd637f169d52bf21968d"}],"profiles":[],"retargeted_fingerprint":"180ee23f6981ca3a0ea9a342036ef35bceca27495bd4c0a3e311f066c00b87fe","runner":"incus-gh-runner-slice3c-live"}
```

The test passed in 29.54 seconds. This proves that a post-preflight alias
retarget cannot change the launched image, a changed profile blocks create,
and post-create profile changes do not alter a VM carrying no attached
profiles. The same execution exercised the stable-UUID checks, ETag-conditional
stop, final identity fetch, and deletion; owned inventory returned to zero.

## Cleanup

The probe restored the original alias and profile state. Final project
inventory was empty. Removed both test images, the test profile and project,
the default-project alternate image, and all transferred artifacts. Destroyed
exact provider server `sv_BDXM5Ekjz0rpk`; its exact-ID lookup returned
`404 NotFound`, and the provider project server list was empty. The host existed
for about 11 minutes, approximately `$0.10` at the quoted `$0.52/hour` before
provider rounding.

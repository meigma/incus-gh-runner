# Changelog

## [1.2.0](https://github.com/meigma/incus-gh-runner/compare/v1.1.0...v1.2.0) (2026-07-24)


### Features

* **incus:** support additional controlled egress ([#50](https://github.com/meigma/incus-gh-runner/issues/50)) ([63ae36b](https://github.com/meigma/incus-gh-runner/commit/63ae36b8132d1eefffc71399dee929901baae7a9))

## [1.1.0](https://github.com/meigma/incus-gh-runner/compare/v1.0.0...v1.1.0) (2026-07-23)


### Features

* **incus:** support LVM isolation baselines ([#49](https://github.com/meigma/incus-gh-runner/issues/49)) ([c41f108](https://github.com/meigma/incus-gh-runner/commit/c41f10841170c5f066c6d2667c07c72a00a26379))
* **release:** dispatch package publication ([#47](https://github.com/meigma/incus-gh-runner/issues/47)) ([dfd1e29](https://github.com/meigma/incus-gh-runner/commit/dfd1e299fe06649edc65f352d1790c12b0849690))

## 1.0.0 (2026-07-22)


### Features

* **auth:** support production PAT credentials ([#26](https://github.com/meigma/incus-gh-runner/issues/26)) ([f493f93](https://github.com/meigma/incus-gh-runner/commit/f493f93f6a11403ccd9af12e55c58e3b2caf7eaf))
* **controller:** prove phase 1 controller core ([#8](https://github.com/meigma/incus-gh-runner/issues/8)) ([9bd37f7](https://github.com/meigma/incus-gh-runner/commit/9bd37f7d62cd2dc43c00986f24b9166c01f2b638))
* **github:** integrate runner scale-set lifecycle ([#11](https://github.com/meigma/incus-gh-runner/issues/11)) ([e778ef1](https://github.com/meigma/incus-gh-runner/commit/e778ef121a1a2c4b13320d591530275b8526b29a))
* **github:** recover failed message sessions ([#16](https://github.com/meigma/incus-gh-runner/issues/16)) ([a76994b](https://github.com/meigma/incus-gh-runner/commit/a76994ba70303939285d4f868bca9e4bf37028e9))
* **image:** prove one-shot guest contract ([#9](https://github.com/meigma/incus-gh-runner/issues/9)) ([85f273a](https://github.com/meigma/incus-gh-runner/commit/85f273a15ae3a3026eab932663c22d1dc5026835))
* **incus:** implement owned runner lifecycle ([#10](https://github.com/meigma/incus-gh-runner/issues/10)) ([d03cace](https://github.com/meigma/incus-gh-runner/commit/d03cace7bbde85c7365c13fda541c87243daddfc))
* **provenance:** add file-backed live proof gate ([#39](https://github.com/meigma/incus-gh-runner/issues/39)) ([173381f](https://github.com/meigma/incus-gh-runner/commit/173381ff18c220177fd75bda2fb920e38a757990))
* **provenance:** add job machine proof primitives ([#36](https://github.com/meigma/incus-gh-runner/issues/36)) ([ce7c89c](https://github.com/meigma/incus-gh-runner/commit/ce7c89c920ac16cf0422bb8e554498b0549524cf))
* **provenance:** add TPM-bound proof key storage ([#41](https://github.com/meigma/incus-gh-runner/issues/41)) ([4d891a8](https://github.com/meigma/incus-gh-runner/commit/4d891a8ed09df53ec1fd4eb059cf551500dbfac6))
* **provenance:** bind authenticated jobs to Incus machines ([#38](https://github.com/meigma/incus-gh-runner/issues/38)) ([c32e134](https://github.com/meigma/incus-gh-runner/commit/c32e134a3cbba57e3aaea1add095fec356d8bb13))
* **provenance:** deliver proofs to runner guests ([#37](https://github.com/meigma/incus-gh-runner/issues/37)) ([ea7e504](https://github.com/meigma/incus-gh-runner/commit/ea7e504087fd7e5d7b49782a5093fc9c48021e79))
* **release:** add native Linux packages ([#46](https://github.com/meigma/incus-gh-runner/issues/46)) ([e69fd1c](https://github.com/meigma/incus-gh-runner/commit/e69fd1c2c2d1e9175b9a3022330eff2c72bbdb2c))
* **release:** publish reference VM image ([#20](https://github.com/meigma/incus-gh-runner/issues/20)) ([1943892](https://github.com/meigma/incus-gh-runner/commit/194389274353c27077304b5a673f49f7a972504e))
* replace the reference VM image with a hardened-image guide ([#45](https://github.com/meigma/incus-gh-runner/issues/45)) ([0b7a4b6](https://github.com/meigma/incus-gh-runner/commit/0b7a4b6f7d6dbb3114f34023254cd36a9c10d242))
* **security:** add Incus isolation baseline ([#28](https://github.com/meigma/incus-gh-runner/issues/28)) ([10f4be2](https://github.com/meigma/incus-gh-runner/commit/10f4be261419f0c36761a0e99b6e5c84ebf1dbad))
* **systemd:** add hardened service deployment ([#18](https://github.com/meigma/incus-gh-runner/issues/18)) ([956a34a](https://github.com/meigma/incus-gh-runner/commit/956a34a8e73fb011d9431ef90a54cac456ad88da))


### Bug Fixes

* **app:** bound component shutdown ([#17](https://github.com/meigma/incus-gh-runner/issues/17)) ([439ca19](https://github.com/meigma/incus-gh-runner/commit/439ca1977860fdc6f7d26c74fedd5ee4eee292b2))
* **ci:** gate unavailable repository services ([#21](https://github.com/meigma/incus-gh-runner/issues/21)) ([82f5ed4](https://github.com/meigma/incus-gh-runner/commit/82f5ed40b0fbacd16960bc98aa96a0b7e9788d42))
* **controller:** back off failed Incus operations ([#19](https://github.com/meigma/incus-gh-runner/issues/19)) ([4979f7d](https://github.com/meigma/incus-gh-runner/commit/4979f7df525a5619a0d08a4257033ee392c7e554))
* **controller:** fail closed on inventory uncertainty ([#30](https://github.com/meigma/incus-gh-runner/issues/30)) ([92cd3ce](https://github.com/meigma/incus-gh-runner/commit/92cd3ce29b6286489969e34f1c7353d1691ab4f0))
* **controller:** fence idle runners before scale-down ([#31](https://github.com/meigma/incus-gh-runner/issues/31)) ([2e72d30](https://github.com/meigma/incus-gh-runner/commit/2e72d30be222edb1cdc10222492aac556fbae007))
* **incus:** boot and clean up Noble runners ([#12](https://github.com/meigma/incus-gh-runner/issues/12)) ([8580997](https://github.com/meigma/incus-gh-runner/commit/8580997c2dbe773f382f3bd4607f1420ed9fe987))
* **incus:** bound runner diagnostics ([#33](https://github.com/meigma/incus-gh-runner/issues/33)) ([e2138ad](https://github.com/meigma/incus-gh-runner/commit/e2138adea423b48ee798e9f909d780895dbbefa3))
* **incus:** pin runner environment identity ([#32](https://github.com/meigma/incus-gh-runner/issues/32)) ([6e11ff1](https://github.com/meigma/incus-gh-runner/commit/6e11ff198d2ab46cd358404f545e545c60641b03))
* **live:** use scoped runner evidence ([#15](https://github.com/meigma/incus-gh-runner/issues/15)) ([56eaf85](https://github.com/meigma/incus-gh-runner/commit/56eaf854b11e4273916099fe2ad596565e9e2a09))
* **provenance:** align proofs with live Incus and GitHub metadata ([#40](https://github.com/meigma/incus-gh-runner/issues/40)) ([e1d5979](https://github.com/meigma/incus-gh-runner/commit/e1d5979b9b9dd944a610496ece42b698c0d16f0f))
* **security:** fail closed on controller inputs ([#27](https://github.com/meigma/incus-gh-runner/issues/27)) ([f04f474](https://github.com/meigma/incus-gh-runner/commit/f04f474510e2fbeff0ed23ed976e0a72e65a720b))
* **security:** harden Incus runner isolation ([#29](https://github.com/meigma/incus-gh-runner/issues/29)) ([9902257](https://github.com/meigma/incus-gh-runner/commit/99022572225f906c0bea56565b091a13cd9e12df))

## Changelog

Notable changes to `incus-gh-runner` will be recorded here by Release Please.

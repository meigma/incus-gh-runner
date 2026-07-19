# Exact-head runtime acceptance evidence

This directory retains the passing combined Incus runtime gate from draft PR
#29. The top-level `checksums.sha256` is the harness-owned checksum manifest;
`runtime-probe/checksums.sha256` independently covers the Go helper evidence.

The final run is bound to:

- source revision `3d787dc1a0aac7a59e34b68e4ebc4f318ee7854f`;
- helper SHA-256 `93fa9da2ced9718f8a4f5fde171f3a091eaaea68cc14359e0aef1a9384930e60`;
- baseline SHA-256 `c2aac4737d94483bf308fa356546c7c50499a0ec51c3aa261397a47126c438d2`;
- image fingerprint `ae1e2b082d50b4f6daf6bdf35561f12b170fd20cacfc09ffcf2a4149c330db1a`;
  and
- run ID `hostile-20260719203752-424722`.

Verify the retained evidence from this directory:

```sh
sha256sum --check checksums.sha256
(cd runtime-probe && sha256sum --check checksums.sha256)
(cd host && sha256sum --check checksums.sha256)
(cd image-artifact && sha256sum --check checksums.sha256)
```

The 666 MiB image archive is intentionally not committed to the journal. Its
workflow receipt, archive checksum, inspected metadata, and qcow2 inspection
are retained in `image-artifact/`. The complete non-image host evidence archive
was copied and independently verified before host destruction.

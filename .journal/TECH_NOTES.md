# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- Add concise, identifier-led Godoc comments to every Go type, function, and method, including unexported declarations and test helpers, so IntelliSense exposes intent during review.
- The v1 controller uses `github.com/actions/scaleset v0.4.0` with `github.com/lxc/incus/v7 v7.2.0`, manages one scale set in a preconfigured Incus environment, and mutates only explicitly owned runner instances. Incus `v7.0.0` failed hosted security analysis with nine unmitigated critical/high CVEs and must not be restored.
- Hot standby is required for v1: target capacity is `min(max_runners, min_runners + TotalAssignedJobs)`, and standby capacity must be JIT-registered, connected, and idle.
- Keep synchronous scale-set callbacks free of Incus I/O. Coalesce demand into a single-owner reconciler and execute Incus work through a bounded worker pool.
- Use one JIT configuration and one job per VM; let the guest power off, collect diagnostics, delete the VM, and reconstruct state from GitHub plus Incus metadata without a v1 database.
- The reusable image is optional. mise installs checksum-pinned distrobuilder 3.3.1 from source on Linux; build the reference VM with `mise exec -- image/build.sh`, then validate boot separately with `image/validate-incus.sh` in an explicitly disposable, non-default Incus project.
- The guest contract writes root-only `/run/incus-gh-runner/payload.json`, creates `payload.ready` only after the payload is complete, deletes both inputs before starting the unprivileged runner, publishes secret-free status and serial-console transitions, and powers off on every terminal path.
- Run the controller as a foreground systemd service with bounded contexts, reconnect backoff, graceful SIGINT/SIGTERM handling, and systemd restart for irrecoverably wedged operations.
- Phases 0 through 2 landed through PRs #7, #8, and #9 at `master` commit `85f273a`: the foundation, controller core, reproducible reference VM, and one-shot guest contract are in place. Real runtime adapters remain intentionally unwired, and the merged image still needs live import/boot validation on an Incus-capable host.
- Start future implementation work from `.journal/001/V1_IMPLEMENTATION_PLAN.md`; use the focused controller and image proposals as the current designs, while treating `TECHNICAL_PROPOSAL.md` as historical umbrella context.

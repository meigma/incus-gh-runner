# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- The v1 controller uses `actions/scaleset` with the Incus v7 Go client, manages one scale set in a preconfigured Incus environment, and mutates only explicitly owned runner instances.
- Hot standby is required for v1: target capacity is `min(max_runners, min_runners + TotalAssignedJobs)`, and standby capacity must be JIT-registered, connected, and idle.
- Keep synchronous scale-set callbacks free of Incus I/O. Coalesce demand into a single-owner reconciler and execute Incus work through a bounded worker pool.
- Use one JIT configuration and one job per VM; let the guest power off, collect diagnostics, delete the VM, and reconstruct state from GitHub plus Incus metadata without a v1 database.
- The reusable image is optional. Build the reference VM offline with `lxc/distrobuilder`, then validate boot and the guest contract separately in an Incus-capable environment.
- Run the controller as a foreground systemd service with bounded contexts, reconnect backoff, graceful SIGINT/SIGTERM handling, and systemd restart for irrecoverably wedged operations.
- Start future implementation work from `.journal/001/V1_IMPLEMENTATION_PLAN.md`; use the focused controller and image proposals as the current designs, while treating `TECHNICAL_PROPOSAL.md` as historical umbrella context.

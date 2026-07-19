# How incus-gh-runner works

incus-gh-runner turns Incus virtual machines into disposable, single-job GitHub Actions runners. It watches a GitHub Actions runner scale set for demand and keeps a population of Incus VMs matched to that demand, creating runners when jobs need them and deleting runners once their job is done. This page explains the mental model behind that behavior: how demand becomes capacity, how a runner moves through its life, why the current controller requires a dedicated Incus host, how it responds to failure, and what its security posture assumes.

It intentionally does not list configuration keys, CLI flags, or wire-level schemas. Those live in the [configuration reference](../reference/configuration.md) and the [guest contract reference](../reference/guest-contract.md).

## Demand flow: from GitHub to Incus

GitHub Actions runner scale sets communicate demand through a long-lived message session: the controller opens a session against `github.config_url` for a given `github.scale_set`, and GitHub pushes job assignment events down that session for as long as it stays open. There is exactly one message session per controller process, feeding exactly one reconciler. incus-gh-runner does not fan out across multiple scale sets or multiple Incus environments — that scope boundary is a deliberate design choice. A single process manages a single scale set against a single Incus project and image, which keeps the cleanup and capacity model simple enough to reason about at a glance.

The reconciler turns that stream of demand into a target VM count using one formula:

```
target = min(max_runners, min_runners + assigned_jobs)
```

`assigned_jobs` is the number of jobs GitHub currently wants this scale set to run. `min_runners` is a hot-standby floor: a number of runners the controller keeps alive and idle even when there is no work queued, so that the first jobs of a burst land on a VM that is already provisioned and connected rather than waiting for one to boot. `max_runners` is a hard ceiling that protects the Incus host from unbounded growth regardless of how much demand GitHub reports. Raising `min_runners` trades idle compute cost for lower job start latency; raising `max_runners` trades host capacity for burst headroom. The formula is the whole story — there is no separate scale-down cooldown or queue-depth heuristic layered on top of it. Every reconcile pass simply recomputes the target and moves the current runner count toward it.

The reconciler itself is event-driven rather than polling GitHub on a fixed cadence: new demand, exact runner-busy observations, completed external operations, and a periodic Incus inventory refresh all feed a single coalescing mailbox that keeps only the newest authoritative GitHub snapshot. A burst of updates therefore collapses into one reconcile decision rather than a queue of stale ones. A configurable number of external operations run concurrently, because creating a VM or fencing its GitHub registration is comparatively slow and the reconciler would otherwise serialize on it.

## The lifecycle of a runner

Every runner is a single Incus VM that exists to run exactly one GitHub Actions job, then disappears. It moves through four states:

- **provisioning** — the VM has been created and started, but has not yet reported that its runner process is live.
- **idle** — the runner is connected to GitHub and waiting for a job (this is what satisfies `min_runners`).
- **busy** — the runner has picked up a job and is executing it.
- **terminal** — the job is done, or the VM failed, or it never came up in time. It is waiting to be deleted.

The one-job-per-VM design is the load-bearing decision here. Because a VM never runs a second job, there is no cleanup step that has to scrub a workspace between jobs, no risk of one job's state leaking into the next, and no need to trust that a long-lived runner stays healthy across dozens of jobs. Reproducibility comes from throwing the VM away, not from disciplined cleanup inside it.

Three sources cooperate to drive a runner through provisioning to termination.
The guest status file proves that the runner process has launched, but it does
not claim whether that process is idle or busy. The current GitHub message
session supplies exact `JobStarted` events, which protect named busy runners
from scale-down. The reconciler treats any ready runner reconstructed after a
controller restart or message-session reconnect as ambiguous and preserves it.

When demand falls, only a ready runner created under the current message
session and not marked busy can become a scale-down candidate. The controller
first removes its exact GitHub registration and confirms that registration is
absent. That fence prevents a new assignment; it does not stop the VM. The
guest runner process then exits, publishes its terminal status, and powers off.
Only after observing that terminal state does the controller capture console
diagnostics and delete the Incus instance. A job-writable hook is never used as
lifecycle authority. See the [guest contract reference](../reference/guest-contract.md)
for the exact status values and file formats involved.

## Cleanup scope, not authorization

Every VM the controller creates carries the configured `owner` value in one
instance metadata key. Inventory includes only exact matches, and deletion
rechecks that same value immediately before mutation. A missing or different
value therefore prevents the controller from accidentally counting or reaping
another controller's VM. The [guest contract reference](../reference/guest-contract.md)
names the exact key.

The marker does not authorize an Incus operation. Any identity that can edit an
instance in the project can copy the value, and the current controller's local
`incus-admin` socket access can bypass project boundaries entirely. Treat the
marker as a cleanup selector that limits mistakes in the controller's own code,
not as protection against a malicious project tenant or a compromised
controller process.

The project, image, profiles, network, storage, and controller authority are
pre-existing deployment boundaries. The current production contract therefore
requires a dedicated, single-purpose Incus host with a restricted runner
project and network. Sharing that host with unrelated trusted workloads would
turn controller compromise into compromise of those workloads as well.

## Failure philosophy

incus-gh-runner treats "cannot get started" and "was working, then hit a problem" as different situations that deserve different responses.

At startup, invalid configuration and failed dependency setup are fast and
loud. If the initial GitHub message session cannot be opened, or if the Incus
preflight check finds the configured image or any configured profile missing,
the process exits. Once those dependencies are resolved, an uncertain initial
owned-runner inventory is different: the controller stays alive and retries
with capped backoff while scheduling no mutation. This avoids systemd restart
limits turning a transient guest-agent outage into an operator-only recovery.

Once running, the posture flips. A dropped GitHub message session is not fatal — the controller closes the stale session and reopens a new one, backing off with capped exponential delay between attempts (starting at `retry.initial`, capped at `retry.maximum`) so a transient GitHub-side hiccup does not turn into a hot retry loop, and a successful reconnection resets that backoff. Incus operation failures follow the same shape: each failure applies a per-operation cooldown that doubles up to the same cap, so a host that is temporarily overloaded gets progressively more breathing room rather than being hammered with retries.

Any Incus failure also does something more conservative than just backing off that one operation: it marks the controller's view of its own inventory as stale, and the reconciler will not create or delete any VM against stale inventory. It waits for a fresh, successful inventory snapshot before mutating anything again. This exists because the cleanup selector only limits accidental mutation when the controller's picture of matching VMs is accurate — mutating against a snapshot that might already be wrong is exactly when the controller should pause rather than guess.

There is a deliberate final escalation path underneath all of this: if the demand-tracking and reconcile components of the process ever get wedged relative to each other — one stops responding and the other cannot shut down cleanly within its budget — the process exits non-zero rather than limping in a half-alive state. systemd's restart policy is the intended backstop for that case, which is why the unit is configured to restart on failure. The controller does not try to be its own supervisor; it fails in a way a process supervisor can act on.

One consequence of this philosophy is worth calling out directly because it
affects how operators should think about restarts: stopping or restarting the
controller does not touch ready VMs whose exact idle state cannot be
reconstructed. They continue to count as capacity but are ineligible for
scale-down. A restart is therefore not equivalent to canceling in-flight work,
and ambiguity may temporarily retain excess capacity until each one-job runner
finishes naturally.

## Security model

The most consequential fact about how incus-gh-runner runs is one line: the local Incus socket it talks to is root-equivalent. Membership in the `incus-admin` group — which the controller's systemd unit grants via a supplementary group — gives full control over every instance, project, and storage pool the Incus daemon manages, not just the ones incus-gh-runner created. The exact owner check narrows the controller's intended behavior, but it does not narrow this credential. A dedicated, single-purpose host is therefore a requirement for the current production deployment, not optional defense in depth.

Two credentials matter and each is handled to avoid landing in configuration or logs. The controller's renewable GitHub credential is either an App private key or a PAT; the selected systemd drop-in delivers it through `LoadCredential=` and the process reads the protected runtime file once at startup. GitHub-side, each VM gets a fresh, single-use JIT runner configuration generated at creation time — never a long-lived registration token — and that configuration is never written to the controller's own logs.

The same discipline continues into the guest. The one-time job payload that carries the JIT configuration is deleted from disk by the guest before the Actions Runner process starts, so the payload does not sit alongside — or inside the visibility of — the job it configures. The serial console, which the controller captures as diagnostics when it deletes a terminal VM, is deliberately limited to secret-free lifecycle lines emitted by the guest's own init sequence (state transitions and shutdown reason) — it is not a transcript of the job.

That said, diagnostics capture deserves a caveat rather than a blanket assurance:

!!! warning "Console diagnostics can contain workload output"
    The guest's lifecycle lines are secret-free by construction, but the serial console is a shared channel. If a job's own process writes to the console — directly, through a crash, or through unexpected verbose output — that content can end up captured in the diagnostics file alongside the lifecycle lines. Treat the diagnostics directory as potentially containing job output, not only controller-authored status text, and restrict access to it accordingly.

For the exact credential precedence rules, environment variable names, and systemd unit settings, see the [configuration reference](../reference/configuration.md). For the payload and status file schemas and the serial console line format, see the [guest contract reference](../reference/guest-contract.md).

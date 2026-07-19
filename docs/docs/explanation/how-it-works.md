# How incus-gh-runner works

incus-gh-runner turns Incus virtual machines into disposable, single-job GitHub Actions runners. It watches a GitHub Actions runner scale set for demand and keeps a population of Incus VMs matched to that demand, creating runners when jobs need them and deleting runners once their job is done. This page explains the mental model behind that behavior: how demand becomes capacity, how a runner moves through its life, why the controller is safe to point at a shared Incus host, how it responds to failure, and what its security posture assumes.

It intentionally does not list configuration keys, CLI flags, or wire-level schemas. Those live in the [configuration reference](../reference/configuration.md) and the [guest contract reference](../reference/guest-contract.md).

## Demand flow: from GitHub to Incus

GitHub Actions runner scale sets communicate demand through a long-lived message session: the controller opens a session against `github.config_url` for a given `github.scale_set`, and GitHub pushes job assignment events down that session for as long as it stays open. There is exactly one message session per controller process, feeding exactly one reconciler. incus-gh-runner does not fan out across multiple scale sets or multiple Incus environments — that scope boundary is a deliberate design choice. A single process manages a single scale set against a single Incus project and image, which keeps the ownership and capacity model simple enough to reason about at a glance.

The reconciler turns that stream of demand into a target VM count using one formula:

```
target = min(max_runners, min_runners + assigned_jobs)
```

`assigned_jobs` is the number of jobs GitHub currently wants this scale set to run. `min_runners` is a hot-standby floor: a number of runners the controller keeps alive and idle even when there is no work queued, so that the first jobs of a burst land on a VM that is already provisioned and connected rather than waiting for one to boot. `max_runners` is a hard ceiling that protects the Incus host from unbounded growth regardless of how much demand GitHub reports. Raising `min_runners` trades idle compute cost for lower job start latency; raising `max_runners` trades host capacity for burst headroom. The formula is the whole story — there is no separate scale-down cooldown or queue-depth heuristic layered on top of it. Every reconcile pass simply recomputes the target and moves the current runner count toward it.

The reconciler itself is event-driven rather than polling GitHub on a fixed cadence: new demand, completed Incus operations, and a periodic inventory refresh all feed a single coalescing mailbox that keeps only the newest demand signal, so a burst of updates collapses into one reconcile decision rather than a queue of stale ones. A configurable number of Incus operations run concurrently, because creating or deleting a VM is comparatively slow and the reconciler would otherwise serialize on it.

## The lifecycle of a runner

Every runner is a single Incus VM that exists to run exactly one GitHub Actions job, then disappears. It moves through four states:

- **provisioning** — the VM has been created and started, but has not yet reported that its runner process is live.
- **idle** — the runner is connected to GitHub and waiting for a job (this is what satisfies `min_runners`).
- **busy** — the runner has picked up a job and is executing it.
- **terminal** — the job is done, or the VM failed, or it never came up in time. It is waiting to be deleted.

The one-job-per-VM design is the load-bearing decision here. Because a VM never runs a second job, there is no cleanup step that has to scrub a workspace between jobs, no risk of one job's state leaking into the next, and no need to trust that a long-lived runner stays healthy across dozens of jobs. Reproducibility comes from throwing the VM away, not from disciplined cleanup inside it.

Two systems cooperate to drive a runner through provisioning to termination, and each owns a different half of the lifecycle. Inside the guest, the runner's own init system watches for a one-time job payload, launches the Actions Runner against it, and — this is deliberate — powers the VM off itself the moment that single job exits, successfully or not. The guest does not wait to be told to shut down; it is responsible for ending its own life once its purpose is fulfilled. Outside the guest, the controller only ever observes state (by combining the VM's Incus power state with a status file the guest publishes) and acts on the terminal state it sees: before it deletes a terminal VM, it captures the VM's serial console output as diagnostics, then stops and deletes the instance. This split matters operationally — a runner that finishes its job and shuts down is not "stuck" or "leaking" while it waits in the terminal state; it is waiting for the controller's next reconcile pass to be cleaned up and have its diagnostics preserved. See the [guest contract reference](../reference/guest-contract.md) for the exact status values and file formats involved.

## The ownership boundary

incus-gh-runner is designed to run on a shared Incus host without becoming a hazard to whatever else lives there. It does this with a narrow, exact-match ownership check: every VM the controller creates is tagged with an owner marker — an instance metadata key carrying the value of the `owner` configuration — and the controller will only list, count, or delete a VM whose marker matches its own configured owner exactly. A VM without the marker, or with a different owner value, is invisible to the controller's mutating operations — it is never counted toward capacity and never deleted, no matter what state it is in. The [guest contract reference](../reference/guest-contract.md) names the exact metadata key.

This is the entire safety mechanism, and it is intentionally simple: there is no naming convention to parse, no heuristic about VM age or image, just one metadata field compared for equality. That simplicity is what makes it trustworthy — a mechanism an operator can verify by inspecting a single instance property is one they can actually reason about under pressure, unlike a mechanism that infers ownership from indirect signals.

It is worth being explicit about what this boundary does and does not buy an operator. It makes it *survivable* to run incus-gh-runner alongside other Incus workloads on the same host: the controller will not touch VMs it does not own, so a shared host does not put unrelated instances at risk of being reaped by a reconcile pass. It does not make a shared host a good idea. incus-gh-runner does not create or manage the Incus project, image, profiles, network, or storage it depends on — those are assumed to pre-exist — and it has no visibility into or opinion about resource contention, noisy neighbors, or the blast radius of a misconfigured `owner` value colliding with another tool's convention. A dedicated Incus host remains the recommended deployment target; the ownership boundary is a safety net for the cases where that is not practical, not a design invitation to co-locate freely.

## Failure philosophy

incus-gh-runner treats "cannot get started" and "was working, then hit a problem" as different situations that deserve different responses.

At startup, failure is fast and loud. If the initial GitHub message session cannot be opened, or if the Incus preflight check finds the configured image or any configured profile missing, the process exits rather than retrying quietly in a degraded state. The reasoning is that a controller which cannot even confirm its dependencies exist has nothing useful to offer by staying up — better to fail immediately and visibly than to sit in a loop reporting a problem no one asked it to retry.

Once running, the posture flips. A dropped GitHub message session is not fatal — the controller closes the stale session and reopens a new one, backing off with capped exponential delay between attempts (starting at `retry.initial`, capped at `retry.maximum`) so a transient GitHub-side hiccup does not turn into a hot retry loop, and a successful reconnection resets that backoff. Incus operation failures follow the same shape: each failure applies a per-operation cooldown that doubles up to the same cap, so a host that is temporarily overloaded gets progressively more breathing room rather than being hammered with retries.

Any Incus failure also does something more conservative than just backing off that one operation: it marks the controller's view of its own inventory as stale, and the reconciler will not create or delete any VM against stale inventory. It waits for a fresh, successful inventory snapshot before mutating anything again. This exists because the ownership boundary's safety guarantee only holds if the controller's picture of which VMs it owns is accurate — mutating against a snapshot that might already be wrong is exactly the kind of failure mode the ownership check is meant to prevent, so the controller would rather pause than guess.

There is a deliberate final escalation path underneath all of this: if the demand-tracking and reconcile components of the process ever get wedged relative to each other — one stops responding and the other cannot shut down cleanly within its budget — the process exits non-zero rather than limping in a half-alive state. systemd's restart policy is the intended backstop for that case, which is why the unit is configured to restart on failure. The controller does not try to be its own supervisor; it fails in a way a process supervisor can act on.

One consequence of this philosophy is worth calling out directly because it affects how operators should think about restarts: stopping or restarting the controller does not touch VMs that are currently running a job. Busy runners are left alone through a controller shutdown or crash and pick up again from GitHub's perspective once the controller comes back — a controller restart is not equivalent to canceling in-flight work.

## Security model

The most consequential fact about how incus-gh-runner runs is one line: the local Incus socket it talks to is root-equivalent. Membership in the `incus-admin` group — which the controller's systemd unit grants via a supplementary group — gives full control over every instance, project, and storage pool the Incus daemon manages, not just the ones incus-gh-runner created. This is why the ownership-boundary discussion above and the deployment guidance converge on the same recommendation: even though the ownership check keeps the controller's own mutating operations scoped to VMs it owns, the underlying socket access is not scoped at all, and a dedicated host is the only way to make that access boundary match the trust boundary an operator actually wants.

Two credentials matter and each is handled to avoid landing in configuration or logs. The controller's renewable GitHub credential is either an App private key or a PAT; the selected systemd drop-in delivers it through `LoadCredential=` and the process reads the protected runtime file once at startup. GitHub-side, each VM gets a fresh, single-use JIT runner configuration generated at creation time — never a long-lived registration token — and that configuration is never written to the controller's own logs.

The same discipline continues into the guest. The one-time job payload that carries the JIT configuration is deleted from disk by the guest before the Actions Runner process starts, so the payload does not sit alongside — or inside the visibility of — the job it configures. The serial console, which the controller captures as diagnostics when it deletes a terminal VM, is deliberately limited to secret-free lifecycle lines emitted by the guest's own init sequence (state transitions and shutdown reason) — it is not a transcript of the job.

That said, diagnostics capture deserves a caveat rather than a blanket assurance:

!!! warning "Console diagnostics can contain workload output"
    The guest's lifecycle lines are secret-free by construction, but the serial console is a shared channel. If a job's own process writes to the console — directly, through a crash, or through unexpected verbose output — that content can end up captured in the diagnostics file alongside the lifecycle lines. Treat the diagnostics directory as potentially containing job output, not only controller-authored status text, and restrict access to it accordingly.

For the exact credential precedence rules, environment variable names, and systemd unit settings, see the [configuration reference](../reference/configuration.md). For the payload and status file schemas and the serial console line format, see the [guest contract reference](../reference/guest-contract.md).

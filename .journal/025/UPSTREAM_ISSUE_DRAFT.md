# Draft upstream issue: `GetMessage` long poll does not wake for `JobStarted`

> Draft only. Do not submit without maintainer approval.

## Title

`GetMessage` long poll does not wake when a `JobStarted` message becomes available

## Body

### Summary

Using `github.com/actions/scaleset v0.4.0`, we consistently observe `JobStarted` only on the request immediately after the current approximately-50-second `GetMessage` request expires empty.

A controlled A/B canary on the same bare-metal host, scale set, proof-capable VM image, workflow, and hot-standby capacity reduced the per-request client deadline from the normal approximately 50 seconds to 5 seconds. `runnerAssignTime`-to-callback delivery fell from `50.531s` to `5.402s`, matching the changed poll boundary.

This appears inconsistent with the v0.4.0 README's documented long-poll behavior: return immediately when a message is available; otherwise block up to approximately 50 seconds and return 202.

### Versions

- `github.com/actions/scaleset`: `v0.4.0`
- Controller source: `github.com/meigma/incus-gh-runner`
- Controller diagnostic commit: `921ee6ff847c8482c07bfc1534143215ec008755`
- Incus: `7.0.1`
- Actions Runner: `2.335.1`
- Host: Latitude c3.small.x86, MEX2

The same normal-poll symptom was independently observed with Actions Runner `2.336.0` in another datacenter.

### Controlled A/B

Both jobs began on an already connected idle runner. The short-poll adapter applied a five-second child context only to `GetMessage` and translated only its own `context.DeadlineExceeded` into `(nil, nil)`. Parent cancellation and all other errors retained normal failure/reconnect behavior.

#### Baseline: normal long poll

- Workflow run: `30166003024`
- Job: `89699019984`
- Workflow proof wait: `47.840s`
- Poll start: `16:37:32.617459289Z`
- Empty poll completion: `16:38:22.730698388Z`
- Empty poll duration: `50.113239099s`
- Next poll returned `JobStarted` in: `149.560077ms`
- Event `runnerAssignTime`: `16:37:32.444015751Z`
- Callback entry: `16:38:22.975129060Z`
- `runnerAssignTime` to callback: `50.531113309s`

#### Canary: five-second per-request deadline

- Workflow run: `30166076210`
- Job: `89699212952`
- Workflow proof wait: `2.089s`
- Poll start after assignment: `16:39:53.242071399Z`
- Intentional timeout completion: `16:39:58.243320342Z`
- Poll duration: `5.001248943s`
- Next poll returned `JobStarted` in: `133.513989ms`
- Event `runnerAssignTime`: `16:39:53.077064531Z`
- Callback entry: `16:39:58.478991631Z`
- `runnerAssignTime` to callback: `5.401927100s`

In both samples, `runnerAssignTime` preceded the in-flight poll start, the in-flight request did not return `JobStarted`, and the next request returned it in approximately 0.13–0.15 seconds.

### Expected behavior

If `JobStarted` becomes available while `GetMessage` is in flight, the long poll should return it promptly, consistent with the README's long-poll contract.

### Actual behavior

The current request expires empty at its poll boundary. The immediately following request receives `JobStarted`. Changing only the boundary from approximately 50 seconds to 5 seconds changes the delivery delay by the same amount.

### Questions

1. Is `JobStarted` intentionally visible only to a new `GetMessage` request rather than waking an in-flight long poll?
2. If so, can that limitation be documented and can the client expose a supported poll-duration option?
3. If not, can the message service wake the pending long poll when lifecycle messages become available?
4. Is canceling and immediately reopening `GetMessage` at a shorter bounded interval supported, and what minimum interval or load guidance applies?

### Security and diagnostics

The controller logs only request timing, empty/message/error outcome, message ID, message-type counts, and the event's queue/assignment timestamps. It does not log queue URLs, authorization headers, tokens, JIT configuration, response bodies, or private-key material.

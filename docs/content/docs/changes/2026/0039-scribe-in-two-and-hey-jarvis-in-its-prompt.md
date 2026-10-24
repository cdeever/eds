---
title: "CHG-0039: scribe in Two, and \"Hey Jarvis\" in Its Prompt"
weight: 39
---

# CHG-0039: scribe Is a Base Sent Once and Its Code Built on the Workload, and "Hey Jarvis" Is in Its Prompt

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The services workload: `eds-scribe-base` loaded, `eds-scribe` built there, nowplayd and voiced sent, all restarted |
| **How** | `make -C deploy ship WHERE=builder` |
| **Risk** | Medium. Most likely to go wrong: the core router under the base's one transfer (INC-0002), which is compressed to 256 MB and held to 5 MB/s |
| **Decisions** | [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/) |
| **Related** | [INC-0002](/docs/incidents/2026/0002-deploys-hung-the-core-router/), whose follow-up this is; [CHG-0038](/docs/changes/2026/0038-a-misheard-play-and-a-longer-pre-roll/) |

---

## Summary

The end of the wake word, kept with each request, was written "Lee Jarvis",
"Jay Jarrus", "Travis", "The band was very generous", and the request after
it was misheard with it. With "Hey Jarvis" in scribe's prompt, the same
recordings, cut as the device cuts them, were written "Jarvis," each time,
and nothing else changed.

The prompt is scribe's code, and scribe's image was 767 MB. It is two now:
`eds-scribe-base`, the runtime, libraries and model with what scribe never
runs taken out (533 MB, 256 compressed), sent when its dependencies change;
and `eds-scribe`, 60 KB of code built on the workload on that base, its
self-test run there.

The grammar skips "Jarvis" as it skips "Ed", and no longer takes
"Genesis' first album" for Genesi's.

## Goal

- The base is sent once, and the router stays up.
- `eds-scribe` built on the workload, its self-test passing there.
- "Hey Jarvis, play Genesis' first album" plays it.

## Procedure

1. `make -C deploy ship WHERE=builder`.
2. `make -C deploy status`; scribe's `/healthz`.
3. The owner asks.

## Undo

`git revert` this change's commit, and ship: the base stays loaded, unused.

## Outcome

| When (UTC) | What happened |
|---|---|
| 04:04:19–04:08:16 | `ship WHERE=builder` from `879103d`: four images neither built nor sent; `eds-scribe-base` built on the Builder and sent, 256 MB compressed at 5 MB/s; nowplayd and voiced sent, 4 MB each; `eds-scribe` built on the workload from 60 KB of sources, its self-test passing. The router stayed up. The ship then stopped: pruning before the restart, podman refused an image the running scribe still used. |
| 04:09 | Pruning moved to after the restart; the six restarted. scribe loaded its model in 2.5 s from the image built there; voiced on `879103d`; no timeouts. |
| 04:09:30 | The prune, which matched only images labelled `eds.deploy`, left fifty untagged images of the evening's deploys; the workload holds EdS's alone, so every untagged image is pruned now. None left; 3.2 GB used of 30. |
| 04:10:17 | Built and loaded again, nothing changed: nothing built, nothing sent, scribe not built again, in 10 s. |

### Departures from the plan

- The prune's order and filter, both corrected in the change.

## Follow-ups

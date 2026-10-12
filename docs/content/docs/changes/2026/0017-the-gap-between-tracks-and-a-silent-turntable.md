---
title: "CHG-0017: The Gap Between Tracks, and a Silent Turntable"
weight: 17
---

# CHG-0017: The Gap Between Tracks, and a Silent Turntable

| | |
|---|---|
| **Date** | 2026-10-12 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `npagent` on the agent Pi (`np-relay`); `nowplayd` and the console on the workload |
| **How** | `make -C deploy/relay push`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: a wrong title for a few seconds after a quiet passage, or a room that dims while a record is being turned over and surprises someone |
| **Decisions** | [ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/), which this is the evidence for |
| **Related** | [CHG-0013](/docs/changes/2026/0013-the-turntable-driver/), [CHG-0016](/docs/changes/2026/0016-idle-the-screen-on-the-console-and-no-buttons-for-a-record/) |

---

## Summary

Three things about silence, asked for together.

- The turntable driver listens for the gap between two tracks, and names the
  track after it from one clip. A track change took thirty to sixty seconds
  to show.
- A turntable that has gone silent is idle about twenty seconds later, not
  five minutes: the stand dims and the sleeve is taken away.
- The console stops showing a sleeve under "Nothing is playing". That one is
  a mistake put right, not a decision: the page hid the picture in a way its
  own style sheet overruled.

## Goal

- A track that follows a gap is named about thirteen seconds in.
- The needle lifted: about twenty seconds later the stand is dim, and the
  console and the cover screen show no sleeve.
- The needle down again: the sleeve and the lights are back with the first
  track named.

## Scope

**In scope:** the agent on the Pi; `nowplayd`'s one setting,
`NP_IDLE_AFTER_STOPPED`; the console's page.

**Out of scope:** `lightd`, `palette`, every firmware, the tenant. The idle
scene is the one the stand already knows.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The gaps on this input are not as quiet as -55 dBFS | the driver | then no gap is heard and the driver behaves as it did; nothing is worse |
| A wrong name after a quiet passage | the console | it is one answer and the next clip corrects it; the cover is held by the album and the collection |
| The agent does not come back after the push | the Pi | `make -C deploy/relay status` straight after; the push before this one is the undo |
| The room dims while a record is turned over | the room | that is what was asked for, and it is a setting |

## Procedure

1. **The Pi.** `make -C deploy/relay push`, then `status`.
2. **The workload.** `make -C deploy deploy WHERE=builder`.

Either order: the two halves do not depend on each other.

## Verification

- The agent's log names a track about thirteen seconds after a gap.
- With the needle up: `nowplayd` says "idle" about twenty seconds after the
  last sound, `eds/nowplaying/current/art` is empty, and the stand's `state`
  says solid at 0.15.
- The console says "Nothing is playing" with no sleeve above it.

## Undo

- **The gap:** push the agent from the commit before this one.
- **The lights:** `NP_IDLE_AFTER_STOPPED=5m` in `deploy/nowplayd.env`, and
  `make -C deploy units`.
- **The console:** deploy the commit before this one.

## Outcome

| When (UTC) | What happened |
|---|---|
| 21:36 | The agent was pushed to the Pi by its owner (`65e6dfc`); the push was refused to the assistant. It came back active with both drivers online, listening to `plughw:UA4FX`. The turntable was silent, so nothing about the gap was seen. |
| 21:38 | The workload's deploy, run by its owner from the worktree, built and loaded the four images and then stopped: the tenant's directory was not passed, and the worktree has none. Nothing was installed or restarted. |
| 21:39 | `make units`, with the tenant's directory named, installed the settings and restarted the four services (`2b486a2`). All four came back active. What was current was iTunes, paused long before, so `nowplayd` was idle at once, `lightd` heard no cover and sent the idle scene, solid at 0.15. The turntable was not on, so the new wait had nothing to act on. |

## Departures

- Both halves were run by the owner; the assistant was refused the push.
- The workload was deployed from a worktree, which has neither the tenant's
  backend settings nor `discogs.env`. The first was named with `INFRA=`, the
  second copied in. The first try lost `INFRA=` to a line break.

## Follow-ups

- [ ] The level of the input in a gap, measured, for ADR-0026.
- [ ] A side played and timed, for ADR-0026.

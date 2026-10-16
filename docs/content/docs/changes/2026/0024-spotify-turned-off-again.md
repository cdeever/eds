---
title: "CHG-0024: Spotify Turned Off Again"
weight: 24
---

# CHG-0024: Spotify Turned Off Again

| | |
|---|---|
| **Date** | 2026-10-16 |
| **Change type** | Configuration |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-16 |
| **Systems** | the agent Pi: one line of its settings, and its agent restarted |
| **How** | `make -C deploy/relay drivers DRIVERS=itunes,turntable` |
| **Risk** | Low. Most likely to go wrong: the agent not coming back, which would stop iTunes and the turntable being followed too |
| **Decisions** | [ADR-0031](/docs/architecture/decisions/0031-spotify-is-shown-like-itunes/) and [ADR-0032](/docs/architecture/decisions/0032-spotify-takes-commands/), unchanged |
| **Related** | [CHG-0022](/docs/changes/2026/0022-spotify-shown-and-on-again/), which turned it on; [CHG-0011](/docs/changes/2026/0011-spotify-turned-off/), the same change once before |

---

## Summary

The Spotify driver was asking Spotify what is playing six times a minute,
all day, as the **Outside Services** count showed. Its owner does not want
that for now and will come back to it. The agent Pi now runs the iTunes and
turntable drivers only. Nothing is removed: the driver, its authorization on
the Pi, the broker grants and the app at Spotify all stay, and one command
turns it back on.

## Goal

- The agent runs with `NP_DRIVERS=itunes,turntable` and makes no request to
  Spotify.
- iTunes and the turntable are followed as before.
- The Spotify source reads as offline.

## Procedure

1. `make -C deploy/relay drivers DRIVERS=itunes,turntable`.

## Verification

- The Pi's settings say `NP_DRIVERS=itunes,turntable`, and `npagent` is
  active.
- No `api.calls` line for `spotify` after the restart, other than the one
  flushed on the way down.
- The console shows `spotify` offline.

## Undo

`make -C deploy/relay drivers DRIVERS=itunes,spotify,turntable`.

## Outcome

| When (UTC) | What happened |
|---|---|
| 17:02 | `drivers DRIVERS=itunes,turntable`. `npagent` stopped, flushed its last count (`6 calls to spotify`), and came back active at 17:02:43 with `NP_DRIVERS=itunes,turntable`. The turntable driver was online within a second. |
| 17:04 | No further `api.calls` for `spotify`. The console's `/api/state`: `spotify` offline, `turntable` online. `itunes` offline, as its own driver reported at 17:02:46: it did not find iTunes, which this change did not touch. |

## Follow-ups

- [ ] Come back to Spotify: what it should cost to ask, and whether the
  poll can be slower when nothing is playing.

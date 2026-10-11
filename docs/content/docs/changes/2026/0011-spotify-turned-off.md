---
title: "CHG-0011: Spotify Turned Off"
weight: 11
---

# CHG-0011: Spotify Turned Off

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Configuration |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | the agent Pi: one line of its settings, and its agent restarted |
| **How** | `make -C deploy/relay drivers DRIVERS=itunes` |
| **Risk** | Low. Most likely to go wrong: the agent not coming back, which would stop iTunes being followed too |
| **Decisions** | [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/), unchanged |
| **Related** | [CHG-0004](/docs/changes/2026/0004-spotify-on-the-agent-pi/), which turned it on |

---

## Summary

The Spotify driver works, and nothing uses what it reports: the stand does
not follow Spotify, by decision. Meanwhile it asks Spotify what is playing
every ten seconds, all day. Its owner is not using it for now and will come
back to it.

After this change the agent Pi runs the iTunes driver only. Nothing is
removed: the driver, its authorization on the Pi, the broker grants and the
app at Spotify all stay, and one command turns it back on.

## Goal

- The agent runs with `NP_DRIVERS=itunes` and makes no request to Spotify.
- iTunes is followed as before, and the stand with it.
- The Spotify source reads as offline, not as a player with nothing loaded.

## Scope

**In scope:** `NP_DRIVERS` in the Pi's settings; a `drivers` target in
`deploy/relay` to set it.

**Out of scope:** the driver's code, the Pi's image, the authorization, the
grants, `NP_COVERLESS_SOURCES` on the workload. All are left as they are for
the day it is turned back on.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The agent does not restart | the Pi | the target reports whether it is active; iTunes reporting is checked after |
| The settings file is damaged | the Pi | one line is deleted and one appended; the file holds the broker password and is not printed, only the one line read back |
| Spotify is left looking online | the bus | its status is a last will; checked on the console afterwards |

## Procedure

```bash
make -C deploy/relay drivers DRIVERS=itunes
```

## Verification

- The target prints `active` and `NP_DRIVERS=itunes`.
- The agent's log names one driver, and no Spotify poll follows.
- The console shows iTunes online and Spotify offline.
- A track change in iTunes still reaches the console.

## Undo

```bash
make -C deploy/relay drivers DRIVERS=itunes,spotify
```

The authorization on the Pi is good until 2027-04-09. After that, turning it
on also needs `make -C deploy/relay spotify AGAIN=1`.

## Outcome

| When (UTC) | What happened |
|---|---|
| 20:47 | The target printed `active` and `NP_DRIVERS=itunes`. The agent logged `agent started drivers=itunes`, connected once, as the iTunes driver, and reported the track paused in iTunes. Nothing in its log mentions Spotify after the restart. |
| 20:48 | The console showed iTunes online with its track and cover, and Spotify offline, last heard at 19:24. |

A track change in iTunes after the restart was not waited for. The agent
reported the paused track on reconnecting, which is the same path.

### Departures from the plan

None.

## Follow-ups

- [ ] The image built for the Pi still starts both drivers unless a card's
  settings say otherwise. A card written while Spotify is off should be
  written with `DRIVERS=itunes`.
- [ ] When it is wanted again: `make -C deploy/relay drivers
  DRIVERS=itunes,spotify`, and after 2027-04-09 a fresh authorization.

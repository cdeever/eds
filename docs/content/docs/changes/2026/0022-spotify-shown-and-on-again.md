---
title: "CHG-0022: Spotify Shown, and On Again"
weight: 22
---

# CHG-0022: Spotify Shown, and On Again

| | |
|---|---|
| **Date** | 2026-10-13 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `npagent` on the agent Pi (`np-relay`); `nowplayd` on the workload |
| **How** | `make -C deploy/relay push`; `make -C deploy/relay drivers DRIVERS=itunes,spotify,turntable`; `make -C deploy units` |
| **Risk** | Low. Most likely to go wrong: the Spotify authorization given on 2026-10-11 no longer stands, and the driver stops and says so; or the agent does not come back after the push |
| **Decisions** | [ADR-0031](/docs/architecture/decisions/0031-spotify-is-shown-like-itunes/), which this is the evidence for |
| **Related** | [CHG-0011](/docs/changes/2026/0011-spotify-turned-off/), which this undoes; [CHG-0004](/docs/changes/2026/0004-spotify-on-the-agent-pi/); [CHG-0020](/docs/changes/2026/0020-the-stand-starts-dark-blinks-opens-and-scans/), whose scanner is what was missed from Spotify |

---

## Summary

Spotify was reported with no cover, and its driver was off. The driver now
takes the album's picture, Spotify is kept off the stand the way iTunes is,
and the driver is on again.

## Goal

- Spotify playing on a named device: the cover screen and the console show
  the album, and the stand is dark.
- The theme from *Knight Rider* on Spotify: the scanner.
- iTunes and the turntable as they were.

## Scope

**In scope:** the agent on the Pi (a new build, and one more driver
running); two lines of `nowplayd`'s settings.

**Out of scope:** `lightd`, the console, every firmware, the tenant. The
Spotify authorization is not given again unless it has ended, and its scopes
do not change.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The authorization has ended | the Pi | the driver stops and logs it; `make -C deploy/relay spotify AGAIN=1` gives it again; iTunes and the turntable are other drivers and are not touched |
| The agent does not come back | the Pi | `status` straight after the push; the push before this one is the undo |
| Spotify takes the room from a record | the room | it cannot: a turntable that is switched on holds it (ADR-0023) |
| `nowplayd` is told the new settings before the agent sends covers | the workload | harmless: a Spotify track with no cover from its driver has one looked up |

## Procedure

1. `make -C deploy/relay push`, then `status`.
2. `make -C deploy/relay drivers DRIVERS=itunes,spotify,turntable`.
3. `make -C deploy units`, for `nowplayd.env`.

## Verification

- The agent's log: the Spotify driver online.
- With Spotify playing: `nowplaying/current` names the track with `art`
  from `player`, the console shows the sleeve, and the stand's scene is
  solid at 0.
- An `api.calls` line for `spotify-images`.

## Undo

- **Off again:** `make -C deploy/relay drivers DRIVERS=itunes,turntable`.
- **Coverless again:** `NP_COVERLESS_SOURCES=spotify` back in
  `deploy/nowplayd.env`, `spotify` out of `NP_DARK_SOURCES`, and
  `make -C deploy units`.

## Outcome

| When (UTC) | What happened |
|---|---|
| 21:39 | The agent was pushed to the Pi (`0ef1db8`) and came back active. The input chosen in CHG-0021, `plughw:CODEC`, was still its setting. |
| 21:39 | `drivers DRIVERS=itunes,spotify,turntable`. The Spotify driver said it was online and that Spotify was stopped. `status`: authorized until 2027-04-09, the device that counts is `MEDIA-SERVER`, and Spotify's API reachable. iTunes paired and reachable; the turntable listening. |
| 21:39 | `make units`, from the worktree. `nowplayd` restarted, no longer said it reports any source without a cover, and said it publishes `itunes` and `spotify` with the look that lights nothing. All four services active. With nothing playing anywhere and the turntable off, `current` was Spotify's, stopped, and the stand was solid at 0. |

## Departures

- Deployed from the branch before it was merged, on the owner's word to do
  it.
- `make units` was run from a worktree: the tenant's directory named with
  `INFRA=`, and `discogs.env` copied in, as in CHG-0017.

## Follow-ups

- [ ] Nothing was playing on Spotify. No cover has been fetched, and
  `spotify-images` has not been counted.

- [ ] What ADR-0031 lists as not yet shown.

---
title: "CHG-0029: The Turntable's Album by What Is on It"
weight: 29
---

# CHG-0029: The Turntable's Album by What Is on It

| | |
|---|---|
| **Date** | 2026-10-19 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The home backend Pi, `dv01rpi001p01` (`192.168.2.40`): nowplayd and npagent, with the other services restarted |
| **How** | `make -C deploy/backend push PI=192.168.2.40` from `main` at `44828a9` |
| **Risk** | Low. Most likely to go wrong: a lookup that names the wrong album for a record not in the collection; the stand is lit from the wrong sleeve until the next track |
| **Decisions** | [ADR-0038](/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it/), whose evidence this is |
| **Related** | [CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/), the Pi this goes to |

---

## Summary

On the first evening at home, *Animals* put on a minute after *The Dark Side
of the Moon* kept *Dark Side*'s rainbow: the driver held a side's first
album for ten minutes of silence. ADR-0038 names the album by what is on the
record instead, and prefers the original album. This puts it on the Pi.

Nothing else changes: the Pi's accounts, its iTunes pairing, Discogs and the
turntable's input are not touched by a push.

## Goal

- The Pi's release file names `44828a9`; every service and npagent active;
  `eds-backend-status` clean.
- On the turntable, without waiting between them: *Dark Side* and then
  *Animals*, each lit from its own sleeve.
- A record turned over keeps its sleeve.
- A record not in the collection lights from its original album.

## Scope

**In scope:** nowplayd and npagent on the home Pi.
**Out of scope:** rebuilding the image (a follow-up, so the next card is not
older than this Pi); the devices.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The wrong album named | the stand, the screen | The next track names it again; `make push` from the commit before undoes it |
| A service fails to start | the Pi | `eds-backend-status`; the push from `6dadefe` undoes it |
| More calls to Deezer and MusicBrainz | outside services | Counted (ADR-0027), cached, MusicBrainz at most one a second |

## Procedure

### Step 1: Push

The services restart, a few seconds each.

**Run:** `make -C deploy/backend push PI=192.168.2.40`

**Verify:** each unit `active`; `eds-backend-status`.

### Step 2: On the turntable

**Verify:** the Goal's last three checks, in nowplayd's log
(`msg=current ... album=`) and on the stand.

## Verification

The Goal's checks.

## Undo

`make -C deploy/backend push PI=192.168.2.40` from `6dadefe`.

## Outcome

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| 2026-10-19 01:54 UTC | 1 | `make -C deploy/backend push PI=192.168.2.40` from `44828a9` (#58 merged): `installed 44828a9`; palette, lightd, nowplayd, the console and npagent active. Untouched by it, as intended: iTunes paired (`192.168.2.104`), `NP_DRIVERS=itunes,turntable`, `NP_TURNTABLE_DEVICE=plughw:UA4FX`, Discogs (226 records listed on restart). |

### Departures from the plan

-

## Follow-ups

- [ ] Rebuild the image (`make -C deploy/backend image fetch`), so the next
      card is not older than the Pi.

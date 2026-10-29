---
title: "CHG-0044: Spotify Is Switched From the Backend Pi's Console"
weight: 44
---

# CHG-0044: Spotify Is Switched From the Backend Pi's Console

| | |
|---|---|
| **Date** | 2026-10-29 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} |
| **Systems** | The home backend Pi, `eds-backend` (`192.168.2.40`): its services, the agent, and the broker accounts `eds-console` and `eds-np-relay` |
| **How** | `make -C deploy/backend push` and `make drivers DRIVERS=itunes,spotify,turntable`, from the branch of PR #106 |
| **Risk** | Low. Most likely to go wrong: Spotify asked all day again, which the switch defaulting to off is there to prevent |
| **Decisions** | [ADR-0051](/docs/architecture/decisions/0051-spotify-is-switched-on-and-off-from-the-console/) |
| **Related** | [CHG-0024](/docs/changes/2026/0024-spotify-turned-off-again/), whose follow-up this answers; [CHG-0043](/docs/changes/2026/0043-the-backend-pi-names-the-record-the-recognizer-heard/), the push before |

---

## Summary

The backend Pi ran `4e51363` and did not run the Spotify driver. After: it
runs ADR-0051's agent and console. Spotify is allowed in `NP_DRIVERS`, and
whether it runs is the console's switch, off unless switched on.

The tenant and the relay Pi are not changed: their half needs dvntm.

## Goal

- With no setting, Spotify offline and asked nothing.
- On and off from the console's page, each in seconds, with no restart.
- On again by itself after the agent restarts.
- iTunes and the turntable untouched throughout.

## Procedure

### Step 1: Push

**Run:** `make -C deploy/backend push`, in the worktree of
`adr-0051-spotify-switch` with `main` merged in (`8179e32`).

**Verify:** each service active, and `eds-backend-setup` granting
`eds-console` and `eds-np-relay` the switch's topic.

### Step 2: Allow Spotify

**Run:** `make drivers DRIVERS=itunes,spotify,turntable`.

**Verify:** the console says `off`, Spotify is offline, and no calls to
Spotify.

### Step 3: The switch

**Run:** on, from `POST /api/spotify`; a restart of `npagent`; something
played on Spotify; off, from the page's button.

## Verification

Every check under Goal, and ADR-0051's list.

## Undo

`make drivers DRIVERS=itunes,turntable`, and `make push` from `main`
without ADR-0051.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-10-29 03:38 | 1 | `installed 8179e32`; `eds-console` and `eds-np-relay` granted the switch's topic; every service active |
| 2026-10-29 03:38 | 2 | `NP_DRIVERS=itunes,spotify,turntable`. The Spotify driver connected for its switch, offline, and the console said `off`. No call to Spotify until 03:50, eleven minutes |
| 2026-10-29 03:50 | 3 | On from `POST /api/spotify`: switched, following the account (MEDIA-SERVER and the S22) and online within a second |
| 2026-10-29 03:50 | 3 | `npagent` restarted: on again in half a second from the retained setting; the old process flushed `3 calls to spotify` on its way down |
| 2026-10-29 03:53 | 3 | *Van Halen II* played on Spotify: current, with Spotify's cover; the stand dark |
| 2026-10-29 03:56 | 3 | Off from the page's button, from the owner's Mac: switched off and offline in the same millisecond. No call to Spotify from 03:56:19 to 03:58:33 |

### Departures from the plan

- ADR-0051 named the relay Pi for its checks. They were made on the
  backend Pi, the same agent and console, because the relay and the
  tenant's Terraform need dvntm.
- The owner first tried the page with a tab left open from before the
  push, and pressed the player's pause; a reload showed the new row.

## Follow-ups

- [ ] The tenant's half of ADR-0051, on a day on dvntm: the grants applied,
      the console deployed, the relay's agent pushed.
- [ ] Rebuild the backend image, so a new card has ADR-0051.

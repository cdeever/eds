---
title: "CHG-0023: Spotify Takes Commands"
weight: 23
---

# CHG-0023: Spotify Takes Commands

| | |
|---|---|
| **Date** | 2026-10-13 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `npagent` on the agent Pi (`np-relay`); the agent's broker account, in the tenant; the Spotify authorization |
| **How** | `make -C deploy/relay push`; `make apply` in `infra/deevnet-tenant-eds`; `make -C deploy/relay spotify AGAIN=1` |
| **Risk** | Low. Most likely to go wrong: the authorization is given again and something about it fails, leaving Spotify unreported until it is done once more |
| **Decisions** | [ADR-0032](/docs/architecture/decisions/0032-spotify-takes-commands/), which this is the evidence for |
| **Related** | [CHG-0022](/docs/changes/2026/0022-spotify-shown-and-on-again/), which put Spotify on the cover screen; [CHG-0012](/docs/changes/2026/0012-the-cover-screen/) |

---

## Summary

The cover screen's buttons did nothing while Spotify was current. The
Spotify driver now carries out play or pause, next, previous and volume
through Spotify's Web API. Three things have to change for a press to
arrive: the agent, the broker account that lets it hear the command, and
what Spotify has authorized it to do.

## Goal

- Each of the five buttons does what it says, with Spotify playing on a
  device that counts.
- iTunes' buttons as before. The turntable still shows none.

## Scope

**In scope:** the agent on the Pi; one more topic the agent's account may
read; the Spotify authorization, given again with one more scope.

**Out of scope:** `nowplayd`, which already passes a command to whichever
source is current; the cover screen's firmware; `lightd`; the console.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| Replacing the broker account issues a new password | the tenant | the plan is read first: it must show the account changed in place, one topic added, and nothing replaced |
| The new authorization does not reach the Pi | the Pi | the old one is replaced only when the new one is installed; `status` says until when it is authorized |
| A press pauses Spotify somewhere else | wherever the account is | the driver sends nothing unless a device that counts is playing |
| The steps are done out of order | everywhere | each is harmless alone: a new agent with the old authorization is refused by Spotify and logs it; an agent not granted the topic says so and goes on reporting |

## Procedure

1. `make -C deploy/relay push`, then `status`.
2. In `infra/deevnet-tenant-eds`: `make plan`, read it, `make apply`.
3. `make -C deploy/relay spotify AGAIN=1`: a browser opens, Agree is
   clicked, and the authorization is handed to the Pi.
4. Restart the agent if it has not been, so it subscribes with the grant.

## Verification

- The agent's log has no `driver.commands_refused` for Spotify.
- A press on the cover screen: `driver.command` in the agent's log, and
  `nowplaying/current` changing within a second or two.

## Undo

- **The buttons hidden instead:** `spotify` in `NP_NO_CONTROLS_SOURCES` in
  `deploy/nowplayd.env`, and `make -C deploy units`.
- **The scope:** remove the app at spotify.com/account/apps and authorize
  again from the commit before this one.
- **The grant:** take the line out of `main.tf` and apply.

## Outcome

| When (UTC) | What happened |
|---|---|

## Departures

## Follow-ups

- [ ] What ADR-0032 lists as not yet shown.

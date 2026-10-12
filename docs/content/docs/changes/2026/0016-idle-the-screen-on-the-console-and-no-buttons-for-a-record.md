---
title: "CHG-0016: Idle, the Screen on the Console, and No Buttons for a Record"
weight: 16
---

# CHG-0016: Idle, the Screen on the Console, and No Buttons for a Record

| | |
|---|---|
| **Date** | 2026-10-12 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `nowplayd`, `lightd` and the console on the workload; the cover screen's firmware; the tenant's Terraform (one grant on the console's account) |
| **How** | `make plan apply` in `infra/deevnet-tenant-eds`; `make -C deploy deploy WHERE=builder`; `make flash` in `firmware/cover-screen` |
| **Risk** | Medium. What lights the room changes: the stand dims by itself for the first time. Most likely to go wrong: the stand dim when something is playing, or lit when nothing is |
| **Decisions** | [ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/) and [ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/), which this is the evidence for |
| **Related** | [CHG-0015](/docs/changes/2026/0015-the-turntables-power-beacon/), on whose branch this is built and whose console change this adds to |

---

## Summary

Three things, asked for together.

- A track paused for fifteen seconds is no longer said to be playing. The
  console says nothing is, and the stand goes dim and warm.
- The cover screen is shown on the console, beside the stand and the beacon.
- While a record is playing the cover screen shows the sleeve and no
  buttons, whatever is touched.

## Goal

- iTunes paused: fifteen seconds later the stand is dim, and the console
  shows nothing playing and the stand dim.
- Play pressed on the cover screen: iTunes plays and the stand is lit again.
- The console lists `cover-screen-01`.
- With the turntable current, a touch on the screen raises nothing.

## Scope

**In scope:** `nowplayd`, `lightd`, the console, the cover screen's
firmware, the console's broker account.

**Out of scope:** the stand's firmware (the idle scene is an ordinary
scene), the agent Pi and its drivers, the beacon, `palette`.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The console is refused its new subscription and does not start | the broker | the tenant is applied before the console is deployed |
| The plan changes something other than one grant | the tenant | read before it is applied: one account changed in place, nothing added, replaced or destroyed |
| The stand goes dim while something plays | the room | idle is decided only for a paused winner or none; a playing or held source overtakes it; `NP_IDLE_AFTER=0s` turns it off |
| The stand stays lit when lightd missed the topic being emptied | the workload | lightd takes five seconds of no cover after subscribing as none |
| The screen's panel or touch settings are lost in the build | the board | the build's settings were compared with the ones the board was last flashed from: both are the defaults |
| The wrong board is flashed | the Mac, with the stand and the screen both attached | the board on the port is asked its project name before it is written |

## Procedure

1. **The tenant.** `make plan`, read it, `make apply`.
2. **The workload.** `make -C deploy deploy WHERE=builder`.
3. **The screen.** `make flash PORT=/dev/cu.usbserial-2120`. Its credentials
   are not touched.

The screen and the workload are safe in either order: the old firmware
ignores the new field, and the new firmware offers its buttons until told
otherwise.

## Verification

- `nowplayd`'s log says "idle" fifteen seconds after a pause, and `lightd`'s
  a scene with `idle=true`.
- `eds/nowplaying/current/art` is empty then, and the stand's `state` says
  solid at 0.15.
- The console: "Nothing is playing", the stand drawn dim, and a pill for the
  screen.
- The screen's serial log says "no controls" for the turntable's track.

## Undo

- **Idle:** `NP_IDLE_AFTER=0s` in `deploy/nowplayd.env`, and
  `make -C deploy units`.
- **The buttons:** remove `NP_NO_CONTROLS_SOURCES` the same way; the
  firmware need not be changed back.
- **Everything:** deploy the commit before this one, and flash the screen
  from it.

## Outcome

| When (UTC) | What happened |
|---|---|
| 12:12 | The tenant was planned: 0 to add, 1 to change in place (the console's account, one subscription), 0 to destroy. The apply was left to its owner. |
| 12:12 | The board on `/dev/cu.usbserial-2120` said it was `cover-screen`, and was flashed with `6dab15e`. It started, found its credentials, connected, and read the current track: stopped, no cover. The stand, on the other port, was not touched. |
| 12:18 | Its owner had applied the tenant, and the services were deployed from the Builder (`049e1e6`). All four came back active. The console listed `cover-screen-01`, online, beside the beacon. `nowplayd` said it was telling controllers to offer nothing for the turntable. |
| 12:18 | What was current was the turntable, held and silent, its beacon on. So idle had nothing to act on: the track paused in iTunes was not the winner, a held source does not go idle, and the stand was lit at full from a cover still retained from before. Nothing was playing and the room did not say so. |

## Departures

- The screen was flashed before the tenant was applied and the workload
  deployed, not after. Either order is safe, and the board was to hand.

## Follow-ups

- [ ] The evidence the two decisions ask for.
- [x] A turntable left on with no record kept the last sleeve lit for as
  long as it was on, and was seen doing it within a minute of the deploy.
  Its owner chose five minutes, and it is `NP_IDLE_AFTER_STOPPED`.

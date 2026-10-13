---
title: "CHG-0020: The Stand Starts Dark, Blinks, Opens and Scans"
weight: 20
---

# CHG-0020: The Stand Starts Dark, Blinks, Opens and Scans

| | |
|---|---|
| **Date** | 2026-10-13 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The LP stand (`lp-stand-01`), flashed; `nowplayd`, `lightd` and the console on the workload |
| **How** | `make build flash` in `firmware/lp-stand`; `make -C deploy ship WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: a stand flashed with something that does not start, which is put right by flashing the build before it; or an opening that is too much for a living room |
| **Decisions** | [ADR-0029](/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong/) and [ADR-0030](/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source/), which this is the evidence for |
| **Related** | [CHG-0019](/docs/changes/2026/0019-the-stand-is-for-records/), which made the room dark and left the stand's own start dim |

---

## Summary

Three things asked for in one afternoon, and one flash.

- The stand is dark from power on, and blinks its first LED red when it
  cannot reach the broker: one to four blinks for how far it got.
- Between dark and lit, the strip opens from its middle and closes to it.
- A sixth effect, `scanner`, and a look of that name for the theme from
  *Knight Rider*, from any source.

## Goal

- Power on: dark, then the scene the stand is sent.
- The turntable switched on: the strip opens to dim. Off: it closes.
- The theme played from iTunes with the turntable off: the strip opens, one
  red eye goes from side to side, and the strip closes when the track ends.

## Scope

**In scope:** the stand's firmware; `nowplayd` (a track's look, and which
looks are for any source); `lightd` (the `scanner` look); the console's port
of the effects.

**Out of scope:** the cover screen and the beacon, which share the stand's
WiFi code and none of this; `palette`; the tenant. The scene descriptor is
still version 1: an effect name is added, and nothing else.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The flash writes over the stand's credentials | the stand | it does not: the `creds` partition is not in the flash arguments, and the partition table is unchanged |
| The wrong board is on the serial port | the bench | the chip and the name of the application in flash are read first; the cover screen is an ESP32 too |
| The services are deployed before the stand is flashed | the room | the stand shows `scanner` as `solid` red until it is; so the stand goes first |
| A fault blinks in the room at night | the room | one LED; and it is saying something true |

## Procedure

1. The stand, on the Mac's serial port: read the chip and the application's
   name; `make build flash`; watch it start.
2. The workload: `make -C deploy ship WHERE=builder`.

## Verification

- The stand's serial log: credentials read, WiFi joined, broker connected,
  its retained scene received, and no fault said.
- `nowplayd` and `lightd` active, and the console's page drawing a stand
  sent `scanner` as a moving eye.

## Undo

- **The stand:** flash the commit before this one.
- **The services:** deploy the commit before this one. A `scanner` scene
  left retained is replaced by the next thing `lightd` sends.

## Outcome

| When (UTC) | What happened |
|---|---|
| 16:47 | The board on `/dev/cu.usbserial-0001` was an ESP32-D0WD-V3 whose application in flash was `lp-stand`, built from `9d39658` on 2026-10-11. |
| 16:48 | Flashed from the worktree: the bootloader, the OTA data and the application, each verified. |
| 16:49 | Reset and watched for 25 seconds. Credentials read from `creds`, all of them provisioned; joined `DVNTM-IOT` in 1.9 seconds; connected to the broker over TLS at 3.6; received its retained scene, solid at brightness 0. No fault was said. |
| 16:52 | The workload was shipped (`90d85f3`). All four services came back active; with every source offline `nowplayd` said stopped, then idle fifteen seconds later, and the stand was sent dark and said it had applied it. |
| 16:52 | A `scanner` scene, red, sent to the stand by hand through `lightd`'s `POST /v1/scene`. The stand applied it - and a second and a half later `nowplayd`'s idle arrived and `lightd` sent dark over it, as it should. |
| 16:52 | Sent again once that had passed. The stand reported `scanner` at brightness 1 for twelve seconds, then solid at 0 when it was sent dark again. |

## Departures

- The stand was flashed before the change was committed or reviewed, on its
  owner's word that it was plugged in and flashable.
- The stand's `sdkconfig` is not in the repository. The one in the main
  checkout was copied into the worktree, so the build is of the same
  settings as the one it replaces.

## Follow-ups

- [ ] The theme itself: every source was offline, so `nowplayd` has not yet
  named the track and published the look. The scanner was sent by hand.
- [ ] Everything ADR-0029 and ADR-0030 list as not yet shown. None of the
  blinks, the opening and closing, or the scanner has been looked at by a
  person.

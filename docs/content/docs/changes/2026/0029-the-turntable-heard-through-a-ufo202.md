---
title: "CHG-0029: The Turntable Heard Through a UFO202"
weight: 29
---

# CHG-0029: The Turntable Heard Through a UFO202

| | |
|---|---|
| **Date** | 2026-10-21 |
| **Change type** | Configuration |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `dv02rpi003p01`, the back-end Pi: one line of its agent's settings, and its agent restarted |
| **How** | `make -C deploy/backend turntable DEVICE=plughw:CODEC PI=192.168.2.40` |
| **Risk** | Low. Most likely to go wrong: the agent not coming back, which would stop iTunes being followed too |
| **Decisions** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/), unchanged |
| **Related** | [CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/), which named `plughw:UA4FX` |

---

## Summary

The turntable was heard through a Roland UA-4FX, and the Pi's agent listened
to `plughw:UA4FX`. Its owner found a Behringer UFO202 and plugged it into the
Pi in the UA-4FX's place. ALSA names it
`CODEC` (card 3, *USB Audio CODEC*), and the UA-4FX is no longer there, so the
agent is told the new name. Nothing else changes: the same line-level feed
from the selector, the same driver, the same recognizer.

The repository's examples, the status scripts' comments and the system
picture name the UFO202 in the same commit.

## Goal

- The Pi's settings say `NP_TURNTABLE_DEVICE=plughw:CODEC`, and `npagent` is
  active.
- `eds-backend-status` says `listening to plughw:CODEC`.
- A record played is reported by the turntable driver.

## Procedure

1. `arecord -l` on the Pi: the UFO202 is card `CODEC`.
2. `make -C deploy/backend turntable DEVICE=plughw:CODEC PI=192.168.2.40`.
3. `make -C deploy/backend status PI=192.168.2.40`.

## Undo

Plug the UA-4FX back in, and
`make -C deploy/backend turntable DEVICE=plughw:UA4FX PI=192.168.2.40`.

## Outcome

| When (UTC) | What happened |
|---|---|
| 16:55 | `arecord -l` on the Pi: one capture card, `card 3: CODEC [USB Audio CODEC]`. The UA-4FX is gone; the settings still said `plughw:UA4FX`. |
| 16:56 | `turntable DEVICE=plughw:CODEC`. `npagent` restarted, active at 16:56:29, `NP_TURNTABLE_DEVICE=plughw:CODEC`. The driver logged `listening to the turntable device=plughw:CODEC`, was online, and reported the turntable stopped. |
| 16:57 | `make status`: `listening to plughw:CODEC`, recognizer installed, `amp.shazam.com:443` reachable. |

Still to see: a record played, reported by the turntable driver.

## Follow-ups

- None.

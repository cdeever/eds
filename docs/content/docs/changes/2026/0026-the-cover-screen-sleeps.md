---
title: "CHG-0026: The Cover Screen Sleeps"
weight: 26
---

# CHG-0026: The Cover Screen Sleeps

| | |
|---|---|
| **Date** | 2026-10-17 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `cover-screen-01` |
| **How** | `make build ota` in `firmware/cover-screen` |
| **Risk** | Low. Most likely to go wrong: the screen sleeping when someone is there, because a source that should count does not name itself in `current` |
| **Decisions** | [ADR-0036](/docs/architecture/decisions/0036-the-screen-sleeps-when-nothing-plays/) |
| **Related** | [CHG-0025](/docs/changes/2026/0025-presence-repeated-and-updates-through-the-broker/), which made updates over the air possible |

---

## Summary

The cover screen is lit all the time, showing a drawn record to nobody when
nothing plays. After this, it goes dark five minutes after `current` last
said something was playing, and lights again for music, a touch, or a lost
broker.

Firmware only, over the air. Nothing on the bus, in the services or in the
tenant changes.

## Goal

- `make ota` in `firmware/cover-screen` ends `kept:`, and the screen's
  `system.boot` names the new build.
- With nothing playing, a `screen.asleep` event five minutes after `current`
  last said `playing`, and the panel dark; the screen still `online` on the
  console.
- A touch on the dark screen lights it, with no buttons, and `screen.awake`.

## Scope

**In scope:** `cover-screen-01`'s firmware.
**Out of scope:** the stand and the beacon, which are unchanged.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The new build does not reach the broker | the screen | ESP-IDF's rollback: the old build boots again within two minutes ([ADR-0035](/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker/)) |
| It sleeps while something plays | the screen | A touch wakes it; the rule is host tested; the old build goes back on over the air |
| It does not wake from the panel's sleep | the screen | A power cycle; the old build over the air |

## Procedure

### Step 1: Send the build

The screen restarts once, a few seconds dark.

**Run:**

```bash
cd firmware/cover-screen
. ~/.espressif/tools/activate_idf_v6.1.sh
make build ota
```

**Verify:**

1. `eds-ota` ends `kept:`.
2. `make logs`, or the console: the screen `online`, a `system.boot`
   naming the new build.

### Step 2: Sleep and wake

**Verify:**

1. iTunes paused: five minutes and some seconds later the screen is dark,
   with a `screen.asleep` event.
2. Play: lit, with the cover, and `screen.awake`.
3. A touch on the dark screen lights it with no buttons; it is dark again
   five minutes later.

## Verification

ADR-0036's checks on the board: asleep after a pause, woken by playing,
woken by a touch.

## Undo

`make build ota` from the commit before this one, in the same directory.
Nothing outside the screen changed.

## Outcome

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| 2026-10-17 13:38 UTC | 1 | `make build ota` from `7dd2035`: `kept` in 17 s. In the screen's events: `ota.offered`, `ota.applied`, `system.boot` (`software`, firmware `7dd2035`), `ota.kept`, `mqtt.connected`. |
| 2026-10-17 13:44 UTC | 2 | `screen.asleep` at 303.6 s of uptime: five minutes after `current`, heard moments after connecting at 3.2 s, named no source. A `screen.health` 2 s before it, and no `mqtt.disconnected` after: asleep, still on the broker. Dark, the owner confirmed. |
| 2026-10-17 13:51 UTC | 2 | A song started in iTunes: `screen.awake`, the cover shown. Then paused: the record shown, and the screen stayed lit, as built: after fifteen seconds `current` says stopped with iTunes still named (ADR-0024), and the first build slept only when no source was named. The owner expected it to sleep; the rule was widened to nothing playing (ADR-0036 revised before merging). |

### Departures from the plan

- The first build slept only with nobody there, and a paused iTunes kept
  it lit. A second build, sleeping when nothing plays, was sent under this
  record.

## Follow-ups

- [ ] ADR-0036's evidence, into the record.

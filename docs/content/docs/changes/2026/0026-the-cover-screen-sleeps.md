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
| **Decisions** | [ADR-0036](/docs/architecture/decisions/0036-the-screen-sleeps-when-nobody-is-there/) |
| **Related** | [CHG-0025](/docs/changes/2026/0025-presence-repeated-and-updates-through-the-broker/), which made updates over the air possible |

---

## Summary

The cover screen is lit all the time, showing a drawn record to nobody when
everything is off. After this, it goes dark five minutes after `current`
names no source, and lights again for a source, a touch, or a lost broker.

Firmware only, over the air. Nothing on the bus, in the services or in the
tenant changes.

## Goal

- `make ota` in `firmware/cover-screen` ends `kept:`, and the screen's
  `system.boot` names the new build.
- With nobody there, a `screen.asleep` event five minutes after `current`
  last named a source, and the panel dark; the screen still `online` on the
  console.
- A touch on the dark screen lights it, with no buttons, and `screen.awake`.

## Scope

**In scope:** `cover-screen-01`'s firmware.
**Out of scope:** the stand and the beacon, which are unchanged; Spotify's
presence, which would keep the screen awake and is off
([CHG-0024](/docs/changes/2026/0024-spotify-turned-off-again/)).

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The new build does not reach the broker | the screen | ESP-IDF's rollback: the old build boots again within two minutes ([ADR-0035](/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker/)) |
| It sleeps while someone is there | the screen | A touch wakes it; the rule is host tested; the old build goes back on over the air |
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

With iTunes unreachable and the turntable switched off, `current` names no
source.

**Verify:**

1. Five minutes later the screen is dark, with a `screen.asleep` event.
2. A touch lights it with no buttons, and `screen.awake`; it is dark again
   five minutes later.
3. The turntable switched on: the screen lights, with a record.

## Verification

ADR-0036's checks on the board: asleep, woken by a source, woken by a touch.

## Undo

`make build ota` from the commit before this one, in the same directory.
Nothing outside the screen changed.

## Outcome

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| 2026-10-17 13:38 UTC | 1 | `make build ota` from `7dd2035`: `kept` in 17 s. In the screen's events: `ota.offered`, `ota.applied`, `system.boot` (`software`, firmware `7dd2035`), `ota.kept`, `mqtt.connected`. |
| 2026-10-17 13:44 UTC | 2 | `screen.asleep` at 303.6 s of uptime: five minutes after `current`, heard moments after connecting at 3.2 s, named no source. A `screen.health` 2 s before it, and no `mqtt.disconnected` after: asleep, still on the broker. Not yet seen by eye; the touch and the turntable not yet tried. |

### Departures from the plan

-

## Follow-ups

- [ ] ADR-0036's evidence, into the record.

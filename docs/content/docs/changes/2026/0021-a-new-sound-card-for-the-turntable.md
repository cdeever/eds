---
title: "CHG-0021: A New Sound Card for the Turntable"
weight: 21
---

# CHG-0021: A New Sound Card for the Turntable

| | |
|---|---|
| **Date** | 2026-10-13 |
| **Change type** | Configuration |
| **Status** | {{< status-badge "complete" "Complete" >}} |
| **Systems** | `npagent` on the agent Pi (`np-relay`) |
| **How** | `make -C deploy/relay turntable DEVICE=plughw:CODEC` |
| **Risk** | Low. One line of the agent's settings, and a restart of the agent |
| **Decisions** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/) |
| **Related** | [CHG-0013](/docs/changes/2026/0013-the-turntable-driver/), which named the first input; [CHG-0019](/docs/changes/2026/0019-the-stand-is-for-records/), since which only a record lights the stand |

---

## Summary

The sound card the turntable is heard through was replaced with another USB
adapter. The agent was still told to listen to the old one by name,
`plughw:UA4FX`, which was no longer there, so the turntable's driver heard
nothing and reported nothing. With the stand lit for records only, that was
a stand that did nothing for a record.

## Procedure

1. `make -C deploy/relay status`, to see what the Pi had: the turntable's
   line said `plughw:UA4FX is not plugged in`, and listed the cards present.
   The new one is `CODEC`.
2. `make -C deploy/relay turntable DEVICE=plughw:CODEC`.

## Verification

- `status` says `listening to plughw:CODEC`.
- The agent names what is playing, and `nowplaying/current` is the
  turntable's.

## Undo

`make -C deploy/relay turntable DEVICE=plughw:UA4FX`, with that card back.

## Outcome

| When (UTC) | What happened |
|---|---|
| 21:20 | The turntable was switched on: the beacon said so, and the stand opened to dim. A record played and the stand stayed dim. |
| 21:25 | `status`: `plughw:UA4FX is not plugged in`, `arecord` unable to open it, and the cards present were `Headphones`, `vc4hdmi0`, `vc4hdmi1` and `CODEC`. iTunes' computer was not answering either. |
| 21:25 | The input was set to `plughw:CODEC` and the agent restarted. |
| 21:26 | The agent said it was playing The Doors, "The WASP (Texas Radio And The Big Beat)", from *L.A. Woman*; `nowplayd` made it current with the sleeve from the owner's collection; the stand reported a chase in four colors at 0.8. iTunes' driver came online in the same minute, stopped. |

## Departures

- Done before it was written down, at the owner's suggestion that the Pi
  needed something for the new adapter.

## Follow-ups

- [ ] The new card's level in a gap between tracks has not been measured.
  The driver takes a gap to be quieter than -55 dBFS
  ([ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/)),
  and that was a number for the old card.
- [ ] A card's ALSA name changes with the card. Nothing says so until
  `status` is read; the driver's `turntable.capture_lost` event is in the
  log store, and no dashboard shows it.

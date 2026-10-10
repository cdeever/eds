---
title: "ADR-0002: Gamma Belongs to the Firmware"
weight: 2
---

# ADR-0002: Gamma Belongs to the Firmware, Not the Wire

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-09-07 |
| **Scope** | Which layer makes which colour decision between a cover image and the strip. |
| **Backfilled** | Decided with the first vertical and argued in the project READMEs and [Boundaries](/docs/architecture/boundaries/); written up here on 2026-10-10. |
| **Related** | [ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/) |

---

## Context

A colour passes through three programs on its way from a sleeve to a strip:
`palette` extracts it, `lightd` turns swatches into a scene, and the firmware
drives the LEDs. Several corrections have to happen somewhere along the way —
rejecting dull swatches, boosting chroma, mapping into the strip's gamut, and
gamma, because WS2812 output is linear in duty cycle and perception is not.

## Options considered

**Correct everything in `lightd`.** One place, easy to tune. But the right
gamma curve is a property of one driver and one diffuser, so `lightd` would
need to know which hardware each stand has, and the colours on the wire would
no longer be the colours intended.

**Correct everything in `palette`.** It already does colour maths. But then it
could not be tuned against a folder of JPEGs with no hardware in mind.

**Split by what each layer knows.**

## Decision

Each layer owns the decisions it has the knowledge for, and is deliberately
ignorant of the layer below:

- **`palette`** knows nothing about music, MQTT or LEDs. Image bytes in,
  perceptual colour data out. No role assignment, no LED correction.
- **`lightd`** owns the hardware-facing colour choices that are the same for
  any strip: rejecting low-chroma swatches, chroma boost in OKLab, and gamut
  mapping by walking chroma down, never by clipping channels.
- **The firmware** owns gamma.

## Consequences

- `palette` can be developed and tested with no infrastructure at all.
- What `mosquitto_sub` shows is the colour actually intended.
- A stand with a different strip or diffuser needs only its own curve.
- OKLab conversion is implemented three times, in Python, Go and C. A change
  to it in one place means checking the other two.
- New knowledge has to be placed deliberately. Track and album metadata, for
  instance, belong to none of these three layers
  ([ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/),
  [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)).

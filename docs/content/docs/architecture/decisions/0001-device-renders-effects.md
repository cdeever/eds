---
title: "ADR-0001: The Stand Renders Effects"
weight: 1
---

# ADR-0001: The Stand Renders Effects Locally and Never Receives Frames

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-09-07 |
| **Scope** | What travels from `lightd` to a stand, and where animation happens. |
| **Backfilled** | Decided with the first firmware and argued in `firmware/lp-stand/README.md`; written up here on 2026-10-10. |
| **Related** | [ADR-0002](/docs/architecture/decisions/0002-gamma-in-firmware/), [Scene Descriptor](/docs/reference/scene-descriptor/) |

---

## Context

The stand is an ESP32 driving an addressable strip over WiFi from a broker it
does not control. The light has to look deliberate at all times, including
when the network is slow or gone, and the stand is meant to sit on a shelf
unattended.

## Options considered

**Stream frames.** The backend computes every pixel and sends frames; the
device is a dumb display. Any effect can be added without touching firmware.
But WiFi latency turns directly into visible stutter, a broker outage freezes
the strip mid-animation, and sixty frames a second per stand is a lot of
traffic for a light.

**Send a scene, render locally.** The backend sends a small description — an
effect name, a speed, a brightness, a weighted palette — and the device
animates it. A new effect means reflashing every stand.

## Decision

The stand subscribes to a **scene descriptor** and renders it locally at
60 fps. It never receives frames.

## Consequences

- A stand whose network has gone away keeps animating the last scene.
- The scene is retained on the broker, so a stand that reboots picks the
  record back up. That retention is load-bearing and has an integration test.
- The payload is small enough to read by eye in `mosquitto_sub`.
- **Adding an effect means reflashing every stand**, so effects are kept
  deliberately few (`solid`, `breathe`, `sweep`), and an unknown effect name
  falls back to `solid` rather than being refused: a publisher will learn a
  new effect before the stands do.
- The scene descriptor becomes a versioned contract between Go and C, and a
  mismatched version is refused rather than guessed at.

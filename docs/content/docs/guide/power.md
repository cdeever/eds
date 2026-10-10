---
title: "Powering the Strip"
weight: 3
---

# Powering the Strip

WS2812-class LEDs draw about **60 mA each at full white**, so a long strip at
full brightness is an amp-scale load. The ESP32 devkit adds roughly 250 mA with
WiFi up and peaks near 500 mA on transmit.

## What EdS actually draws

Brightness is applied **before** the gamma 2.6 table, which means current falls
away far faster than the brightness figure suggests. Measured against the
firmware's own lookup table:

| Scenario | mA/LED | 8 | 16 | 30 | 60 |
|---|---:|---:|---:|---:|---:|
| Boot default (solid warm white, brightness 0.15) | 1.2 | 10 | 20 | 37 | 74 |
| lightd default, breathe peak on orange | 10.3 | 82 | 164 | 308 | 615 |
| Saturated red at brightness 0.8 | 12.2 | 98 | 195 | 366 | 733 |
| Worst case: full white, brightness 1.0 | 61.0 | 488 | 976 | 1830 | 3660 |

The boot row is so small because the default scene's 0.15 brightness on
(255, 170, 90) becomes PWM duties of 2, 1 and 0 out of 255. Almost all of that
current is the controller ICs idling. The "unconfigured stand shows warm white"
behaviour is effectively free even on a full strip.

## Bench testing on USB

**16 LEDs** is a comfortable working figure: even at the breathe peak that is
~164 mA of strip and ~420 mA total.

**8 LEDs** is unconditionally safe — worst case is 488 mA of strip even if a
scene arrives at full white and brightness 1.0, so nothing published can
overload the port.

**Past ~30 LEDs, use an external 5 V supply** with its ground tied to the
ESP32.

> [!NOTE]
> The host's USB port is rarely the fragile part — it current-limits and disables
> on overload. The devkit is: powering a strip from its 5 V pin routes all of
> that current through the USB connector and a small onboard diode often rated
> around 500 mA to 1 A.

## Wiring

- **330–470 Ω in series on the data line**, at the ESP32 end.
- **1000 µF across 5 V and ground** at the strip input, to absorb the inrush
  when many LEDs switch at once.
- **Common ground** is mandatory when using an external supply.
- The ESP32 drives 3.3 V data into 5 V-logic WS2812s. This usually works and is
  technically out of spec; flicker on the first pixel is the symptom.

## Limiting count in firmware

`LP_LED_COUNT` is compile-time — `main.c` declares `static lp_rgb_t
frame[LP_LED_COUNT]` — so changing it needs `menuconfig`, a rebuild and a
reflash.

A strip longer than the configured count is safe and useful for bench work: the
driver only ever writes `max_leds` pixels, so a 60-LED strip configured as 16
lights the first 16 and leaves the rest dark and idle.

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
| Waiting for a record (solid warm white, brightness 0.15) | 1.2 | 10 | 20 | 37 | 74 |
| lightd default, breathe peak on orange | 10.3 | 82 | 164 | 308 | 615 |
| Saturated red at brightness 0.8 | 12.2 | 98 | 195 | 366 | 733 |
| Worst case: full white, brightness 1.0 | 61.0 | 488 | 976 | 1830 | 3660 |

The first row is so small because the idle scene's 0.15 brightness on
(255, 170, 90) becomes PWM duties of 2, 1 and 0 out of 255. Almost all of that
current is the controller ICs idling. It is what a stand is sent while the
turntable is on and silent. At power on the strip is dark
([ADR-0029](/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong/)),
and a stand that cannot reach the broker lights one LED, red, in blinks.

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

One 5 V supply runs both the strip and the board:

```
5 V supply (+) ──┬── strip +5V
                 └──▶|── ESP32 5V / VIN pin
                    1N5817, stripe towards the board

5 V supply (−) ──┬── strip GND
                 └── ESP32 GND pin

ESP32 GPIO 18 ──[330–470 Ω]── strip DIN
```

**Data is GPIO 18.** That is `LP_LED_GPIO`, whose default is 18; it is changed
in `menuconfig` under Strip → Data GPIO. It is the only GPIO the strip needs:
a WS2812 has no clock line.

Branch at the supply or at the strip's input, so the strip's current does not
pass through the board. The board takes its 5 V on the pin labeled 5V, VIN or
VBUS, which feeds the onboard regulator. **Never the 3V3 pin.**

For the stand's 18 LEDs the worst case is 18 × 61 mA, about 1.1 A, plus the
board: a **5 V 2 A** supply covers it with room to spare.

**The diode is for reflashing in place.** With USB plugged in and the supply
off, the strip would otherwise draw from the USB port, backward through the
board's 5 V pin. The diode lets current into the board and none out, so USB
powers the board alone. Use a Schottky (1N5817 to 1N5819): it drops about
0.3 V, which still leaves the regulator enough. A 1N4001 drops too much.

It does not protect the other direction. What keeps the supply out of the USB
port is the devkit's own diode between the port and the 5 V pin, which most
have. On a board without one, lift the wire from the 5 V pin before plugging
USB in and leave ground attached.

- **330–470 Ω in series on the data line**, at the ESP32 end.
- **1000 µF across 5 V and ground** at the strip input, to absorb the inrush
  when many LEDs switch at once.
- **Common ground** is mandatory when using an external supply.
- The ESP32 drives 3.3 V data into 5 V-logic WS2812s. This usually works and is
  technically out of spec; flicker on the first pixel is the symptom.

## Limiting count in firmware

`LP_LED_COUNT` is 18, the number that fit across the front of the stand. It
is compile-time — `main.c` declares `static lp_rgb_t
frame[LP_LED_COUNT]` — so changing it needs `menuconfig`, a rebuild and a
reflash.

A strip longer than the configured count is safe and useful for bench work: the
driver only ever writes `max_leds` pixels, so a 60-LED strip configured as 16
lights the first 16 and leaves the rest dark and idle.

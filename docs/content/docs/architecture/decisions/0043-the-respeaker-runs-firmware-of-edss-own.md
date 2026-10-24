---
title: "ADR-0043: The reSpeaker Runs Firmware of EdS's Own"
weight: 43
---

# ADR-0043: The reSpeaker Runs Firmware of EdS's Own, and Sends a Request When Its Button Is Held, Then When It Hears Its Name

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a request said into the reSpeaker has played its album; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-24 |
| **Scope** | What runs on the reSpeaker Lite's XIAO ESP32-S3: how it hears, when it sends, and how it is provisioned and updated. Not what a request becomes, which is [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/)'s. |
| **Extends** | [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/), whose `voice/<device>/utterance` this sends |
| **Related** | [ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/), [ADR-0035](/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker/), [ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/) |

---

## Context

The reSpeaker Lite is two chips. An XMOS XU316 takes two microphones and
gives one cleaned-up voice: echo cancelled, noise suppressed, beam steered.
Its firmware (1.0.8, read over DFU on 2026-10-24) sends that over I2S to a
XIAO ESP32-S3 (8 MB PSRAM, 8 MB flash), which came with an Arduino test
sketch from Seeed. The XIAO is what can be programmed.

## Options considered

**ESPHome**, which Seeed and Home Assistant publish for the board. Its voice
pipeline is Home Assistant's, and its microWakeWord is the best on this
chip; the rest would be a second way of doing what EdS's three devices
already do.

**Firmware of EdS's own**, in ESP-IDF and C, like the stand, the screen and
the beacon: credentials in NVS (ADR-0004), updates through the broker
(ADR-0035), presence repeated (ADR-0034), events to the tenant's log store.
The wake word is the one new thing, and microWakeWord is a TensorFlow Lite
model that runs without ESPHome.

## Decision

**The reSpeaker runs EdS firmware, `firmware/ears`. It sends what it heard
as one WAV on `voice/respeaker-01/utterance`: first while a button is held,
then after a wake word.**

- **It listens to the XMOS chip over I2S**, the XMOS driving the clock. The
  firmware measures the rate it is given and says it, and makes 16 kHz mono
  16-bit from whichever channel carries the voice, as ADR-0042 asks.
- **Held button first.** The XIAO's own button sends what was said while it
  was held, up to fifteen seconds. Every part of the path can be shown from
  the device before a wake word exists, and the button stays as the way to
  ask when the room is too loud for one.
- **Then a wake word**, microWakeWord with a stock model, then "Hey Ed":
  what follows it, until a pause, is sent the same way.
- **The rest is the screen's, copied**: the `creds` partition and `make
  provision` from the tenant's outputs, WiFi on the IoT segment, presence on
  `voice/respeaker-01/status`, events on `log/respeaker-01`, two OTA slots
  with rollback in the bootloader from the first flash.
- **What was heard is answered with a light**: the XIAO's LED while it
  listens and sends, and a pattern for "not understood".

## Consequences

- **Four firmwares**, sharing copied files, as the three before do.
- **A request is audio on the IoT segment** for as long as it takes to send,
  as ADR-0042 already accepts.
- **The XMOS chip is left as Seeed made it.** Its firmware is updated, if
  ever, over its own USB port with `dfu-util`.

## What would settle it

- [x] The rate and channel the XMOS chip sends, measured: 16 kHz on BCLK 8,
      WS 7, DIN 44, the voice the same on both channels (2026-10-24).
- [x] A request said into it plays its album: asked from the console, not a
      button, since the voice kit's buttons are the XMOS chip's; *Aerosmith*,
      2026-10-24 ([CHG-0035](/docs/changes/2026/0035-only-words-scribe-is-sure-of/)).
- [ ] The same after a wake word.
- [ ] An update through the broker, and a rollback.

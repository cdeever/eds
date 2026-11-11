---
title: "CHG-0049: The Ears Wake to \"Hey DJ Ed\""
weight: 49
---

# CHG-0049: The Ears Wake to "Hey DJ Ed"

| | |
|---|---|
| **Date** | 2026-11-11 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The ears (`respeaker-01`); scribe and voiced on the services workload |
| **How** | `make -C tools/wakeword install`; `make build flash monitor` in `firmware/ears`, by cable; `make -C deploy deploy WHERE=builder` for scribe and voiced |
| **Risk** | Low. The board keeps its credentials and its old slot. Most likely to go wrong: the model's arena does not fit, and the ears log it and do not wake; the BOOT button still asks |
| **Decisions** | [ADR-0054](/docs/architecture/decisions/0054-the-ears-wake-to-hey-dj-ed-trained-here/) |
| **Related** | [CHG-0036](/docs/changes/2026/0036-the-respeaker-wakes-to-hey-jarvis/), whose "Hey Jarvis" this replaces; [CHG-0039](/docs/changes/2026/0039-scribe-in-two-and-hey-jarvis-in-its-prompt/), whose prompt this changes |

---

## Summary

The ears woke to ESPHome's "Hey Jarvis" (CHG-0036). They wake to "Hey DJ
Ed", the first model `tools/wakeword` trained (ADR-0054): cutoff 0.96, 245
of 255, which missed 12% of synthetic phrases and woke falsely never in
about five hours of party audio. scribe's prompt starts "Hey DJ Ed." in
place of "Hey Jarvis.", and voiced takes "Hey DJ Ed", however "DJ" is
written, as the wake word before a request; "Jarvis" is still taken, for
a board that rolls back.

## Goal

- The ears' boot log names "Hey DJ Ed", with the arena it uses.
- "Hey DJ Ed, play Aerosmith's first album", said across the room, plays it.
- scribe and voiced on the workload are the ones built from this change.

## Scope

**In scope:** the ears' firmware and Kconfig, scribe's prompt, voiced's
grammar, the docs that name the wake word.
**Out of scope:** another model, and the room's music in training
(ADR-0054's later steps); the stand, the screen and the beacon.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The arena is too small for this model | `app/wake` at boot | It says so and the ears do not wake; BOOT still sends. Raise `EA_WAKE_ARENA` |
| It wakes too often to the room | the room, ADR-0044's ducking | Raise the cutoff; the week of ADR-0054 counts `voice.woken` |
| scribe and voiced wait for builder | the workload | The device keeps 600 ms before its wake word, mostly "…Ed"; the voiced running takes a word or two before "play" |

## Procedure

### Step 1: The model into the firmware

`make install` from the run on the external disk: `hey_dj_ed.tflite`
(62,304 bytes, SHA-256 `479c031a…`) and `hey_dj_ed.json`. `CMakeLists.txt`
embeds it, `app/wake` names it, Kconfig's word is "Hey DJ Ed" and its cutoff
245. The board's own `sdkconfig` is gitignored and set the same by hand.

### Step 2: Flash the ears, by cable

```bash
. ~/.espressif/tools/activate_idf_v6.1.sh
make -C firmware/ears build flash monitor
```

### Step 3: scribe and voiced

```bash
make -C deploy deploy WHERE=builder
```

## Log

| Time | What happened |
|---|---|
| 2026-11-11 | Built: `_binary_hey_dj_ed_tflite_start` in `ears.elf`. Host tests 58 of 58, nowplaying's check and scribe's tests pass. builder did not answer, so scribe and voiced wait. |

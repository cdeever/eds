---
title: "CHG-0036: The reSpeaker Wakes to \"Hey Jarvis\""
weight: 36
---

# CHG-0036: The reSpeaker Wakes to "Hey Jarvis", and Hears a Request Until It Ends

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `respeaker-01`: firmware flashed by cable. The services workload: `eds-voiced` rebuilt with the others and all restarted |
| **How** | `make -C firmware/ears build flash`; `make -C deploy ship WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: the device waking for something that is not its name, which sends a few seconds of the room; voiced acts only on a request it is sure of (INC-0001) |
| **Decisions** | [ADR-0043](/docs/architecture/decisions/0043-the-respeaker-runs-firmware-of-edss-own/) |
| **Related** | [CHG-0035](/docs/changes/2026/0035-only-words-scribe-is-sure-of/) |

---

## Summary

The reSpeaker listens for a wake word: microWakeWord's "Hey Jarvis", ESPHome's
model, on TensorFlow Lite Micro with the audio frontend it was trained on.
What follows it is heard until the speaker stops - 0.7 s quieter than the
room by 10 dB, at most six seconds - and sent, with the 350 ms before the
wake was decided on, so that a "play" said straight after keeps its P. voiced
allows a word or two before "play", for the end of the wake word that comes
with it, but never a negation.

Sending a request is written in 16 KB pieces, not the MQTT client's default
1 KB: a five-second request took 16 to 22 seconds to send before.

## Goal

- "Hey Jarvis … play Aerosmith's first album" plays it.
- The room's talk does not wake it, over an evening.
- A request is sent in under two seconds.

## Procedure

1. `make -C firmware/ears build flash`.
2. `make -C deploy ship WHERE=builder`.
3. The owner says it.

## Undo

Flash `273912b`'s firmware: the console asks, nothing listens for a name.

## Outcome

| When (UTC) | What happened |
|---|---|
| 02:09–02:13 | First flashed without the pre-roll and the buffer: the model loaded (22,580 of 45,720 arena bytes) and kept up; "Hey Jarvis" woke it at 251/255, nothing else in three minutes passed 1/255. The request ran the full eight seconds in a talking room, was sent in 16 s, and was written "Clay Aerosmith's first album": the P lost. Not a request; nothing played. |
| 02:16:00–02:16:39 | voiced shipped and the device flashed with the pre-roll, the six-second limit and the 16 KB buffer. |
| 02:17:04 | The owner, 24 s after the device started: woken at 253/255. The device had not yet reached the broker - its first connection waited out the 20-second network timeout set for sending - so the request was kept back: "no broker; the request was not sent". |
| 02:19:43 | The owner again: woken at 250/255, the request ended by the quiet after 2.3 s, sent. scribe wrote it "So this is what I think.", not confident, in 10 s: the owner was on a Zoom call beside the reSpeaker, and it was the call's voice. Nothing played. |
| 02:22:40–02:27:39 | The network timeout put back to the default (the buffer is what made sending fast); each request's console line says how it ended; scribe decodes once, without Whisper's warmer retries, which is where the ten seconds went. Same words and verdicts on every recording tried. Restarted, the device was on the broker in 5.4 s. |

Over the evening, with music and a Zoom call beside it, nothing but the
owner's "Hey Jarvis" passed 1/255.

### Departures from the plan

## Follow-ups

- "Hey Ed", trained.

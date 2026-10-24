---
title: "CHG-0035: Only Words scribe Is Sure Of"
weight: 35
---

# CHG-0035: Only Words scribe Is Sure Of, and the reSpeaker Asked From Its Console

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-24 |
| **Systems** | The services workload: the six services rebuilt, `eds-scribe` and `eds-voiced` changed, all restarted. `respeaker-01`: firmware flashed by cable |
| **How** | `make -C deploy ship WHERE=builder`; `make -C firmware/ears build flash`, from `e9ed753` on `voice-play-album` |
| **Risk** | Low. Most likely to go wrong: scribe's thresholds refusing speech it should take, which is a request said again, not a record played wrongly |
| **Decisions** | [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/), [ADR-0043](/docs/architecture/decisions/0043-the-respeaker-runs-firmware-of-edss-own/) |
| **Related** | [INC-0001](/docs/incidents/2026/0001-the-beatles-played-for-a-request-nobody-made/), which this corrects |

---

## Summary

The reSpeaker's first request played *Abbey Road*, which nobody asked for
([INC-0001](/docs/incidents/2026/0001-the-beatles-played-for-a-request-nobody-made/)):
scribe's model wrote its own prompt back, a request in words, and voiced
acted on it. scribe's prompt is now a list of names, and scribe says whether
the words are worth acting on, by Whisper's own thresholds; voiced acts on
nothing else.

The reSpeaker is asked from its USB console until it has a wake word: the
voice kit's buttons are the XMOS chip's, not the XIAO's.

## Goal

- Silence, noise and the request said outside the window are answered
  "not heard clearly", and nothing plays.
- A request said in the window plays its album.

## Procedure

1. `make -C deploy ship WHERE=builder`; `make -C deploy status`.
2. `make -C firmware/ears build flash`.
3. Enter on the console with nothing said: "not heard clearly".
4. Enter, then "Play Aerosmith's first album", said by the owner.

## Undo

Ship and flash `1badd8a`'s scribe and voiced, and `1c6c645`'s firmware;
which is to say, put INC-0001 back. Not done.

## Outcome

| When (UTC) | What happened |
|---|---|
| 01:46:01–01:49:27 | `ship WHERE=builder` from `e9ed753`: six images, all six restarted; no service timed out connecting. |
| 01:49:50 | `respeaker-01` flashed. |
| 01:50 | On the workload, scribe: the owner-free recording of "Play Aerosmith's first album" confident (no speech 0.005); five seconds of silence not, and no words. |
| 01:51:41 | Two windows asked for on the console. In the room, someone else was speaking: the first written as talk and not confident ("not heard clearly"), the second as clear talk that was no request ("not a request"). Nothing played. |
| 01:54:23 | The owner, in the window: "Play Aerosmith's first album." The WAV reached the broker whole (160,812 bytes, tapped with voiced's account); scribe wrote it confidently in 3.5 s; voiced asked for *Aerosmith* at 01:54:30.8; "Make It" played at 01:54:31.1. |

### Departures from the plan

- Step 3, nothing said, was the room's own talk instead of silence: a
  harder test, passed.
- The device took about 22 s to finish sending the second request, though
  voiced had it within five; its console never showed what was heard. Left
  as a follow-up.

## Follow-ups

- The wake word.
- The device's send: `esp_mqtt_client_publish` of 160 KB held the listening
  for ~22 s on a -69 dBm link, and the heard reply did not reach the console.

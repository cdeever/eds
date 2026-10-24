---
title: "CHG-0035: Only Words scribe Is Sure Of"
weight: 35
---

# CHG-0035: Only Words scribe Is Sure Of, and the reSpeaker Asked From Its Console

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
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

### Departures from the plan

## Follow-ups

- The wake word.

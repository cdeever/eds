---
title: "CHG-0038: A Misheard \"Play\", and a Longer Pre-roll"
weight: 38
---

# CHG-0038: A Misheard "Play" Is Taken for One Where the Rest Is an Album by Its Place, and the Pre-roll Is 600 ms

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The services workload: nowplayd and voiced sent and all restarted. `respeaker-01`: flashed by cable |
| **How** | `make -C deploy ship WHERE=builder`, the first with `load.sh` ([INC-0002](/docs/incidents/2026/0002-deploys-hung-the-core-router/)); `make -C firmware/ears build flash` |
| **Risk** | Low. Most likely to go wrong: a sentence that is not a request read as one; only "X's Nth album" and "the Nth album by X" are read behind a misheard "play" |
| **Decisions** | [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/), [ADR-0043](/docs/architecture/decisions/0043-the-respeaker-runs-firmware-of-edss-own/) |
| **Related** | [CHG-0037](/docs/changes/2026/0037-a-request-answered-sooner/) |

---

## Summary

After CHG-0037 and ADR-0044, "Hey Jarvis, play Aerosmith's first album"
played in 8 s from the wake word: 2 s heard, sent in 89 ms, 4 s to write
down, 1.7 s to count. The music dipped while it listened. "Play Van Halen's
first album", straight after, was written "Tae Van Halen's first album."
and was not a request.

- **voiced** takes "tae", "clay", "pray" and their like for "play" at the
  start of a request, but only where the rest is an album by its place with
  the possessive said, or "the Nth album by X"; never after a negation, and
  never for "ALBUM by ARTIST".
- **The device** keeps 600 ms from before the wake was decided on, not 350.

## Goal

- "Hey Jarvis, play Van Halen's first album" plays it.
- Only nowplayd's and voiced's images cross to the workload.

## Procedure

1. `make -C deploy ship WHERE=builder`.
2. `make -C firmware/ears build flash`.
3. The owner asks.

## Undo

`git revert` this change's commit; ship and flash.

## Outcome

| When (UTC) | What happened |
|---|---|

### Departures from the plan

## Follow-ups

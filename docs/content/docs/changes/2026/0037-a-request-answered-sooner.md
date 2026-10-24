---
title: "CHG-0037: A Request Answered Sooner"
weight: 37
---

# CHG-0037: A Request Answered Sooner

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `respeaker-01`: firmware flashed by cable. The services workload: all six rebuilt, `eds-voiced`'s unit changed and `/var/lib/eds-voiced` made |
| **How** | `make -C firmware/ears build flash`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: a request cut before its last word, which is a request said again |
| **Decisions** | [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/), [ADR-0043](/docs/architecture/decisions/0043-the-respeaker-runs-firmware-of-edss-own/) |
| **Related** | [CHG-0036](/docs/changes/2026/0036-the-respeaker-wakes-to-hey-jarvis/), whose request took 21 s from the wake word to the music |

---

## Summary

From "Hey Jarvis" to the music took 21 s in CHG-0036: 6.4 s heard, because
the room never got as quiet as the floor learned from its quietest moments;
5.3 s to send 200 KB at -80 dBm; 4.6 s to write down; 5 s to count
Aerosmith's albums in MusicBrainz after a restart had forgotten them.

- **The end of a request** is quiet judged against the speaker too: 18 dB
  below the loudest they have been, smoothed over a tenth of a second, for
  0.6 s. What follows the last word but a quarter of a second is not sent.
- **The send** has room: lwIP's send buffer and window are 32 KB, not under
  6 KB.
- **scribe** writes at most 48 tokens. `tiny.en` was tried and kept out: no
  faster to speak of, worse at names, and it fills silence with "the
  Beatles" until it runs out.
- **voiced** keeps the discographies it counts in `/var/lib/eds-voiced`,
  for a month, so that a restart forgets nothing it found.

## Goal

- "Hey Jarvis, play Aerosmith's first album" ends at the quiet, not the
  limit, and plays in under eight seconds from the end of speaking.
- A request is sent in under two seconds.
- After a restart of voiced, Aerosmith is not counted again.

## Procedure

1. `make -C deploy deploy WHERE=builder`; `make -C deploy status`.
2. `make -C firmware/ears build flash`.
3. The owner asks, twice: once to fill what is kept, once after
   `sudo systemctl restart eds-voiced`.

## Undo

`git revert` this change's commit; deploy and flash.

## Outcome

| When (UTC) | What happened |
|---|---|
| 02:38:38–02:42:13 | `deploy WHERE=builder` from `668bb1f`: all six, `/var/lib/eds-voiced` made; no timeouts. |
| 02:42:44 | `respeaker-01` flashed. |
| 02:43:21 | The owner asked. The device took 27 s to join the WiFi at -76 to -84 dBm; the request ran to the limit over Aerosmith still playing, and its send stalled ten seconds and was lost. Nothing played. |
| 02:52 | The owner added an antenna to the XIAO and moved the access point: -45 dBm, the WiFi in 0.9 s, the broker in 3.2 s. |
| 02:55:23 | ADR-0044's two grants applied: `respeaker-01` may publish `woken`, voiced may read it. |
| 02:58–03:20 | The deploy for ADR-0044 hung the core router, twice ([INC-0002](/docs/incidents/2026/0002-deploys-hung-the-core-router/)). Nothing was restarted. |
| 03:29 | nowplayd and voiced (`93bfca1`) copied alone, 20 MB in 4 s, and restarted; the relay's npagent pushed; the device flashed by USB. scribe left as it was: its change is the 48-token bound. |
| 03:34 | `load.sh` in place: images sent only when their sources change, at 5 MB/s. |

### Departures from the plan

## Follow-ups

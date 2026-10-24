---
title: "CHG-0041: voiced Asks eds-inventory"
weight: 41
---

# CHG-0041: voiced Asks eds-inventory for the Owner's Albums

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-24 |
| **Systems** | The services workload: eds-inventory installed beside EdS's services (eds-inventory's CHG-0001); voiced sent and its unit replaced |
| **How** | In eds-inventory: `make -C deploy image load unit WHERE=builder`, then `make -C deploy library XML=…`. Here: `make -C deploy ship WHERE=builder`, `make -C deploy units` |
| **Risk** | Low. Most likely to go wrong: the inventory not answering, and albums guessed at as with no library; `make deploy`'s restart of every EdS service |
| **Decisions** | [ADR-0047](/docs/architecture/decisions/0047-voiced-asks-eds-inventory/) |
| **Related** | [CHG-0040](/docs/changes/2026/0040-the-owners-library-for-voiced/), the index this replaces |

---

## Summary

eds-inventory's inventoryd goes onto the services workload with the owner's
iTunes library collected into it, and voiced is pointed at it with
`NP_INVENTORY_URL`. `library.json` stays where it is, unread while the
inventory is configured.

## Goal

- `systemctl is-active eds-inventory` is active, and
  `curl 127.0.0.1:8740/v1/artists/find?name=Arrowsmith` on the workload
  answers Aerosmith.
- voiced's log shows no "could not be asked" warning for a request.
- "Hey Jarvis, play Arrowsmith's first album" plays *Aerosmith*.

## Procedure

1. In eds-inventory, `make -C deploy image load WHERE=builder`, then
   `make -C deploy unit`: the image, its unit, `/var/lib/eds-inventory`.
2. `make -C deploy library XML="…/iTunes Library.xml"`: about 600 KB of
   albums sent, collected, and removed.
3. Here, `make -C deploy ship WHERE=builder` for voiced, and
   `make -C deploy units` for its unit's `NP_INVENTORY_URL`.
4. The owner asks.

## Undo

Remove `NP_INVENTORY_URL` from `eds-voiced.service` and `make -C deploy
units`: voiced reads `library.json` again. `systemctl disable --now
eds-inventory` stops the inventory.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 18:07:25–18:08:32 | 1 | eds-inventory's image built on the Builder and sent, 8.8 MB at 5 MB/s |
| 18:08:46 | 1 | `eds-inventory.service` started: `inventoryd listening`, `artists=0` |
| 18:09:00 | 2 | The albums sent and collected: `itunes: 3665 albums; 3665 new`. Arrowsmith answered Aerosmith on the workload |
| 18:09:35–18:10:39 | 3 | `make -C deploy deploy WHERE=builder`: only nowplayd and voiced sent (about 4 MB each); the units installed and every service restarted once. voiced's container has `NP_INVENTORY_URL=http://127.0.0.1:8740` |
| 18:25:31 | 4 | "Hey Jarvis. Play Led Zeppelin's first album.": voiced asked the inventory and sent *Led Zeppelin*; iTunes played "Good Times Bad Times" at 18:25:32. No "could not be asked" warning |
| 18:27:06 | 4 | "Play Van Halen's first album.": *Van Halen*, from the inventory's albums by year when MusicBrainz did not answer |

The inventory answers in 10 to 17 ms on the workload.

### Departures from the plan

- **The request was Led Zeppelin's first album**, not Arrowsmith's; the
  inventory chose it.
- **A slow start: 17 and 13 seconds**, and none of it the inventory's.
  MusicBrainz, asked to count each artist's albums (ADR-0041), failed two of
  four calls for Led Zeppelin, the slowest 6.3 s, and timed out at 10 s for
  Van Halen. Both artists were among voiced's kept discographies and were
  asked for again: entries kept before ADR-0045 have no MusicBrainz ids, and
  a failed ask is not kept.

## Follow-ups

- [ ] Discographies counted once and kept by eds-inventory, so a request
  does not wait on MusicBrainz (its enrichment; this record's slow start).
- [ ] Once ADR-0047 has run a while: remove `NP_LIBRARY`, `cmd/library` and
  `make library`.

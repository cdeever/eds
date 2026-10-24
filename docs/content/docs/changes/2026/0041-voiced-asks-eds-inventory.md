---
title: "CHG-0041: voiced Asks eds-inventory"
weight: 41
---

# CHG-0041: voiced Asks eds-inventory for the Owner's Albums

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "planned" "Planned" >}} |
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

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| | | |

---
title: "CHG-0040: The Owner's Library for voiced"
weight: 40
---

# CHG-0040: voiced Matches Requests Against an Index of the Owner's Library

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "planned" "Planned" >}} |
| **Systems** | The services workload: voiced sent and its unit replaced; `library.json` copied to `/var/lib/eds-voiced`. The relay Pi: npagent, for the driver's title matching |
| **How** | `make -C deploy ship WHERE=builder`, `make -C deploy units`, `make -C deploy library XML=…`; the relay's push |
| **Risk** | Low. Most likely to go wrong: a near but wrong artist or album chosen; without the index, voiced is as it was |
| **Decisions** | [ADR-0046](/docs/architecture/decisions/0046-requests-are-matched-against-the-owners-library/) |
| **Related** | [CHG-0039](/docs/changes/2026/0039-scribe-in-two-and-hey-jarvis-in-its-prompt/) |

---

## Summary

voiced gains `NP_LIBRARY`, an index of the owner's iTunes library made from
its XML, and chooses from it the artist and album a request means, by the
library's own names. The driver asks iTunes for the whole title first and
its plain start after.

## Goal

- "Genesis' first album", "Leonard Skinner's first album", "Arrowsmith's
  first album" and "Abbey Road by the Beatles" each play on the first try.

## Procedure

1. `make -C deploy ship WHERE=builder`; `make -C deploy units` for the
   unit's `NP_LIBRARY`.
2. `make -C deploy library XML="…/iTunes Library.xml"`: about 300 KB sent.
3. The relay's npagent pushed.
4. The owner asks.

## Undo

Remove `/var/lib/eds-voiced/library.json`: voiced guesses as before at the
next request. Or `git revert` this change's commit, and ship.

## Outcome

| When (UTC) | What happened |
|---|---|

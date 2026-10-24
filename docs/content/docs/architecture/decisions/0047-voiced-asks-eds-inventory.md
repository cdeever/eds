---
title: "ADR-0047: voiced Asks eds-inventory"
weight: 47
---

# ADR-0047: voiced Asks eds-inventory for the Owner's Albums, in Place of Its Own Index

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a spoken request plays an album the inventory chose; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-24 |
| **Scope** | Where voiced finds the owner's artists and albums when it turns a request into `play_album`. Not how it parses a request, or counts an artist's albums in MusicBrainz, which are unchanged. |
| **Extends** | [ADR-0046](/docs/architecture/decisions/0046-requests-are-matched-against-the-owners-library/), whose matching moves to the inventory unchanged |
| **Related** | [A Music Inventory](/docs/architecture/proposals/music-inventory/), the proposal this begins; eds-inventory's [ADR-0002](https://cdeever.github.io/eds-inventory/docs/architecture/decisions/0002-the-itunes-library-is-collected-from-its-xml/) and [ADR-0003](https://cdeever.github.io/eds-inventory/docs/architecture/decisions/0003-it-deploys-itself-to-eds-workload/) |

---

## Context

voiced matches requests against `library.json`, an index of the owner's
iTunes library that EdS makes from the XML and copies to voiced by hand
(ADR-0046). Knowing what the owner has is an inventory concern, and
[eds-inventory](https://cdeever.github.io/eds-inventory/) now does it: it
collects the same XML, keeps the same albums, and answers the four
questions voiced asks with the same matching and thresholds, carried over
from EdS's `internal/library`.

## Options considered

**Ask eds-inventory over HTTP**, on the same workload, as lightd asks
palette.

**Keep the index**, and let eds-inventory serve other clients. Two copies of
the owner's library, made two ways, to drift apart.

**Have eds-inventory write `library.json`** for voiced to read. The
inventory would learn a client's file format, which its boundary forbids.

## Decision

**voiced asks eds-inventory, at `NP_INVENTORY_URL`, for the owner's artists
and albums.**

- voiced asks through an `Owner`: the artist nearest a name, the album
  nearest a title, the first of several titles owned, an artist's albums by
  year. eds-inventory is one; the index is the other, read only when no
  inventory is configured.
- **What is not near enough is `null`** in the inventory's answer, and is
  "not in the library" to voiced, as before.
- **An inventory that does not answer is said, and the album is guessed
  at**, as it is with no library at all (ADR-0045). A request is not lost
  because the inventory is down.
- **It is not an outside service**: it runs on the same host, and its calls
  are not counted (ADR-0027).

## Consequences

- **The library is collected once, into the inventory**, by eds-inventory's
  `make -C deploy library`. EdS's `make library` and `NP_LIBRARY` remain
  until this record is settled, then go, with `cmd/library` and the index
  file.
- **voiced depends on a service from another repository.** Its unit wants
  `eds-inventory.service`, which eds-inventory installs on the workload.
- **A contract crosses repositories**: eds-inventory's version 1. voiced
  refuses another version. A test asks a real inventory when
  `NP_TEST_INVENTORY` names one.

## What would settle it

- A spoken request, on the workload, plays an album the inventory chose.

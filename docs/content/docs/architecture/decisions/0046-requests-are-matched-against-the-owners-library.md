---
title: "ADR-0046: Requests Are Matched Against the Owner's Library"
weight: 46
---

# ADR-0046: Requests Are Matched Against the Owner's Library, as Its XML Lists It

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the requests below play on the first try; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-24 |
| **Scope** | How voiced turns a heard artist and an album, or an album's place, into the titles `play_album` names. |
| **Extends** | [ADR-0041](/docs/architecture/decisions/0041-an-artists-nth-album-is-counted-in-musicbrainz/), whose count orders the library's albums; [ADR-0045](/docs/architecture/decisions/0045-an-nth-album-not-owned-is-the-next-one-that-is/), whose `instead` carries the library's titles now |

---

## Context

Every miss on 2026-10-24 was a guess at how the library spells something.
iTunes will not list its library to EdS
([ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/)),
so voiced asked for MusicBrainz's titles and the driver checked what played.
Each fix covered the miss before it: Genesis' first is the reissue *And The
Word Was* in the library; Skynyrd's is *Pronounced leh-nerd skin-nerd*, not
"(pronounced 'lĕh-'nérd 'skin-'nérd)"; "Arrowsmith" and "Leonard Skinner"
are how speech writes two bands down.

iTunes writes its library out as XML, which the owner can copy: 19,461
tracks, 3,686 albums, 1,950 artists.

## Options considered

**Keep guessing**, a rule a miss. Every new artist is a new chance of one.

**The library over another protocol** (DAAP sharing, the Windows COM
interface). DAAP sharing refused a library login as DACP did; COM needs a
program running on the PC.

**The XML, as an index voiced matches against.** What the library has,
spelled as it has it, without asking iTunes anything.

## Decision

**When voiced has an index of the owner's library, a request is answered
from it, by the library's own names; without one, as before.**

- **The index is small and personal**: artist, album title, year, track
  count and compilation flag, from music tracks only. No file paths, play
  counts or ratings. `cmd/library index` makes it (about 300 KB) from the
  XML. Neither is committed; both are gitignored.
- **It is copied by hand** (`make -C deploy library XML=…`) to
  `/var/lib/eds-voiced/library.json`, and read again when it changes. Only
  the index travels, never the XML ([INC-0002](/docs/incidents/2026/0002-deploys-hung-the-core-router/)).
- **The artist is the nearest by spelling and sound**: exact after folding
  case, accents and a leading "the"; else MusicBrainz's artist when the name
  is one of its aliases; else Jaro-Winkler at 0.9 or more, or a sound-alike
  key for names of six letters or more. Below that, nothing by them.
- **An album by name is the nearest of theirs**, by the same measure, with
  edition notes ("(Remastered)", "- 2013 Remaster") ignored and numbers
  required to agree, so "Van Halen" is never "Van Halen II".
- **An album by place is counted in MusicBrainz and found in the library**:
  the first of the artist's studio albums from the Nth on that the library
  has - the first by its other release names too - and the next ones owned
  in `instead`. Where MusicBrainz cannot count them, the library's own by
  year, without compilations or greatest hits and live albums.
- **What voiced sends is the library's**, character for character; the
  driver's start-of-title query and its check of what plays stay.

## Consequences

- **The index goes stale** until it is copied again: an album bought since
  is not found by name, and is passed over by place.
- **A wrong but near artist is possible**, and would be said back on
  `voice/<id>/heard`.
- **The owner's library is on the workload**, as names and years, readable
  only by voiced's user.

## What would settle it

- [ ] Genesis', Skynyrd's and Aerosmith's first albums, and "Abbey Road by
      the Beatles", each play on the first try, with no misses in the
      driver's log.

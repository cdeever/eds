---
title: "ADR-0022: The Owner's Collection Names the Record"
weight: 22
---

# ADR-0022: For a Source That Plays the Owner's Records, the Owner's Collection Names the Record

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a record on the platter has been shown with its own sleeve; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-12 |
| **Scope** | How nowplayd completes a track from a source that plays physical records: it asks the owner's collection at Discogs before any catalogue, and the collection's answer replaces the album the source named. |
| **Extends** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), in which a known album is never renamed. For the sources named here, it is. |
| **Related** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/), which left this open; [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/), the other setting that treats named sources differently |

---

## Context

The turntable driver names a track by what it sounds like. A recording is
pressed on many records - the album, its reissues, compilations, box sets -
and nothing in the sound says which. The recogniser names an album anyway.
On the bench it called one track of *Chicago Transit Authority* a box set,
and a track of *No Jacket Required* a singles collection.

nowplayd then looks that album up, finds the box set's cover, and lights the
room for a record that is not on the platter.

There is one fact nothing has used: the record playing is one its owner has.
The owner keeps a list of them at Discogs, 226 at the time of writing, each
the exact pressing, with its tracklist and a picture of its sleeve.

[ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/)
has a rule in the way. A catalogue's answer is checked against the album the
source named, and an album a source named is never renamed: right for iTunes,
which knows its album, and exactly wrong here.

## Options considered

**Discogs as one more catalogue**, first in `NP_CATALOGUES`. The least code.
But a catalogue is asked about the album the source named, and its answer is
refused unless it is that album. Asked about "The Singles", a collection that
holds *No Jacket Required* has nothing to say. And it would be asked for
every source, including ones that know their album.

**The driver asks Discogs itself.** It would then report the right album to
begin with. But a driver reports what it perceives and looks nothing up
([ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/)),
and the token would live on the Pi.

**The driver sends no album**, so the album is always the question, and the
catalogues answer by track. Nothing new in nowplayd. But a catalogue asked
which album a track is on answers as loosely as the recogniser does, and a
record not at Discogs would lose the one-cover-a-side the driver now keeps.

**Named sources are looked up in the owner's collection first**, by artist
and title, and what it says is the album. One new question in nowplayd, asked
only for sources said to play the owner's records.

## Decision

**A source can be named as playing its owner's own records
(`NP_COLLECTION_SOURCES`). For those sources, nowplayd asks the owner's
collection first, by artist and title, and the record it finds is the album
and its sleeve the cover.**

- **The album is replaced**, not merely filled in. This is the one place a
  source's album is not believed, and it is believed nowhere less.
- **A track the collection does not hold is completed as before**, from the
  catalogues, under the album the source named.
- **A record with no picture at Discogs** is looked up in the catalogues
  under the record's own title.
- **A track on several owned records** goes to an album before a
  compilation, always: the owner's choice, made knowing that a greatest hits
  played through will show each song's own album where that album is owned.
  Then the one named as the source named the album; the one last answered
  with, since a side is played through; vinyl before a disc; the earliest.
- **The collection is Discogs', read through its API** with the owner's
  personal access token. The list is read at start and every six hours. A
  record's tracklist is read the first time its artist is heard, a few at a
  time, and kept in memory. One cover is held.
- **It is a package of its own** (`internal/discogs`) behind an interface
  nowplayd's publisher owns, as the catalogues are. It knows nothing of
  sources.

### Discogs' terms

Read on 2026-10-11. Titles and tracklists are public domain. Pictures and a
user's collection are restricted: for personal use, not passed on, not shown
more than six hours stale, not kept longer than needed, and shown with "Data
provided by Discogs." This is one person's own collection lighting their own
room. The list is refreshed every six hours, one picture is held and none is
stored, and the console says the words beside a cover from there.

The cover does go onto the tenant's bus, retained, as every cover does. That
is inside one household's network and is what "showing it" means here. If
EdS is ever used by anyone but its owner, this is the part to read again.

## Consequences

- **The cover is the sleeve on the stand**, the pressing owned, for any record
  that is at Discogs.
- **nowplayd holds a second secret**, the token. It reads one account and can
  change it too: Discogs has no read-only token. It lives where the broker
  login lives, in a file on the workload that is never committed.
- **It is only as right as the collection is complete.** A record not listed,
  whose track is on a record that is, gets the other record's sleeve. The
  source's album as a hint guards against it when the recogniser was right,
  and nothing guards against it when both are wrong.
- **Compilations of several artists are not found.** Records are found by
  the artist they are credited to.
- **The first track by an artist with many records may be completed from a
  catalogue**, while their tracklists are read, and corrected on the next.
- **A dependency on a fourth outside service**, and the first with a limit
  that can be met in ordinary use: 60 requests a minute. Being told to wait
  is waited out.
- **A look ([ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/))
  is matched on the record's own title**, which is the name it was written
  against.

## What would settle it

Tried against the real collection, 2026-10-12: 226 records listed in three
requests. The tracks played on the bench for ADR-0021 were each found on the
record they were played from - *Chicago Transit Authority*, *No Jacket
Required*, *Hello, I Must Be Going!* - where the recogniser had named a box
set, a singles collection and a remaster. Of three copies of the last, the
vinyl was chosen. Discogs asked once to be left alone for a minute, and was.

Not yet shown:

- [ ] A record on the platter, on the console, with its own sleeve and
  `art.from` of `discogs`.
- [ ] The stand lit from it.
- [ ] A record that is not in the collection, completed as before.
- [ ] A whole side without the cover changing.

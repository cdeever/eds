---
title: "ADR-0038: The Turntable's Album Is the Original One the Track Is On"
weight: 38
---

# ADR-0038: The Turntable's Album Is the One the Track Is On, and the Original One, Not a Time Since the Last

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a side of one record and then another by the same artist have been played; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-19 |
| **Scope** | Which album nowplayd names for a track heard on the turntable, and so which cover lights the room. The turntable driver, nowplayd's resolution for the turntable, and the catalogs it asks. |
| **Supersedes** | [ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/)'s "the driver remembers the album for ten minutes"; the rest of ADR-0026 stands |
| **Extends** | [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), the owner's collection first, which stands and still comes first |
| **Related** | [ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/), whose album looks follow the album named; [ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/), which counts every call this adds |

---

## Context

On 2026-10-18, the first evening on the home Pi
([CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/)), *The
Dark Side of the Moon* was played, lifted, and *Animals* put on 52 seconds
later. The recognizer named "Pigs On the Wing, Pt. 1" and then "Dogs", which
are right. The stand kept *Dark Side*'s rainbow look for both, because both
were reported as on *The Dark Side of the Moon*.

The driver did that on purpose. It keeps the first album named on a side and
gives it to every later track by the same artist, until ten minutes of
silence, for two reasons that are real:

- **Turning a record over is silence too**, and the second side must keep its
  sleeve.
- **The recognizer names a box set or a compilation for some tracks**: on the
  bench, a track of *Chicago Transit Authority* came back as a box set. Taken
  at its word, the cover changes in the middle of a side.

But a time cannot tell a record turned over from another record by the same
band put on, and its owner does not accept a rule that has to be waited out.
The same evening shows a second fault: *Dark Side*'s first track came back
with no album, and the catalog found *The Dark Side Of The Moon (50th
Anniversary) [2023 Remaster]*, whose sleeve is not the one on the shelf.

The owner's rule is simpler, and the catalogs can follow it:

> "Pigs on the Wing" is for sure *Animals*. And always prefer original albums
> over compilations and box sets.

Asked on 2026-10-19, Deezer's track search put *Animals* first for "Pigs on
the Wing" and "Dogs", and *The Dark Side of the Moon* first for "Time", each
ahead of remixes, live albums and *A Foot in the Door*; and it lists an
album's tracks. MusicBrainz says what kind each album is (studio album,
compilation, live, remix, demo) and named *Animals* as the one studio album
among nine for "Pigs on the Wing". But its search for "Time" returned 25 live
recordings before *Dark Side*: it is good at classifying, not at finding.

## Options considered

**Keep the rule, with a shorter time.** Turning a record over and changing it
take the same minute. Any time is wrong for one of them.

**Ask the owner's collection only** (ADR-0022). It has each pressing's track
list and is exactly right when it has the record. It does not always: the
owner's Discogs lacks some Pink Floyd today, *Animals* among them.

**Believe the recognizer every time.** Right for *Animals*; wrong for the box
set, and wrong when it names no album at all.

**Name the album by what is on it.** Keep an album only while the tracks heard
are on it; otherwise take the original album the track first appeared on.

## Decision

**For a track heard on the turntable, nowplayd names the album in this order,
and no rule anywhere depends on how long it has been quiet:**

1. **The owner's collection** (ADR-0022, unchanged): the record in it whose
   track list has this track. Its pressing, and its sleeve, are the ones on
   the platter.
2. **The record already playing**, if its track list has this track. This is
   what keeps a side's sleeve across a flip, and across a track the
   recognizer files under a box set: "Time" is on *Dark Side*, so *Dark Side*
   stays. It is checked against the track list, not assumed.
3. **The original album the track is on**: of the albums the catalogs say
   have it, the first studio album, in the order of first release. Not a
   compilation, a box set, a live album, a remix, a demo, a single or an
   anniversary edition, when an original exists. An edition points at its
   original: *The Dark Side Of The Moon (50th Anniversary) [2023 Remaster]*
   is a reason to ask about *The Dark Side Of The Moon*. The recognizer's own
   album is asked about only when the catalogs have no album for the track,
   and not when its own track list rules the track out: that is the album of
   the record before, carried over.
4. If none can be found: the recognizer's album, and then the catalogs'
   cover search as today.

- **The driver stops choosing albums.** It reports the recognizer's answer
  as it came, an empty album included. Deciding needs track lists and
  catalogs, which are nowplayd's (ADR-0022's collection, ADR-0027's counted
  clients); the driver stays "a WAV in, a name out" (ADR-0021).
- **Finding and classifying are different catalogs.** Deezer is asked which
  albums a track is on, and for an album's tracks; MusicBrainz is asked what
  kind of release each candidate is and when it first came out. Either may be
  replaced; the rule is the order above, not the services.
- **A track list is fetched once an album and kept**, as covers are, so a side
  costs one lookup and the tracks after it cost none.
- **The artist must be the artist**, not a name that contains it: "The
  Machine Performs Pink Floyd" lists "Speak to Me / Breathe" too, on a
  tribute album.
- **Matching titles is forgiving**: "Pigs on the Wing 1", "Pigs On the Wing,
  Pt. 1" and "Pigs on the Wing (Part One)" are one track; "(2018 Remix)",
  "(Live)" and "- 2011 Remaster" are not part of a title.

## Consequences

- **Changing records works at any speed.** *Animals* after *Dark Side* is
  *Animals* from its first track.
- **A flip keeps its sleeve without a timer**, as long as the record's track
  list is known. When it cannot be had (the catalogs down), step 3 still names
  the original album, which for a flip is the same one.
- **The cover is the original's.** The *50th Anniversary* sleeve is not shown
  for *Dark Side*; the 1973 one is, unless the owner's collection holds the
  anniversary pressing, which then wins by step 1.
- **More calls, counted.** About two lookups per new album (candidates, then
  kinds) and one track list, cached; none for the tracks after the first.
  Within MusicBrainz's one-call-a-second rule.
- **An original that was never released as such is missed.** A track that
  only ever came out on a compilation is named by the compilation, which is
  correct.
- **A live record on the platter is named by its studio original** unless it
  is in the collection. Accepted: step 1 is the owner's way to say otherwise.
- **ADR-0026's ten-minute memory goes**, from the driver and from its README.

## What would settle it

Asked of the live catalogs on 2026-10-19, before it was deployed: "Pigs On
the Wing, Pt. 1" and "Dogs" heard as *Dark Side* named *Animals*; "Speak to
Me/Breathe" with no album named *The Dark Side of the Moon*; "Time", *Dark
Side*; *Meddle (2016 Remaster)*, *Meddle*; Chicago's track filed under
*Original Album Series*, *Chicago Transit Authority*; *Hysteria* and
*Waiting for the Sun* as themselves. The first tries found three faults the
tests then took in: Deezer finds nothing for "Pt. 1" (it is asked again in
plain words), a name containing the band's is a tribute, and MusicBrainz
answers 503 when asked quickly (it is asked once more, and spaced further).

Tested on the host, with recorded catalog answers: *Dark Side* then *Animals*
within a minute names *Animals*; a flip of *Dark Side* keeps it; "Time" named
under a box set keeps *Dark Side*; "Speak to Me" with no album names *The
Dark Side of the Moon*, not the anniversary remaster; a track only on a
compilation names the compilation.

On the turntable:

- [ ] *Dark Side* side one, then *Animals*, without waiting: the stand changes
      from the rainbow to *Animals*' sleeve on its first track.
- [ ] A record turned over: the same sleeve on the other side.
- [ ] A record not in the collection lights with its original sleeve.

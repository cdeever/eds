---
title: "ADR-0045: An Nth Album Not Owned Is the Next One That Is"
weight: 45
---

# ADR-0045: An Artist's Nth Album, When the Library Does Not Have It, Is the Next of Theirs It Does

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a first album the library lacks has been asked for and the next one played; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-24 |
| **Scope** | What "Genesis' first album" plays when the owner's library does not have *From Genesis to Revelation*. |
| **Extends** | [ADR-0041](/docs/architecture/decisions/0041-an-artists-nth-album-is-counted-in-musicbrainz/), whose count this goes on along; [ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/), whose command gains alternatives |

---

## Context

"Genesis' first album" was heard right, twice, and counted right: *From
Genesis to Revelation*, 1969. The owner's iTunes has it as its reissue,
*And the Word Was…*, and so was asked for a title it has not got. With
nothing matched it played the band's other songs from *Abacab*, the first by
name, which the driver rightly took for a miss; where nothing at all
matches, having emptied its queue, it stops. The library cannot be asked what it
has ([ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/)),
so a miss is only found by trying.

## Options considered

**Strict**: the album asked for or nothing, and say so. Correct, and in a
room it is silence where music was.

**The next album the library has**: the count goes on, in order, until one
plays. "First" becomes "the first of theirs I have", which in a person's
own room is what was meant.

## Decision

**For an album asked for by its place, voiced names up to three more of the
artist's studio albums after it, in order; the driver plays the first of
them the library has.**

- **`play_album` carries `instead`**: up to three album names, by the same
  artist, at most 200 bytes each. Only voiced's count fills it; an album
  asked for by name has none.
- **The order is the count's.** For the first or the Nth, the albums after
  it; for the latest, the ones before it.
- **The driver tries each as it tries the first**, and stops at the first
  the player says it is playing. It logs which, or all it tried.
- **A command that names alternatives may take longer**: twenty seconds in
  the agent, where a button has five. Each miss is about a second and a
  half.
- **Before the next album, the album's other names**: the titles its
  releases have gone by in MusicBrainz, most used first, up to three - the
  owner's *From Genesis to Revelation* is its reissue, *And the Word Was…*.
  Never one that is the artist's name, or that begins another of their
  albums or is begun by one, since the player matches by the start of a
  title: a reissue called "Genesis" would play the 1983 *Genesis*.
- **A title is matched by its start** (ADR-0040 amended): the player is
  asked for the plain words a title begins with - from its first letter, up
  to the first that is not a plain letter, digit or space - because it
  matches characters as they are, and only at the start ("contains" it does
  not do). MusicBrainz's "(pronounced 'lĕh-'nérd 'skin-'nérd)" is asked for
  as "pronounced" and is the owner's "Pronounced leh-nerd skin-nerd". The
  driver then takes what plays for the album only if its letters and
  digits, without case or accents, are the title's, with nothing but an
  edition's note after - "(…", "[…", "- …", "…" - so "Van Halen II" is never
  "Van Halen", and "Led Zeppelin II (Remastered)" is "Led Zeppelin II".

## Consequences

- **What plays may not be the album named**, and the device's light says
  only that an album was asked for. The log says which played.
- **A miss still stops the music**, until an album plays or the four are
  tried.
- **An artist the library has nothing by** costs eight seconds of trying.

## What would settle it

- [ ] "Genesis' first album", with the library lacking it, plays the next of
      theirs it has.

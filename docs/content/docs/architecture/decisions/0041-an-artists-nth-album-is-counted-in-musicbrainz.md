---
title: "ADR-0041: An Artist's Nth Album Is Counted in MusicBrainz"
weight: 41
---

# ADR-0041: An Artist's First Album Is Their First Studio Album, Counted in MusicBrainz

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until an album asked for by its place has played; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-24 |
| **Scope** | What "Aerosmith's first album" means, where the count comes from, and how the name it gives is made to match a library. Not how a request is heard, nor what plays it. |
| **Extends** | [ADR-0038](/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it/), whose idea of an original album, a studio album with nothing else to it, is the one counted here |
| **Related** | [ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/), which plays an album by its names and cannot ask the library what it holds; [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), the owner's Discogs collection |

---

## Context

A request to play "Van Halen's first album" has to become the names
[ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/)
plays an album by: the artist, and the album's title, as the iTunes library
writes them. The library cannot be asked which albums it has. Something else
has to know the order of an artist's albums.

## Options considered

**The owner's Discogs collection.** It knows what the owner has, but on
vinyl: the CDs and downloads in iTunes are not in it, and its earliest
record by an artist is the first the owner bought a copy of, not the
band's first.

**Deezer's list of an artist's albums.** No call limit, and it is already
asked for covers. It does not say which albums are live or compilations, and
lists each edition as its own album.

**MusicBrainz's release groups for the artist.** Each is typed (album,
single, live, compilation, soundtrack) and dated by its first release, and
the API can leave out groups that only bootlegs belong to. One call a
second, as for [ADR-0038](/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it/).

## Decision

**An artist's Nth album is the Nth of their studio albums by first release,
as MusicBrainz lists them, with the names straightened toward a library's.**

- **The artist** is MusicBrainz's whose name or alias is the one heard,
  letters and digits alike: "acdc" is AC/DC; "Van Halen" is not "Edward Van
  Halen", and a tribute band is not the band. Its name as MusicBrainz writes
  it is the one sent, since a library is likelier to have it.
- **A studio album** is a release group of type Album with no secondary
  type, credited to the artist alone, with at least one official release
  (`release-group-status=website-default`). One with no date comes last.
- **Counted from 1**, and from the newest with a negative number: "latest"
  is -1.
- **Names are written as a library writes them**: straight quotes for
  MusicBrainz's curly ones ("Guns N' Roses"), and an untitled album without
  the brackets MusicBrainz puts around it ("Led Zeppelin IV").
- **An artist's list is read whole and held** for as long as the program
  runs, every page of it: MusicBrainz lists release groups in no order, a
  hundred at a time. A miss is held too.

`internal/resolve` has it as `Originals.Discography`, beside ADR-0038's
lookups and under the same one-call-a-second gate. `cmd/discography ARTIST
[N]` asks it by hand.

## Consequences

- **A first request for an artist takes a few seconds**: an artist and one
  or two pages, a second and a half apart. A second request for the same
  artist asks nothing.
- **Some counts are not the one a person means.** MusicBrainz types *A Hard
  Day's Night* and *Help!* as soundtracks, and counts the American Beatles
  albums as albums of their own: "the Beatles' third album" is
  *Introducing… The Beatles*. Half-live albums such as *Ummagumma* count as
  studio albums. A miscount plays the wrong record, by the artist asked for.
- **The title is MusicBrainz's, not the library's.** Where they differ
  ("Van Halen II" against "Van Halen 2"), nothing plays, and the driver says
  so ([ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/)).
- **MusicBrainz is asked for more**, counted with its other calls
  ([ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/)).

## What would settle it

Tested against a stand-in for MusicBrainz: studio albums alone, in order,
across pages; the artist by its exact name and not a member's or a
tribute's; a discography read once; names straightened.

Asked of MusicBrainz on 2026-10-24: Aerosmith's first is *Aerosmith*
(1973), Van Halen's *Van Halen* (1978), "acdc"'s *High Voltage*, Led
Zeppelin's fourth *Led Zeppelin IV*, Guns N' Roses' first *Appetite for
Destruction*, the Beatles' latest *Let It Be*.

Not yet shown:

- [ ] An album named this way, played by `play_album`, for an artist whose
      name and album differ.
- [ ] How often the library's title differs from MusicBrainz's, over the
      owner's own requests.

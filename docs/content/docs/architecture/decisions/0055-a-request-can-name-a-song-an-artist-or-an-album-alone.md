---
title: "ADR-0055: A Request Can Name a Song, an Artist, or an Album Alone"
weight: 55
---

# ADR-0055: A Request Can Name a Song by Its Artist, an Artist to Shuffle, or an Album Without Its Artist

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until each has played from a spoken request; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-11-11 |
| **Scope** | What voiced reads a request for besides an album by its artist, the two commands that carry it, and how the iTunes driver plays them. Not the wordings for an album, which are grammar (`internal/voice`). |
| **Extends** | [ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/), whose `play_album` is the model for both commands; [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/) |
| **Related** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), [ADR-0045](/docs/architecture/decisions/0045-an-nth-album-not-owned-is-the-next-one-that-is/), [ADR-0047](/docs/architecture/decisions/0047-voiced-asks-eds-inventory/) |

---

## Context

A spoken request names an album and its artist, or an artist's album by
its place (ADR-0040, ADR-0041). The owner asked on 2026-11-11 for three
more: **a song** ("play Come Together by the Beatles"), **an artist**
("play some Van Halen"), and **an album without its artist** ("play
Abbey Road").

iTunes for Windows refuses this remote its library (ADR-0040): an album
is played by naming it, and what then plays is the only proof the names
matched. A song and an artist can be asked for the same way. eds-inventory
knows the owner's artists and their albums, but no songs yet (its tracks
are in the schema under review), and cannot find an album by its title
alone.

## Options considered

**An artist.** All their songs, shuffled; one of their albums, at random;
or all their songs in album order, as iTunes plays an artist today. The
owner chose **all their songs, shuffled**.

**A song.** With its artist only, or by its title alone. A title alone
matches covers, live versions and other songs of the same name; the owner
chose **its artist must be said**.

**An album without its artist.** A route in eds-inventory to find an album
by its title on today's schema, or waiting for the new schema. The owner
chose **a route on today's schema**, to move with the rest.

**How the commands are carried.** One command with a field saying what
kind; or **one command for each**, as `play_album` is one: a receiver that
does not know `play_track` refuses it by name (ADR-0020), where an unknown
kind inside `play_album` would be played as an album.

## Decision

**voiced reads three more requests, and sends two new commands,
`play_track` and `play_artist`, which go where `play_album` goes
(`NP_ALBUM_SOURCE`, iTunes) whatever is current.**

- **`play_track`** names an artist and a title. The driver empties the
  queue, asks for the artist's songs whose title begins with the one
  asked, with shuffle off, and then asks what plays: the artist's, and
  a title that is the one asked with at most an edition's note after it
  ("Come Together - Remastered 2009"). Anything else is a failed command.
- **`play_artist`** names an artist. The driver turns shuffle on, empties
  the queue, asks for all of the artist's songs, and checks the artist of
  what plays.
- **`play_album` turns shuffle off first**, so an album played after an
  artist is in its order.
- **An album without its artist** is looked up in eds-inventory by its
  title (`GET /v1/albums/find?title=`). One artist near enough: it is
  `play_album` with that artist. None, or more than one: not understood,
  and voiced says which artists, so the request can be said again with one.

How the words are read. **These two are choices made here, for the owner
to overturn**:

- **"Play X" alone is the artist X if the owner has one by that name,
  else the album X.** "Van Halen" is both; the artist wins, and the album
  is still "Van Halen by Van Halen" or "Van Halen's first album".
- **"Play X by Y" is the album X if the owner has it, else the song X.**
  It reads the same either way, and the library says which. Said with
  "the song" or "the track", it is always the song.
- "Play some X", "play songs by X", "play anything by X" and "shuffle X"
  are always the artist.

Without eds-inventory, voiced reads "play X by Y" as an album, as before,
and "play X" alone not at all.

## Consequences

- **Two new commands in the contract**, each refused by name by anything
  that does not know it, as the screen and the turntable's driver are
  (ADR-0020); the console sends none.
- **The relay Pi's agent changes**, which takes `make -C deploy/relay
  push`; voiced alone sending them to an old agent gets a failed command,
  not a wrong one.
- **A song that is not the one asked stops the music**, as an album that
  is missed does (ADR-0045): the queue was emptied to ask for it.
- **Shuffle is the driver's to set**, for every one of the three: what
  the owner last chose on iTunes itself is not kept.
- **eds-inventory gains a route** on the schema it has; the schema's
  rework carries it over.

## What would settle it

- [ ] "Play Come Together by the Beatles" plays it, and a song the owner
      has not got is a failed command.
- [ ] "Play some Van Halen" plays their songs shuffled, and an album asked
      for after it plays in order.
- [ ] "Play Abbey Road" plays it; a title two artists share is answered
      with both.
- [ ] iTunes for Windows takes `dacp.shufflestate` from this remote.

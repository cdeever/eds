---
title: "A Music Inventory"
weight: 1
---

# Proposal: A Music Inventory

| | |
|---|---|
| **Status** | Proposal, not decided and not built |
| **Date** | 2026-10-24 |
| **Where** | **eds-inventory**, a project of its own: [repository](https://github.com/cdeever/eds-inventory), [docs](https://cdeever.github.io/eds-inventory/). It runs in EdS's Deevnet IoTaaS tenant |
| **Would extend** | [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), [ADR-0038](/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it/), [ADR-0041](/docs/architecture/decisions/0041-an-artists-nth-album-is-counted-in-musicbrainz/), [ADR-0045](/docs/architecture/decisions/0045-an-nth-album-not-owned-is-the-next-one-that-is/), [ADR-0046](/docs/architecture/decisions/0046-requests-are-matched-against-the-owners-library/) |

A local inventory of the owner's music, in every format, that holds one
answer for each album and that EdS asks first. EdS would be its customer,
the way `lightd` is a customer of `palette`.

## Why

EdS answers "what album is this, what does it look like, what else is
there" by asking a different place for each question:

| Question | Asked today | Where in EdS |
|---|---|---|
| Which record is on the platter, and its sleeve | the owner's Discogs collection | `nowplayd` |
| The album a track is on, and the original among reissues | Deezer finds it, MusicBrainz says what each album is | `nowplayd` |
| A cover, when the source has none | Deezer, then iTunes Search, then MusicBrainz and the Cover Art Archive | `nowplayd` |
| An artist's Nth album | MusicBrainz | `voiced` |
| What the digital library holds | `library.json`, made from iTunes' XML and copied by hand | `voiced` |

Each is a chase of its own, with its own way of failing and its own count
of calls to someone else's service
([ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/)).
Each holds a piece of the owner's music, and none holds all of it. The same
album is looked up again on every play.

## What it is

- **It holds the owner's music in every format**: the records on the shelf,
  the digital libraries in iTunes and MusicBee, the albums saved in Spotify
  and Tidal, and anything else owned, CDs among them.
- **Discogs stays the source of truth for the vinyl.** Records are still
  cataloged there, and the inventory keeps itself in step with it.
- **It enriches, once.** The inventory itself asks the outside services for
  covers, originals and discographies, the first time it meets an album, and
  keeps what it is allowed to keep. EdS asks only the inventory, and an
  album is looked up once, not on every play.
- **One identity per album, its own.** The inventory gives each album an ID
  of its own, and keeps the IDs other services use for it, MusicBrainz,
  Discogs, iTunes, Spotify and Tidal, beside it, so an album heard from any
  source is the same album.
- **It is asked over HTTP**, with a versioned JSON contract (`"v": 1`) and
  an OpenAPI document that keeps both sides honest, as `palette` is asked by
  `lightd`.

## The boundary

The inventory knows music and nothing about EdS: no MQTT, no rooms, no
lights, no turntable. That is the same line `palette` keeps, knowing color
and nothing about music, and it is what lets the inventory be built, tested
and used without EdS running at all, and by something other than EdS.

A look of its own for an album
([ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/))
is EdS's idea, and stays in EdS.

## What EdS would ask it

- Which album the owner has that a heard track is on, and in which format.
- The cover for an album.
- The original album, among reissues, compilations and box sets.
- An artist's albums in order, so that "their first album" can be counted.
- Whether the owner has an album, by the nearest artist and title, so a
  spoken request names one that can be played.

## What would change in EdS, later

- In `nowplayd`, asking the Discogs collection and then the catalogs in turn
  would become asking the inventory.
- In `voiced`, `library.json` and counting albums in MusicBrainz would
  become asking the inventory.
- EdS's own calls to outside services would mostly move into the inventory,
  and be counted there.
- Each of the decisions it would extend gets a new record when that
  happens; none is rewritten.

## What would not change

- **Now Playing still decides what is playing.** The inventory says what an
  album is, never what is current.
- **The recognizer still names a clip.** The inventory hears nothing.
- **The drivers still report their players.**

## Open questions

- **Which machine it runs on.** It runs in EdS's tenant, since a tenant has
  no way in from another, and EdS asks it as `lightd` asks `palette`. Whether
  that is the services workload or one of several Pis, when the one-Pi back
  end ([ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/))
  becomes more than one, is not decided.
- **How it keeps in step** with Discogs, the libraries and the streaming
  services, and how often.
- **What EdS does when it is down**: fall back to today's chase, or report
  only what it knows.
- **What Discogs allows to be kept.** Today EdS holds one Discogs cover, in
  memory, and credits "Data provided by Discogs." beside it. A cache of
  Discogs' data and pictures may not be allowed, so the vinyl's covers may
  have to come from the Cover Art Archive instead.
- **How the two repositories keep the contract honest**: the inventory
  publishes its OpenAPI document, and EdS's client is tested against it.

---
title: "Capturing Vinyl"
weight: 2
---

# Proposal: Capturing Vinyl

| | |
|---|---|
| **Status** | Proposal, not decided and not built |
| **Date** | 2026-10-25 |
| **Where** | EdS listens and captures; [eds-inventory](https://cdeever.github.io/eds-inventory/) knows what is captured and what is missing |
| **Would extend** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/), [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), [ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/) |

Every record played is captured to FLAC, by itself: put a record on and it
is kept, track by track, and a track already kept is skipped. Nobody
presses record. Over time the digital collection comes to include what is
held only on vinyl, and every capture teaches EdS to name that record
itself.

## Why

- **The vinyl-only records join the digital collection.** A record with no
  CD or rip becomes playable anywhere the house plays music, and asked for
  by voice like any other.
- **The turntable is named locally.** EdS names a record today by sending a
  few seconds of it to Shazam, through an endpoint nobody promised
  ([ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/)).
  A capture is fingerprinted, so the next time the record plays it is named
  from the owner's own collection: the same pressing, the same mastering,
  even the same surface. Shazam is needed only for a record not yet heard.

## What EdS already has

Capturing is a second use of what the turntable driver already hears, not
new listening:

| A capture needs | EdS has |
|---|---|
| when a side begins | the beacon (the turntable switched on, [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)) and the needle coming down |
| where a track ends | a second of quiet with music after it ([ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/)) |
| when a side is done | twenty seconds of silence |
| what is playing | the recognizer, then the owner's Discogs collection for the record ([ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/)) |

What it does not have is the sound. The driver hears the amplifier's tape
output in mono at 16 kHz, which is enough to name a track and not enough to
keep one.

## How it would work

1. **A side begins**, and the capture starts, at archival quality.
2. **It is split into tracks** by the gaps, held to the edition's track list:
   how many tracks the side has and about how long each is, from Discogs or
   MusicBrainz. Gaps alone are fooled by a quiet passage or tracks that run
   into each other; the list alone does not know where the needle came
   down. Together they do.
3. **Each whole track is kept** as FLAC, tagged with what it is. A track cut
   short (the needle lifted, the side not finished) is thrown away, and kept
   on a later play.
4. **A track already kept is skipped.** EdS asks the inventory what is still
   missing from the edition; a record is fully captured when nothing is.
   Side A today and side B next month add up.
5. **The capture is fingerprinted**, so the next play is named locally.

**The first play of a record** still needs a name before there is a
fingerprint: the recognizer names the track, as today, and the owner's
collection names the edition. When more than one pressing is owned, the
original is taken, and can be corrected.

## The boundary

EdS listens, captures and names; it knows the sound. The inventory knows
music: a capture is to it one more copy of the vinyl edition, with its
tracks, and it can say which are still missing. It hears nothing, as the
[music inventory proposal](../music-inventory/) promises.

## Open

- **The capture's sound.** A card chosen for naming tracks is not one to
  archive with. The house already turns this stereo's output into S/PDIF for
  its switch ([Background](../../spdif/)); a digital input there would keep
  exactly what that converter makes, at its resolution and its quality.
  Otherwise, a converter chosen for the purpose. Which, and at what
  resolution.
- **Where the files go.** The media server is the natural home. Written
  there with tags naming the MusicBrainz IDs and the inventory's edition,
  they would be collected with MusicBee's library, already identified. At
  24-bit/96 kHz an LP is on the order of a gigabyte, so copies are rate-capped
  ([INC-0002](/docs/incidents/2026/0002-deploys-hung-the-core-router/)).
- **Where it runs.** On the agent Pi beside the driver, or a device of its
  own on the same sound.
- **Splitting** a side whose tracks run into one another, and a record whose
  track list is not known.
- **What is kept of the surface**: clicks and pops left as played, or a
  copy cleaned up.
- **Vinyl FLAC or CD FLAC**, for an album held both ways: which the house
  plays first is the owner's preference, not a rule (eds-inventory's "best
  copy").

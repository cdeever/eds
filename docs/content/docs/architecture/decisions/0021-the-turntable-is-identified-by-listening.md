---
title: "ADR-0021: The Turntable Is Identified by Listening"
weight: 21
---

# ADR-0021: The Turntable Is Identified by Listening, With a Recogniser That Can Be Replaced

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a record on the platter has lit the stand; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-11 |
| **Scope** | A third driver in `npagent`, `turntable`: it listens to the amplifier's line-level output on the agent Pi, has a recogniser name what it hears, and reports that as a source on the now-playing bus. |
| **Extends** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), which said a turntable that identifies its record would be one more driver, and [ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/) |
| **Related** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/) and [ADR-0014](/docs/architecture/decisions/0014-tidal-by-lastfm/), which each declined an unpublished interface; [ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/), [ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/) |

---

## Context

The stand was built to stand beside a record that is playing. So far it
follows iTunes, because iTunes can be asked what it is playing and a turntable
cannot.

A turntable says nothing about its record. Whatever identifies it has to
work from the record itself: what it looks like, or what it sounds like.

The amplifier has a tape output: line level, fixed, after the phono stage,
and beside the listening path rather than in it. The agent Pi
([ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/)) is in reach of
it, is a Pi 4 with 8 GB, and already runs the agent every other driver is in.

## Options considered

### What identifies the record

**A camera on the sleeve or the label.** It sees the record before it plays,
and knows the pressing. But it needs a camera placed and lit, a picture of
every sleeve to compare against, and it knows which record and not which
track or whether the needle is down.

**Listening.** One cable, no placement, and it knows when music starts and
stops because it hears it. It knows the track and has to work out the record.

### What names the audio

**AcoustID and Chromaprint**, the free and open fingerprint service. Its own
documentation says it identifies whole files and is not for short clips or
audio with noise. A needle put down mid-track is exactly what it is not for.

**An official recognition API**: AudD, ACRCloud. Documented and supported.
AudD is about $5 a month for this use after 300 free requests; a permanent
free tier at ACRCloud could not be confirmed. Not free, and the requirement
was free.

**Fingerprints of the owner's own records**, matched on the Pi: Olaf,
Panako. Free, offline, and nobody else's terms. But they match audio against
audio, so every record has to be played to it once, about forty minutes each,
and told what it is, before anything is recognised at all.

**Shazam, through an open client.** ShazamIO (MIT) and SongRec (GPL-3.0)
compute Shazam's fingerprint locally and send it to the endpoint Shazam's own
apps use. Free, no account, no preparation, a very large catalogue, and what
every comparable open project uses. The endpoint was never published: it was
worked out from the apps, Apple has retired neighbouring endpoints without
notice, and its terms are not likely to permit this.

### Where the recogniser sits

**In the driver.** Simplest, and it makes the driver the thing that breaks
when the endpoint does.

**Behind an interface, as a program.** The driver hands a clip to a command
and reads back a name. Which recogniser that command is, the driver does not
know.

## Decision

**The turntable is identified by listening, by a `turntable` driver in
`npagent` on the agent Pi.**

**The first recogniser is Shazam's, through ShazamIO, and it is kept behind a
seam so that it can be replaced without the driver changing.**

This uses an unpublished interface, which
[ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/) and
[ADR-0014](/docs/architecture/decisions/0014-tidal-by-lastfm/) each declined.
Those declined impersonating a player's own client while signed in to
someone's account, where a published way to ask existed. Here no account is
involved and nothing is signed in to; what leaves the Pi is a fingerprint, not
audio; a few requests a minute are made while a record plays and none
otherwise; and there is no published, free way to ask the same question. It is
still not a promise anyone made. It is accepted as the way to have this
working now, and a local index of the owner's records is the intended way to
stop depending on it.

The driver:

- **Listens with `arecord`**, a program and not a library, so the module
  stays free of C ([ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/)).
  Mono, 16 kHz, through a `plughw` device so ALSA does the converting.
- **Counts time in audio heard**, a tenth of a second at a time, not by the
  clock. A capture that stalls is then not mistaken for silence.
- **Knows silence for certain and nothing else.** Silence for twenty seconds
  is `stopped`. Music is `playing`: with the track once the recogniser names
  it, and with no track when it cannot.
- **Sends a twelve-second clip every twenty seconds** while music plays, and
  only a clip that is mostly music.
- **Believes the first answer after silence, and no single answer after
  that.** A different track replaces the current one when two clips running
  agree on it. Four clips running that the recogniser cannot name let the
  track go.
- **Keeps the album the side began with** for the tracks after it by the
  same artist, across the silence of turning the record over. The recogniser
  names an album per track and does not agree with itself: on the bench, two
  tracks of one LP came back as two albums.
- **Names no cover.** nowplayd finds one, as for any source without
  ([ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/)).
- **Takes no commands** ([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)).
  There is nothing on a turntable for one to reach.

The recogniser is `/usr/local/bin/eds-recognize`: a WAV file on standard
input, one JSON object out. It is the only file that knows it is Shazam. It
runs in a Python environment of its own on the Pi, with every package pinned
to what was tried.

It runs on the existing agent Pi, beside the iTunes driver, under the same
account and the same registered device (`np-relay`). The agent's unit gives up
`PrivateDevices` for access to sound devices and nothing else.

## Consequences

- **The stand can follow a record with no change to nowplayd, lightd or the
  firmware.** The turntable is a source like any other.
- **It can stop working without anything here changing.** The endpoint is
  Apple's to move. The driver then reports music it cannot name, and says in
  the log store that the recogniser cannot be asked. Mending it is new pins in
  one requirements file, or a different recogniser behind the same seam.
- **What is listened to leaves the house as fingerprints**, to Shazam, a few
  times a minute while a record plays. Nothing is sent in silence.
- **The cover may be another edition's.** The recogniser names the track, and
  the album it names with it is often a compilation or a reissue. The owner's
  Discogs collection is the fix, as a catalogue in nowplayd, and is a decision
  of its own.
- **Records the catalogue does not hold are never named**: obscure pressings,
  private ones. The local index is the fix for those too.
- **Two sources playing at once take turns.** nowplayd has no priority among
  sources, and a record and iTunes both playing will each win when its track
  changes. Unchanged, and not expected to matter in one room.
- **A cover takes about half a minute to change**, at the start of a record
  and between tracks: twelve seconds of clip, and a second opinion for a
  change.
- **The agent Pi is now tied to where the amplifier is**, by a cable.

## What would settle it

Tried on the bench, 2026-10-11, on the agent Pi with a Roland UA-4FX on the
tape output and ShazamIO 0.8.1:

- Fourteen clips of twelve seconds, from three tracks of two records, and
  fourteen answers, each in about half a second.
- A quiet passage, a piano alone at -46 dBFS, was named.
- Peaks between -12 and -16 dBFS, nothing clipped.
- Of three tracks, two were given an album that is a compilation.
- The driver itself, run on the Pi with `npagent watch` and publishing
  nothing: a third record was named fourteen seconds after it started
  listening, and reported once in the minute and a half that followed.
- The same again as installed, as the agent's own user under its unit's
  confinement: a track named, so the sound card and the recogniser are both
  in reach from inside it ([CHG-0013](/docs/changes/2026/0013-the-turntable-driver/)).

Not yet shown, and what this record waits on:

- [ ] That the bench source was the turntable itself, and the answers right.
- [ ] A record on the platter lights the stand through the driver. So far:
  its track became `current` with a cover, and lightd sent a scene from that
  cover, with the stand itself offline at the time.
- [ ] The needle put down in the middle of a track.
- [ ] A record Shazam is unlikely to know.
- [ ] The input's level with the needle up, and in the run-out groove, against
  the -55 dBFS the driver takes for silence.
- [ ] A whole side, to see the track changes and the gaps between them.

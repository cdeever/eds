---
title: "ADR-0042: A Request Is Heard on the Workload"
weight: 42
---

# ADR-0042: A Spoken Request Is Sent Whole to the Workload, Written Down There, and Made a Command

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a request said in the room has played its album; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-24 |
| **Scope** | How "Hey Ed, play Van Halen's first album" travels from a microphone to `play_album`: what the device sends, where speech becomes words, and how words become a command. Not the device's firmware, which is a decision of its own. |
| **Extends** | [ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/), whose command this sends, and [ADR-0041](/docs/architecture/decisions/0041-an-artists-nth-album-is-counted-in-musicbrainz/), whose count it asks |
| **Related** | [ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/), the broker as the one way a device reaches EdS; [ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/) |

---

## Context

The microphone is a reSpeaker Lite: an XMOS chip that cleans up what a room
says, and a small ESP32-S3 beside it. The ESP32-S3 can listen for a wake
word and keep a few seconds of audio. It cannot turn speech into words. A
device can reach nothing of EdS's but the broker
([ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/)).

## Options considered

**Home Assistant's voice pipeline**, which the reSpeaker's own firmware
speaks. It brings a home automation system EdS does not have, to do one
thing.

**Speech to words on the agent Pi**, beside the drivers. A Pi 4 writes down
a few seconds of speech in several seconds, and the agent Pi is not where
the bus's thinking is done.

**Speech to words on the workload**, the way covers become colors there: a
service that knows nothing about music, beside one that does.

## Decision

**The device sends the request whole, as a WAV, on the broker. On the
workload, `scribe` writes it down and `voiced` makes it a command.**

- **The device sends one message per request**: `voice/<device>/utterance`,
  a WAV of 16 kHz, 16-bit mono PCM, of what followed the wake word. Never
  retained. Fifteen seconds at most, about half a megabyte, which the broker
  already carries for covers. The wake word is the device's, and is not in
  it.
- **`scribe` knows nothing about music, MQTT or albums.** A WAV in, words out
  (`POST /v1/transcribe`), with faster-whisper's `base.en` model on the
  workload's CPU. Like palette, it can be developed against a folder of
  recordings with no infrastructure at all.
- **`voiced` knows requests.** It hears each utterance, asks `scribe` for its
  words, and reads them with a small grammar: "play the Nth album by X",
  "play X's Nth album" (first to tenth, latest, newest, last, debut), and
  "play ALBUM by X". The Nth is counted as
  [ADR-0041](/docs/architecture/decisions/0041-an-artists-nth-album-is-counted-in-musicbrainz/)
  says. It sends `play_album` on `nowplaying/command`, from `voiced`.
- **What was heard is said back** on `voice/<device>/heard`, never retained:
  the words, and either the album asked for or why there is none. The device
  can answer with a light or a sound, and the console can show it.
- **Words that are not a request are dropped**, and said back as not
  understood. Nothing nearer is guessed.
- `voiced` is a program of the nowplaying module, because it counts albums
  with its resolver; it runs as its own service, with its own broker
  account.

## Consequences

- **The request leaves the device as audio.** Anything said after the wake
  word is on the broker for a moment, and in the workload's memory for as
  long as it takes to write down. None of it is kept.
- **The workload is small**: two cores and two gigabytes, which `base.en`
  fits. A larger model would need more.
- **A request takes a few seconds**: the speech, writing it down, and the
  first count of an artist's albums.
- **The grammar is short**, and a request said another way is not
  understood. Widening it, or reading requests with a language model, is a
  later decision.
- **A new device account** may publish its utterances and read what was
  heard; `voiced`'s account reads the utterances and may publish commands.

## What would settle it

- [x] A WAV of "play Van Halen's first album" on the development broker
      becomes `play_album` for *Van Halen* by Van Halen (2026-10-24, from
      macOS `say`, with scribe and voiced on a Mac). Four of six requests
      were written down right; "Aerosmith" was written "Arrowsmith", and
      "AC/DC", spelled out by the voice, "ACDC's Z". Both were said back as
      not found or not understood, and nothing was asked for. Writing down
      took 0.4 s; counting an artist's albums for the first time, 2 s more.
- [x] The same on the workload, with `voiced`'s account, passed on by
      nowplayd to iTunes
      ([CHG-0034](/docs/changes/2026/0034-voiced-and-scribe-on-the-workload/)):
      published with the reSpeaker's account, *Van Halen* playing about
      4 s later, from stopped.
- [x] A request said in the room, through the reSpeaker, plays its album:
      "Play Aerosmith's first album", 2026-10-24
      ([CHG-0035](/docs/changes/2026/0035-only-words-scribe-is-sure-of/)).
      The first attempt played *Abbey Road* instead
      ([INC-0001](/docs/incidents/2026/0001-the-beatles-played-for-a-request-nobody-made/)).
- [ ] How long a request takes, from the end of speaking to the first track.
      From the recording's arrival: about 4 s, of which writing it down
      is 3.5 s on the workload's two cores (CHG-0034).

---
title: "Background: Sound Around the House"
weight: 1
---

# Background: Sound Around the House

EdS did not bring music to the rooms of the house. A whole-house digital
audio system was already doing that, was built years before EdS, and carries
on exactly as it did. EdS listens in on it. This page is the short version;
the project's own page has the whole of it:
[S/PDIF Audio Distribution](https://chrisdeever.com/electronics/spdif-audio-distribution/).

## Whole-house digital sound distribution

A media server in the basement sends one digital feed (S/PDIF) to every
room, and each room's own stereo plays it.

**My music, my control.** It is built by hand, and that is worth the
effort:

- **No monthly subscription fees.** Nothing in the house's sound depends on
  a company's service or its pricing; the music plays whether or not anyone
  is paying for an app.
- **Each room keeps its own stereo**, vintage ones included, instead of
  speakers chosen by whoever sells the system.
- **Custom integrations are possible** with the providers already in use,
  iTunes, Spotify and the owner's Discogs collection among them, in ways no
  packaged system offers. EdS is one of those integrations.
- **No microphones listening to every conversation** and sending it to the
  cloud. EdS's ears hear only what is said to them, and the words are
  written down on EdS's own workload, not by someone else's service. The
  one sound EdS sends out of the house is a few seconds of the record, to
  name it.

### Switching between the turntable and the house music

Put a record on the turntable and it takes over the feed, so the record
plays in every room with no button pressed anywhere. A smart power strip on
the turntable is how the system knows a record has taken over: switch the
turntable on and the house is the record's; switch it off and the media
server has it back.

### Listening areas

- **Home office and vinyl listening room**: the primary listening room, and
  home of the turntable and the record collection.
- **Family room home theater**, which covers the family room and the
  kitchen.
- **Master bedroom**, on a vintage Marantz.
- **Retro gaming room**, on a vintage Carver system like the one in
  *Ferris Bueller's Day Off*.
- **Outside**: speakers fed from the basement, and an FM transmitter that
  carries the feed to a vintage boom box.

## What EdS adds to it

The music already reaches every room, and it can already be controlled:
iTunes and Spotify from their phone apps, and the media server itself over
Remote Desktop. EdS does not replace any of that. It adds ways that are
more convenient, and more fun, differently for each room, and it stays out
of the path the music takes.

**Today**, in the vinyl listening room, the record is shown: a stand lit in
the colors of its cover, and a screen beside it that shows the cover and can
pause, skip and turn down what iTunes is playing.

**In development**, an album can be asked for out loud, and iTunes plays it.
For now there is one device that listens, the ears, and it sits wherever
it is being worked on. Ears are meant to be on several devices throughout
the house, or on one that is carried from room to room.

**Meant for later**, and not built yet:

- **Where hands are busy**, as in the kitchen, the room has ears of its
  own, and music is asked for without a phone in hand.
- **Where people sit**, a screen says what is playing and can change it.
- **Where the room has a character of its own**, as the gaming room does,
  lights and looks are its own and not the vinyl room's.
- **Where there is no screen at all**, outside, a request is still heard
  and answered.
- **Wherever it is carried**, a portable cover screen with ears built in:
  plugged in in any room, it shows what is playing and hears what is asked
  for.

## Why this matters to EdS

- **EdS can fail without the music noticing.** Every part of EdS can be off
  and a record still plays in every room.
- **The house already knows some things.** The power strip answered "is the
  turntable on?" long before EdS asked; EdS reads the same strip
  ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)).

How EdS hears the feed and the strip is in [How EdS Runs](../how-it-runs/).

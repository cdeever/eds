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

One digital feed (S/PDIF) is sent from the basement to every room, and each
room's own stereo plays it.

### Listening areas

- **Home office and vinyl listening room**: the primary listening room, and
  home of the record collection.
- **Family room home theater**, which covers the family room and the
  kitchen.
- **Master bedroom**, on a vintage Marantz.
- **Retro gaming room**, on a vintage Carver system like the one in
  *Ferris Bueller's Day Off*.
- **Outside**: speakers fed from the basement, and an FM transmitter that
  carries the feed to a vintage boom box.

### The media server and its sources

A media server in the basement plays the house's music, from more than one
source:

- **Local collections**, in iTunes and MusicBee.
- **Streaming services**, Spotify and Tidal among them.

It is the house's central digital library and its gateway to streaming, and
its digital output is the primary source for every room.

### The digital S/PDIF switch

Before the music is sent through the house, the media server's sound card
digital output passes through an automatic digital switch. The media server
is on the switch's primary input, and its other inputs are overrides: when a
signal appears on one, the switch passes that source to the house instead,
and when the signal goes away, the media server has the house back. Nobody
presses a button.

### Switching between the turntable and the house music

The turntable is one of those overrides. The home office stereo's analog
output, with the turntable playing through it, is converted to digital and
sent to the switch as its second input.

Wiring it in directly is not enough for the switch to change over by
itself. The converter (an ADC) sends a signal whenever it has power, even
with nothing playing, so the switch would never hand the house back. A
smart power strip was added so that the ADC has power only while the
turntable does.

The result: turn on the turntable, and the house's music is the spinning
record. Turn it off, and the media server has the house back.

## What EdS adds to it

The music already reaches every room, and it can already be controlled:
iTunes and Spotify from their phone apps, and the media server itself over
Remote Desktop. EdS does not replace any of that. It adds ways that are
more convenient, and more fun, differently for each room, and it stays out
of the path the music takes.

**Today**, in the vinyl listening room:

- **The LP stand.** A record's jacket is twelve inches square of album art,
  and most of the time it sits in a crate. On the stand, the jacket of the
  record that is playing is on display, lit from behind in its own colors:
  drawn from the cover itself and set slowly moving, so the room takes on
  the look of the album for as long as it plays.
- **The cover screen.** A small touch screen beside it shows the cover of
  whatever is playing, and for iTunes it is a remote too: play, pause,
  skip, louder, quieter, without reaching for a phone.

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

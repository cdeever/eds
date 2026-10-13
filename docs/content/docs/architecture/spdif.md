---
title: "Background: Sound Around the House"
weight: 3
---

# Background: Sound Around the House

EdS did not bring the turntable's sound to the basement. A whole-house
digital audio system was already doing that, was built years before EdS,
and carries on exactly as it did. EdS listens in on it, and reuses one of
its switches. This page is the short version; the project's own page has the
whole of it:
[S/PDIF Audio Distribution](https://chrisdeever.com/electronics/spdif-audio-distribution/).

## What was already there

**One digital feed, in every room.** A media server in the basement plays
the house's music, and its digital output (S/PDIF) is split and sent to each
room over the coaxial cable the house was built with for television. Each
room's own stereo has a small converter (a DAC) on its auxiliary input.
Switch a room's stereo on and the music is there.

**The turntable is a second source that overrides the first.** The record
player is upstairs, on its own stereo. A converter (an ADC) on that
amplifier's tape output turns whatever it is playing into S/PDIF and sends it
back down to the basement on a second cable. There it meets the media
server's feed at an **automatic digital selector**, which passes the media
server until it sees a signal on its other input, and then passes that
instead. Put a record on and it plays in every room, with no button pressed
anywhere.

**The power strip is how the selector knows.** The selector switches on the
presence of a signal, and an ADC that is powered sends one whether or not a
record is turning. So the ADC is plugged into a smart power strip whose
master outlet is the turntable's: it has power only while the turntable is
switched on. Turntable on, ADC on, the selector hands the house to the
record. Turntable off, the ADC goes quiet and the media server has the house
back.

All of that is older than EdS and none of it depends on EdS.

## What EdS adds to it

Two things, and neither is in the path the music takes:

- **A listener.** A USB sound card on the agent Pi takes the selector's
  output, as one more room would. The turntable driver hears the same feed
  the house does
  ([ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/)).
- **A beacon on the strip.** An ESP32 is plugged into another of the strip's
  switched outlets, beside the ADC. It has power exactly when the ADC does,
  and its connection to the broker says so
  ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)).

## Why this matters to EdS

- **What the Pi hears is not always the turntable.** With the turntable off,
  the selector passes the media server, and that is what reaches the sound
  card. Sound alone cannot say a record is playing. The beacon can: while it
  is offline, the turntable's driver is not listened to.
- **The strip already answered the question.** The house needed to know
  "is the turntable on?" long before EdS did, and settled it with a power
  strip. EdS asks the same strip, with a device that touches nothing in the
  audio path.
- **EdS can fail without the music noticing.** The Pi, the broker and every
  service can be off and a record still plays in every room.

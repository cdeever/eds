---
title: "Architecture"
weight: 10
---

# Architecture

EdS is shaped by a handful of principles, applied the same way in every
service, driver and device. This page says what they are and why each was
chosen. The pages in this section show them at work, and the
[decisions](decisions/) record each place one was applied and what was
turned down.

## A system of subsystems

EdS is one system made of subsystems, each named for something a person in
the room experiences: the LP Light Show, the Remote Display, Voice Request, and
Now Playing beneath them all. Each subsystem is made of components, a
program or a device apiece, and every component belongs to exactly one of
them or to the foundation they share.

A subsystem is the unit EdS grows by. A new one is added beside the others,
with components of its own, and meets them only at the broker
([Subsystems and Components](subsystems/)).

## Each part does one job, and knows nothing of its neighbors

Every component takes a plain input, returns a plain output, and keeps what
happens in between to itself. `palette` sees image bytes and knows nothing
of music or LEDs; `lightd` sees covers and knows nothing of tracks; the
firmware sees a scene and knows nothing of where it came from.

That is what lets each part be built, tested and tuned alone, and replaced
without the others noticing. When knowledge has to live somewhere, it lives
in the one layer that needs it, and a change to it reaches no further. The
cost is that a fact sometimes has to be told across a boundary instead of
found out, and that is accepted.

## Everything meets at the broker

No part of EdS talks to another directly. Each one publishes what it knows
to the MQTT broker and subscribes to what it needs. A new subscriber is
added without changing any publisher, and a publisher can be rewritten
without any subscriber knowing.

## State waits for whoever starts late

Almost everything published is **retained**: the broker keeps the latest
word on each topic and hands it to anything that subscribes afterward. A
program or a device that restarts finds the state of the world waiting for
it, instead of sitting wrong until the next change. Retention is a design
choice here, not an optimization.

The exceptions are as deliberate. A **command** is never retained, because
one the broker kept would be carried out again by everything that connected
later. And a retained **presence** is only a claim: something is believed to
be online because it was heard saying so recently, not because the broker
remembers it once did.

## One program decides, so everything agrees

Where several parts need the same answer, one program works it out and
publishes it. `nowplayd` alone decides what is playing; the stand, the
screen and the console show what it says. They cannot disagree because none
of them decides, and a lookup made once is not made once per subscriber.

## Report only what is known

A part says what it is sure of and no more. Music that cannot be named is
"playing", with no track. A spoken request that does not fit the grammar is
answered as not understood. An album that is not in the library is a failed
command. In each case a nearer guess was available and refused: a wrong
answer stated confidently is worse than a gap, because nothing downstream
can tell the difference.

## Devices keep going without help

A device on a shelf should never need a person. So the firmware fails soft:
a scene it cannot read is ignored and the last one keeps playing, an effect
it does not know becomes a solid color, the network is retried forever, and
an update that cannot reach the broker undoes itself. Animation is drawn on
the device, never streamed to it, so a network hiccup cannot make the lights
stutter.

When it truly cannot work, it says so in a way a person can read without a
laptop: the stand blinks a count.

Services take the opposite stance on startup. A setting that is wrong stops
the daemon at once, rather than letting it light the room wrong quietly.

## Ask little of other people's services

EdS leans on services it does not run: a recognizer, a record collection,
catalogs of albums and covers, a streaming account. It asks each one as
little as it can, once rather than once per subscriber, and every call is
counted. Anything that depends on a service nobody promised is kept in one
place, so that losing it changes one file.

## Secrets never live in the repository

Credentials are never compiled into firmware or committed beside the code.
Devices read theirs from a partition written at provisioning; services read
theirs from the environment. Only what is not secret, a network name or an
account name, has a default in the source.

## Built to be tested without the hardware

The logic that matters is kept apart from the hardware it runs on, so that
it can be tested on a laptop. Firmware is tested on the host; the screen's
pictures are drawn to files; `palette` is tuned against a folder of JPEGs
and tested on synthetic sleeves made to show each kind of trouble. What
cannot be separated is kept thin.

## Decisions are written down

Every place where more than one approach was viable has a record of what
was chosen, what was turned down, and why. A question settled once is looked
up, not argued again. When the answer changes, a new record says so, and the
old one is left as it was.

## In this section

- [Background: Sound Around the House](spdif/): the whole-house audio
  system that was here first, and what EdS borrows from it.
- [Subsystems and Components](subsystems/): the whole system in one
  picture, the foundation beneath it, and a page for each subsystem:
  [Now Playing](subsystems/now-playing/),
  [LP Light Show](subsystems/lp-light-show/),
  [Remote Display](subsystems/remote-display/) and
  [Voice Request](subsystems/voice-request/).
- [Layer Boundaries](boundaries/): which layer owns which color decision.
- [Proposals](proposals/): ideas EdS expects to build on but has not
  decided: [a music inventory](proposals/music-inventory/) and
  [capturing vinyl](proposals/capturing-vinyl/).
- [Decisions](decisions/): every choice, argued.

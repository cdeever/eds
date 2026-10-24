---
title: "Subsystems and Components"
weight: 2
---

# Subsystems and Components

EdS is one system made of **subsystems**, and each subsystem is made of
**components**.

| Term | What it is | Example |
|---|---|---|
| **System** | EdS as a whole: everything here, and the bus it meets on | EdS |
| **Subsystem** | something a person in the room experiences from end to end, with a name of its own | the Light Show |
| **Component** | one program or one device, built, tested and deployed on its own | `lightd`, the LP stand |

A subsystem is named for what it does for a person, not for the parts it is
made of. The Light Show is a stand that glows in the colors of the record;
that it takes a Go service, a Python service and an ESP32 to do it is the
subsystem's own business.

Every component belongs to exactly one subsystem, or to the foundation they
all stand on. A component that seems to belong to two is usually two jobs in
one place, and is a sign that a boundary has been drawn in the wrong spot.

## The subsystems

| Subsystem | What a person sees | Components |
|---|---|---|
| [**Now Playing**](#now-playing) | nothing directly: it is what every other subsystem knows about the music | the drivers, the recognizer, the power beacon, `nowplayd` |
| [**Light Show**](#light-show) | the LP stand lit in the colors of the record on the turntable | `lightd`, `palette`, the LP stand |
| [**Remote Display**](#remote-display) | a screen showing the cover, with buttons to pause, skip and set the volume | the cover screen |
| [**Voice Request**](#voice-request) | an album played because someone asked for it out loud | the ears, `voiced`, `scribe`, the library index |

Beneath them is the [**foundation**](#the-foundation): the broker, the console,
updates, logs, and where it all runs.

### Now Playing

Knowing what is playing is the one thing every other subsystem needs, so it
is a subsystem of its own and the others are built on it.

- **The drivers** (`npagent`, on the agent Pi): one for each source of
  music, iTunes, Spotify and the turntable, each reporting what its player
  is doing and carrying out commands where the player can take them.
- **The recognizer** (`eds-recognize`): a WAV in, a name out, for the
  turntable driver.
- **The power beacon**: an ESP32 on the turntable's switched outlet, whose
  connection says the turntable is on.
- **`nowplayd`**: decides which source is current, names the record from
  the owner's collection, finds the cover, and publishes one answer for
  everyone else. It also routes every command to the source that should
  carry it out.

What it offers the rest of EdS is a set of topics: `current`, the cover at
two sizes, the look, and the command route
([Now Playing](/docs/reference/now-playing/)).

### Light Show

The first vertical, and the reason EdS exists: a cover becomes light.

- **`lightd`**: takes the cover from Now Playing, asks `palette` for its
  colors, makes them fit for a strip, and sends the stand a scene.
- **`palette`**: image bytes in, colors out. It knows nothing of music or
  of LEDs.
- **The LP stand**: an ESP32 and 18 LEDs, animating the scene on its own.

The stand is lit for records; for other sources it is dark
([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/)).
How each layer divides the color decisions is in
[Layer Boundaries](../boundaries/).

### Remote Display

A screen beside the stand that shows what is playing and can change it
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)).

- **The cover screen**: an ESP32 with a 320 by 240 touch panel. It shows the
  small cover and the title, draws buttons when the source can take
  commands, sends what is pressed, and sleeps when nothing plays.

It is one component, and it is a subsystem all the same: it is something a
person uses on its own, and it would still be worth having with no stand
beside it. A second screen, or a remote of another kind, would join it here.

### Voice Request

An album played because someone asked for it out loud
([ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/)).

- **The ears**: a reSpeaker Lite running EdS's own firmware. It sends what
  was said as one WAV, and says when it has been woken.
- **`scribe`**: a WAV in, words out. It knows nothing of music.
- **`voiced`**: reads the words against a short grammar, counts an artist's
  albums in MusicBrainz when a place is named, chooses from the owner's
  library, and sends `play_album`. It turns the music down while a request
  is being heard.
- **The library index**: what the owner's iTunes library holds, made from
  its XML and never committed.

It uses Now Playing's command route like any controller: it asks for an
album, and `nowplayd` hands the request to the source that can play it.

### The foundation

What every subsystem stands on, and that no subsystem owns.

- **The broker** (`mqtt01`, Deevnet IoTaaS's): where everything meets.
- **The console**: the page a person looks at, showing every subsystem at
  once, and switching whether the stand is lit for records or for every
  source.
- **Updates** (`eds-ota`): new firmware for every device, through the
  broker.
- **Events and counts**: devices' events, carried to the tenant's log
  store, and the count of what is asked of outside services.
- **Where it runs**: the tenant's workload, the agent Pi, their images and
  their deployment.

## How subsystems meet

Subsystems meet only where components do: at the broker, on topics. Nearly
all of it goes through Now Playing. The Light Show and the Remote Display
read what it publishes; the Remote Display and Voice Request send it
commands. No subsystem subscribes to another's internal topics, and none
knows another is there.

That is what lets a subsystem be added, changed or switched off without the
others noticing. The Light Show was running before the Remote Display
existed, and Voice Request was added without either of them changing.

## Adding a subsystem

A new subsystem starts with a name, for what it does for a person, and a row
in the table above. Then:

- Its components are its own. A component taken from another subsystem to
  save effort is a coupling that will have to be undone.
- It reads from Now Playing what it can, and asks for what is missing
  there, not by subscribing past it.
- What it needs from another subsystem that is not already offered on a
  topic is a decision to record first. So is a new command.

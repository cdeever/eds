---
title: "ADR-0020: A Touch Screen That Can Change What Plays"
weight: 20
---

# ADR-0020: A Touch Screen Shows the Cover and Can Change What Plays, by Asking nowplayd

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a button on the screen has paused the real iTunes; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-11 |
| **Scope** | A second device, a small touch screen, that shows the cover of what is playing and has five buttons: play or pause, previous, next, volume up, volume down. This is the first thing in EdS that changes anything outside itself. |
| **Extends** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), in which a driver reports and reads nothing, and [ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/), which paired with iTunes as a remote and used the pairing only to read |
| **Related** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/), [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/), [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/), which said controls would be a decision of their own |

---

## Context

The stand shows a record's colours. A screen beside it can show the cover
itself, and a screen that can be touched is the obvious place for the few
controls a listener reaches for.

The board is an ESP32-2432S028R: a plain ESP32 with 4 MB of flash and no
external memory, a 320 x 240 ILI9341 panel, and an XPT2046 resistive touch
controller. With WiFi and TLS up it has roughly 100 KB of heap free, and no
single piece of it large enough for a decoded picture.

Until now everything in EdS flowed one way. Drivers report a player and read
nothing; every other program reads the bus and writes only about itself. The
console was built to "only look", with controls named as a later decision.
This is that decision, made for a device rather than for the page.

Three questions had to be answered: how a button press reaches a player, how
a cover reaches a screen that cannot decode one, and what a device that can
skip a track is allowed to do.

## Options considered

### How a press reaches the player

**The screen talks to iTunes itself.** It cannot. The screen is on the IoT
segment and iTunes is on the home LAN; the agent Pi exists because nothing
else bridges the two
([ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/)).

**The screen publishes to the driver's own topic**, read out of `current`'s
`source`. One hop fewer. But it puts a controller on the drivers' side of the
contract, which every other subscriber is kept off, and gives a device on the
IoT segment a grant over every source's command topic.

**The screen publishes to one topic, and nowplayd passes it on.** nowplayd
already decides which source is current; it decides where a command goes by
the same rule. A controller says what it wants of "what is playing" and
knows no player. One more hop, through a service that has to be up anyway
for there to be a current track.

### How the cover reaches the screen

**The screen decodes the cover as published.** It is up to 600 pixels, may
be a PNG or a progressive JPEG, and may be several hundred kilobytes. The
ROM's decoder reads baseline JPEG only, and there is no memory to hold either
the file or the picture.

**`lightd` makes the screen's picture.** It already turns a cover into what
a device shows. But it is the light daemon, and this would make it the
picture daemon too.

**A new service makes it**, as `lightd` does for the stand. The cleanest
layering, at the cost of a container, an account and a deployment for one
resize.

**nowplayd publishes a small copy beside the cover.** nowplayd already
promises every subscriber a cover
([ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/));
a small one is the same promise in a size more subscribers can use. It
learns to scale a picture and learns nothing about any screen.

## Decision

A new firmware, **`firmware/cover-screen`**, for a device registered as
`cover-screen-01`.

**Commands go to nowplayd, and nowplayd passes them on.**

- A controller publishes to `nowplaying/command`:
  `{"v":1,"command":"next","from":"cover-screen-01"}`. The commands are
  `play_pause`, `next`, `previous`, `volume_up` and `volume_down`. `from` is
  for the logs and is not trusted for anything.
- nowplayd forwards the message unchanged to
  `nowplaying/source/<id>/command`, where `<id>` is the source of the current
  track. A paused track keeps its source, so `play_pause` resumes it. With no
  current source the command is dropped, and logged.
- The iTunes driver subscribes to its own command topic and carries the
  command out over the pairing it already holds. Volume moves five points in
  a hundred a step.
- **A command is never retained, and a retained one is ignored** by nowplayd
  and by the driver alike. A command is for now.
- **The screen sends at QoS 0 and only while connected.** A press that is
  lost is pressed again; a press delivered after a reconnect skips a track
  nobody is asking to skip.
- **Spotify takes no commands.** Its driver subscribes to nothing and its
  authorization stays read-only
  ([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).
  A command while Spotify is current is forwarded to a topic nobody reads.
- A program that cannot subscribe to its command topic still does everything
  else. The grant and the program are deployed separately, and a refused
  subscription must not stop an agent reporting.

**nowplayd publishes a small cover.**

- Beside each cover on `nowplaying/current/art` it publishes
  `nowplaying/current/art/small`, retained: a baseline JPEG whose longer side
  is 240 pixels, aimed at no more than 32 KB.
- A cover that cannot be made small is published as an empty message, so a
  subscriber is not left holding the album before's.
- When no cover is published at all, the small topic is left alone, as the
  cover topic is.

**The screen follows `current` and nothing else.**

- It shows the small cover while `current` names a cover, and a drawn record
  otherwise. That is what keeps it from showing an earlier album's cover
  under a Spotify track.
- A touch brings up five buttons over the dimmed cover; they go after five
  seconds. The first touch only brings them up.
- It renders locally, in bands, from the JPEG it was sent. It never receives
  frames ([ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/)).
- Its two secrets are provisioned, never compiled in
  ([ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/)),
  and it reports its events as the stand does
  ([ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/)).

**What its account may do**: publish its own presence, its own log, and
`nowplaying/command`; subscribe to `nowplaying/current` and the small cover.
Nothing else.

## Consequences

- **A device on the IoT segment can now pause the music and change its
  volume.** That segment is rated for devices assumed reachable by things
  EdS did not write. Whoever takes this screen's credential can do what its
  five buttons do, and nothing more: the account cannot write a scene, a
  track or anyone else's presence. The harm is bounded to a nuisance, and
  that bound is the list of five commands. Adding a sixth is worth a thought.
- **Volume has no ceiling.** Sixteen presses of `volume_up` from silence is
  full volume. Nothing limits how loud a command can make the room.
- **nowplayd is now in the path of a button press.** If it is down, the
  buttons do nothing. So does the cover.
- **Drivers no longer only report.** The iTunes driver carries out commands
  on the same pairing it reads with. The pairing was always a remote's; this
  is the first use of what a remote can do.
- **The screen does not check that the small cover is the current track's.**
  It trusts `current` to say whether there is one and the topic to hold it.
  Between a track change and the new cover arriving it may show the old one
  for a moment.
- **The screen's code for WiFi, the broker, credentials and events is a copy
  of the stand's.** A fix to one is owed to the other. Sharing it means
  restructuring a stand that works, and is left until a third device asks.
- **The console still only looks.** Nothing here changes its account.
- nowplayd gained an image scaler, written against the standard library.
- A source that takes commands does not say so anywhere. A controller cannot
  tell that its buttons will do nothing while Spotify is current.

## What would settle it

1. A button on the screen pauses and resumes the real iTunes, and skips a
   track. In particular: whether iTunes accepts a command from a client that
   sends `Viewer-Only-Client: 1`, as the driver does on every request.
2. Volume up and down move iTunes' volume, by the step intended.
3. The small cover, as nowplayd makes it from real covers, decodes on the
   board and is the right way up, in the right colours.
4. The board holds a connection to the broker over TLS with the cover's
   memory taken, across a day.
5. Touches land where the finger is. The panel's orientation and the touch
   controller's calibration are settings, with defaults that are a guess
   until the board has been touched.
6. The substrate grants `nowplaying/source/+/command` to publish, with its
   wildcard.

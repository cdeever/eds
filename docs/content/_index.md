---
title: "EdS"
type: docs
---

# EdS

<p class="subtitle"><em>A house that knows what is playing, and does something about it.</em></p>

EdS builds on the music already playing around the house. It works out what
is on, from a record on the turntable to a stream on the media server, and
adds to it: displays that show it, controls that change it from wherever
you are, and automations that set the room without anyone asking.

Everything EdS adds happens beside the music, never in it.

## The sound is left alone

*Vinyl the way it was cut, in a room that listens along.*

For the purist, this is the whole point. Put a record on and nothing about
the sound changes: in the listening room, the path from stylus to speakers
is analog from end to end, and the signal never becomes a number on its
way. EdS is not in that
path. It listens from a tap off to the side, the way a tape deck would.
Switch EdS off and the stereo plays exactly as it did before.

## The light follows the sleeve

A record's jacket is twelve inches square of album art, and most of the
time it sits in a crate. Prop it on the LP stand and it is on display, lit
from behind in its own colors. EdS reads the cover art, draws out its
colors, and sets them moving behind the sleeve: a slow wash for one record,
a chase of accents for the next. The room takes on the look of the album
for as long as it plays, without anyone choosing a thing.

When the music stops, the light settles to a low, warm glow.

## A screen that belongs to the stereo

Beside the turntable is a small touch screen with one job. It shows the
cover of whatever is playing. Touch it and it becomes a remote for iTunes:
play, pause, skip, louder, quieter, without reaching for a phone. It is
always where you are standing, and it is never busy being anything else.

With a record on, it is simply the sleeve. Nothing can skip a record, so it
does not pretend to.

## It knows without being told

- **Play a record** and EdS hears a few seconds of it, names the track, then
  looks through your own collection for the pressing you actually own. The
  cover it shows is your sleeve, not a reissue's.
- **Play from the library** and it follows along, track by track.
- **Switch the turntable on** and the room becomes the turntable's. Switch
  it off and the room goes back to whatever else is playing. Nobody presses
  anything.

## Room by room

The house already sends music to every room
([Background: Sound Around the House](docs/architecture/spdif/)), and it
can already be controlled: iTunes and Spotify from their phone apps, and the
media server itself over Remote Desktop. EdS does not replace any of that.
It adds ways that are more convenient, and more fun, differently for each
room, and it stays out of the path the music takes.

**Today** that is the vinyl listening room: the LP stand and the screen
beside it, above.

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

## Why build it

A packaged whole-home system would play music in every room too. EdS is
built by hand for what that would give up:

- **No subscription for the system itself.** The house's sound does not
  depend on a company's service or its pricing.
- **Every room keeps its own stereo**, vintage ones included, rather than
  speakers chosen by whoever sells the system.
- **It works with what is already in use**: iTunes, Spotify, your own
  Discogs collection, and whatever comes next, integrated the way you want
  them rather than the way a vendor allows.
- **It listens only when asked.** The wake word is heard on the device
  itself, nothing is sent before it, and what you say is written down on
  EdS's own equipment, not by someone else's service. The one sound sent out is a few
  seconds of a record, to name it.

## Under the hood

A turntable, a few small devices, and a handful of services that meet at one
message bus. [How EdS Runs](docs/architecture/how-it-runs/) has the whole
system in one picture, the [architecture](docs/architecture/) page the
principles it is built on, and every design choice is written down with what
it was chosen over.

<div class="section-cards">
  <a class="section-card" href="docs/architecture/">
    <h3>Architecture</h3>
    <p>The principles EdS is built on, the whole system in one picture, and which layer is allowed to decide what.</p>
  </a>
  <a class="section-card" href="docs/guide/">
    <h3>User Guide</h3>
    <p>Setting up a stand and running the services.</p>
  </a>
  <a class="section-card" href="docs/develop/">
    <h3>Developer Guide</h3>
    <p>Building, testing and bringing up each project.</p>
  </a>
  <a class="section-card" href="docs/reference/">
    <h3>Reference</h3>
    <p>The contracts: HTTP APIs, the scene descriptor, MQTT topics.</p>
  </a>
  <a class="section-card" href="docs/architecture/decisions/">
    <h3>Decisions</h3>
    <p>Why it is built this way, and what was turned down. Read before reopening a question.</p>
  </a>
  <a class="section-card" href="docs/changes/">
    <h3>Changes and Incidents</h3>
    <p>What was changed on something running, and what happened.</p>
  </a>
</div>

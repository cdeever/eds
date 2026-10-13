---
title: "ADR-0028: The Stand Is for Records"
weight: 28
---

# ADR-0028: The Stand Is Lit for Records Only, and Is Dim Only While the Turntable Is On

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until it has been seen in the room; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-13 |
| **Scope** | Which sources light the stand; what it shows when nothing is playing; and how lightd is told, without learning what a source is. |
| **Extends** | [ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/), whose idle scene is now for one case of two, and whose "it is not dark" no longer holds for the other. [ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/): a look can be named for there being no cover, and a look can be "light nothing". [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/): a second way for a source to be reported and not lit, which keeps its cover. [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/): the switch says one thing more. |
| **Extended by** | [ADR-0029](/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong/): the stand is dark from power on too, and blinks when it cannot reach the broker. [ADR-0030](/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source/): one look is shown from a dark source too |
| **Related** | [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/), whose boundary this keeps; [ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/), whose dimmed room between two sides is the case that stays dim |

---

## Context

[ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/)
gave a room with nothing playing a scene of its own: steady, warm, at 0.15.
It chose that over dark because "a dark stand cannot be told from a dead
one".

Lived with, that is a stand that is lit all the time. Nothing plays for most
of every day, and for all of it the strip is on. The dim scene was meant for
a record being turned over, or the minute between switching the turntable on
and the needle coming down: somebody is there, about to play something. It
should not also be what an empty house looks like.

EdS already knows the difference. The turntable's beacon
([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/))
is on exactly while somebody has switched the turntable on.

The same goes for iTunes. It is an LP stand: a sleeve stands on it. Lit
from whatever iTunes is playing in another room, it is lit for a record that
is not there. iTunes and the rest of the media behind the house will get
something of their own, made for them, later. Until then the stand should
be lit by records and by nothing else. But the cover screen and the console
show iTunes' cover and should go on doing so, which rules out the way
Spotify is kept off the stand
([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)):
that publishes no cover at all, and a cover not sent leaves the stand lit
from the last one.

What EdS has no way to do is say either thing to lightd. lightd is told there is no
cover by the cover topic being emptied, and an empty message has room for
nothing else.

## Options considered

**lightd reads the beacon.** One subscription. But lightd would know that a
device on a switch means a record is coming, which is knowledge about music
and equipment that
[ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)
keeps on the other side of the cover topic.

**nowplayd leaves the cover topic alone when nothing is switched on**, and
something else darkens the stand. There is nothing else, and "not sent"
already means "leave the room as it is"
([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).

**A new topic**, say `nowplaying/current/session`. Clear, and it costs a
grant in two broker accounts, a setting in lightd, and one more message to
keep in step with the cover.

**The firmware darkens itself** after a while with no scene. The stand would
decide something about the room, and a retained dim scene would light it
again on every restart.

**iTunes reported without its cover**, as Spotify is. The screen and the
console would lose the sleeve, and the stand would stay lit from the last
record for as long as iTunes played.

**lightd is told which source a cover is from**, and chooses. That is the
knowledge ADR-0009 keeps from it, again.

**A look, for both.** nowplayd already tells lightd, by name, when a
cover should not be lit from its own colors
([ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/)):
a name on the music side, drawn on the light side. A look names its cover.
One that names none can be the look of there being none, and a look can be
"light nothing from this".

## Decision

**The stand is lit from a cover only for the sources that are meant to
light it, which for now is the turntable. When nothing is playing it is
dark, and dim - the idle scene of ADR-0024 - only while some source's
equipment says it is switched on. nowplayd says each of these by naming a
look, and lightd draws it.**

- **`NP_DARK_SOURCES` names the sources no room is lit for**: `itunes` on
  the workload. Their covers are published as before, to every subscriber,
  and each is given the look `dark`:
  `{"v":1,"look":"dark","art":"<sha256>"}`. It comes before any look the
  album has.
- **lightd draws `dark` as `scene.Dark`**, and does not ask the extractor.
  The stand is sent dark, not left alone: iTunes starting as the turntable
  is switched off does not leave the last sleeve's colors up.
- **Nothing else about a dark source changes.** It is arbitrated, shown on
  the screen and the console, and controlled from the screen as before.

- **nowplayd publishes a look with no cover named** on
  `nowplaying/current/look`, retained, before it empties the cover topics:
  `{"v":1,"look":"standby","art":""}` when the beacon of any source in
  `NP_SOURCE_POWER` is on, `{"v":1,"art":""}` when none is.
- **"On" is the beacon's word alone.** A turntable whose driver is offline
  is still a turntable somebody switched on. A beacon never heard from is
  not on.
- **A switch that changes while the room is idle is said at once**, as that
  look again and nothing else, whether or not it changes who is current.
- **lightd shows `standby` as `scene.Idle`, and any other name, or none, as
  `scene.Dark`**: the same scene at brightness 0. It is sent and retained
  like any scene, so a stand that restarts shows its own dim start for the
  moment before it connects, and then goes dark.
- **lightd is still told nothing about why.** It knows a name. It does not
  know what a beacon is, or that there is a turntable.
- **Dark is what lightd shows when it has heard no look at all.** With no
  look topic set, or a nowplayd that does not say, an emptied cover topic is
  a dark stand.
- **The setting is a list of sources and not "whatever is not switched
  on".** They come to the same thing today, since a turntable that is on
  holds the stand. A list says what is meant, and is what is edited the day
  iTunes gets lights of its own.

## Consequences

- **A dark stand can no longer be told from a dead one by looking.** That
  was ADR-0024's reason, and it is given up knowingly. The console still
  tells them apart: the stand's presence, and the scene it was sent, which
  reads "solid · brightness 0%".
- **Switching the turntable on lights the stand dim** within a few seconds
  of the beacon connecting, before any record is named. That is now the
  sign that EdS has noticed.
- **Switching it off darkens the stand, not at once.** The beacon's will
  takes about fifteen seconds, and if nothing else is reporting, nowplayd
  waits `NP_IDLE_AFTER` again before calling the room idle. Half a minute,
  at worst.
- **iTunes plays in a dark room.** The stand does nothing for it, and for
  now neither does anything else. That is the point, until backend media
  gets its own display. The album looks
  ([ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/))
  are seen only from a record.
- **"The stand follows what is playing" is no longer true**, and the pages
  that said so are changed. It follows the record.
- **A beacon stuck `online`** - switched off while the broker was down
  (ADR-0023) - now also leaves the stand dim until the turntable is next
  switched on and off.
- **A lightd from before this lights a `dark` cover from its own colors**,
  as it does any look it does not know.
- **The look topic carries two kinds of message**, told apart by whether
  `art` is empty. One topic holds one retained message, so while a cover is
  up the look of no cover is not retained anywhere; it is published again
  each time the cover is taken away.
- **A lightd from before this refuses a look that names no cover**, logs it,
  and goes on dimming. The two are deployed together.
- **No firmware changes.** The stand already renders brightness 0 as off,
  and has a host test that says so.

## What would settle it

Tested as sequences of events in nowplayd's hub and publisher, and in
lightd's server: a dark source's cover published with its look and lighting
nothing, the look before the emptied topic, the look alone when a
switch changes, both orders of arrival, an unknown name.

Not yet shown:

- [ ] iTunes playing with the turntable off: the stand is dark, and the
  cover screen and the console show the sleeve.
- [ ] The turntable switched on in that dark room: the stand is dim before
  a record is named.
- [ ] A side turned over: dim, not dark, and lit again from the sleeve.
- [ ] The turntable switched off after a record: dark, and how long it took.
- [ ] lightd restarted while the room is idle, both ways: the stand ends as
  it was.

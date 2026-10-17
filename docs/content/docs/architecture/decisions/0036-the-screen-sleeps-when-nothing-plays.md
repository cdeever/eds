---
title: "ADR-0036: The Screen Sleeps When Nothing Plays"
weight: 36
---

# ADR-0036: The Cover Screen Sleeps When Nothing Has Played for Five Minutes, and Wakes for Music

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} 2026-10-17 — slept and woken on the board ([CHG-0026](/docs/changes/2026/0026-the-cover-screen-sleeps/)); see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-17 |
| **Scope** | The cover screen's firmware: when it turns its panel off, and what wakes it. Nothing on the bus changes. |
| **Extends** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), the screen itself |
| **Related** | [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), which makes a turntable that is switched off not heard; [ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/), which turns a long pause into stopped; [ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/), the last time the screen had to be told something without knowing a source; [ADR-0029](/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong/), the stand's dark that is not a fault |

---

## Context

The cover screen is lit all the time. With nothing playing it shows a drawn
record all night, to nobody. Its owner asked first for it to sleep when the
turntable's power, iTunes and Spotify are all gone, and to come back when
one of them is.

The screen knows no source, by decision
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/),
[ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/)),
and its broker account reads two topics. nowplayd, though, already decides
who is there. Its arbiter picks only among sources that are online and not
switched off:

- the turntable's driver is online whenever its sound card is, but while its
  beacon says the outlet is off it is not heard
  ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/));
- iTunes is online while its driver holds a session with the player;
- Spotify is online while Spotify answers its driver.

When none is left, `current` is published as stopped with **no `source`**.
With one, `source` is always named.

The first build slept on exactly that, and did, on the board. Then iTunes
was played and paused, and the screen stayed lit: a paused iTunes is still
there, and after fifteen seconds `current` says stopped with iTunes still
named, so that play can be pressed
([ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/)).
Its owner expected it to sleep, and widened the rule: **nothing playing**,
not only nobody there.

The board's backlight is a plain pin, and the ILI9341 has a sleep command
that keeps what it was showing.

## Options considered

**The screen reads the presence topics itself**: the beacon's and each
driver's `status`. It teaches the firmware the sources' names, widens its
broker account, and puts in the screen a second copy of the arbiter's rule
about switched-off turntables. And it would not see a pause.

**nowplayd says it outright**, with a new field in `current` or a topic of
its own. Clearer to read, and a contract change to say what `current`
already says in `state` and `source`.

**Nobody there: `current` names no source.** Shipped first, and too narrow:
a paused player keeps it lit.

**Nothing playing: `current` does not say `playing`, or names no source.**
Both are in the one message the screen already reads.

## Decision

**The cover screen turns its panel and backlight off after five minutes in
which `current` has not said `playing` with a source named, and turns them
on again when it does.**

- **Five minutes, counted from the last thing that kept it awake**: the
  music stopping or pausing, the source going, a touch, or the broker
  coming back. Paused, then stopped by nowplayd, then nobody there, is one
  quiet, not three: none starts the wait over. A side turned over on the
  turntable is well inside it.
- **Playing wakes it**, and so does a source named where there was none: the
  turntable switched on in a room where nothing was there.
- **A touch wakes it, and only wakes it.** The cover, or the record, comes
  back without the buttons, and the finger has to lift before the next touch
  means anything, as with the first touch on a lit screen. The next touch
  raises the buttons as before, and play is pressed from there.
- **No network or no broker wakes it, and keeps it awake**, with its corner
  marked as before. A dark screen has to mean asleep, not broken; a screen
  that cannot hear `current` cannot know that nothing is playing.
- **Until it has heard `current`, it is awake.** A screen just switched on
  shows that it works.
- **What arrives while it sleeps is kept and drawn on waking**, the picture
  before the light, as at power-on.
- **Asleep is not off.** WiFi and the broker stay up, which is how it hears
  that it should wake, and how an update reaches it
  ([ADR-0035](/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker/)).
- The rule is pure C beside what a touch means (`app/ui/ui.c`), host tested;
  the board's half is two calls into the panel.

## Consequences

- **The screen still knows no source**, and nothing on the bus changed. It
  reads `state`, which it already did, and whether `source` is there.
- **A pause is five minutes of light, then dark.** Play pressed on the Mac
  or the phone wakes it; on the screen, a touch and then play.
- **A turntable switched on in a room where iTunes is paused does not wake
  it**: the source changes and nothing plays. The record starting does.
- **Spotify does not keep it awake** unless something plays, though its
  driver is online whenever Spotify answers. It is off
  ([CHG-0024](/docs/changes/2026/0024-spotify-turned-off-again/)).
- **A screen with older firmware stays lit**, as before.
- **The stand is not this.** Its dark and dim are lightd's, from the look
  ([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/));
  the screen's sleep is its own.

## What would settle it

Tested on the host: it sleeps five minutes after the music stops, pauses or
the source goes, and not before, and none of those after another starts the
wait over; playing, a source named, a touch, or losing the broker wakes it;
a touch that wakes it neither raises the buttons nor sends anything.

On the board, the first build
([CHG-0026](/docs/changes/2026/0026-the-cover-screen-sleeps/)):

- [x] With iTunes unreachable and the turntable off: `screen.asleep` five
      minutes after it connected, still on the broker; dark to the eye.
- [x] A song started in iTunes: `screen.awake`, the cover shown.

On the build with the wider rule:

- [x] iTunes paused: `screen.asleep` five minutes after the screen heard
      `current`, iTunes still named.
- [x] A touch on the dark screen lit it without buttons; the next touch
      raised them and play and pause worked; dark again five minutes after
      the last touch.
- [x] Play pressed away from the screen, with it dark: `screen.awake` and
      the cover, with no `command.sent` from the screen before it.

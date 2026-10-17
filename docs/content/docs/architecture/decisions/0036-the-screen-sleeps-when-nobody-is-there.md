---
title: "ADR-0036: The Screen Sleeps When Nobody Is There"
weight: 36
---

# ADR-0036: The Cover Screen Sleeps When No Source Is There, and Wakes for One

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the screen has slept and woken on the board; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-17 |
| **Scope** | The cover screen's firmware: when it turns its panel off, and what wakes it. Nothing on the bus changes. |
| **Extends** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), the screen itself |
| **Related** | [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), which makes a turntable that is switched off not heard; [ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/), the last time the screen had to be told something without knowing a source; [ADR-0029](/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong/), the stand's dark that is not a fault |

---

## Context

The cover screen is lit all the time. With the turntable switched off and
nothing playing on the Mac it shows a drawn record all night, to nobody. Its
owner asked for it to sleep when the turntable's power, iTunes and Spotify
are all gone, and to come back when one of them is.

The screen knows no source, by decision
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/),
[ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/)),
and its broker account reads two topics. nowplayd, though, already decides
exactly this. Its arbiter picks only among sources that are online and not
switched off:

- the turntable's driver is online whenever its sound card is, but while its
  beacon says the outlet is off it is not heard
  ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/));
- iTunes is online while its driver holds a session with the player;
- Spotify is online while Spotify answers its driver.

When none is left, `current` is published as stopped with **no `source`**.
With one, `source` is always named.

The board's backlight is a plain pin, and the ILI9341 has a sleep command
that keeps what it was showing.

## Options considered

**The screen reads the presence topics itself**: the beacon's and each
driver's `status`. It is the literal rule, and it teaches the firmware the
sources' names, widens its broker account, and puts in the screen a second
copy of the arbiter's rule about switched-off turntables.

**nowplayd says it outright**, with a new field in `current` or a topic of
its own. Clearer to read, and a contract change to say what `current`
already says by leaving `source` out.

**The screen sleeps when `current` names no source.** One field it already
receives, read for one more thing.

## Decision

**The cover screen turns its panel and backlight off after five minutes in
which `current` has named no source, and turns them on again when one is
named.**

- **Five minutes, counted from the last thing that kept it awake**: the
  source going, a touch, or the broker coming back. A driver that loses its
  player and finds it again within minutes is not worth a blink of the
  screen.
- **A touch wakes it, and only wakes it.** The cover, or the record, comes
  back without the buttons, and the finger has to lift before the next touch
  means anything, as with the first touch on a lit screen. The next touch
  raises the buttons as before.
- **No network or no broker wakes it, and keeps it awake**, with its corner
  marked as before. A dark screen has to mean asleep, not broken; a screen
  that cannot hear `current` cannot know that nobody is there.
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
  sleeps because nowplayd has nobody to name, and nowplayd's rule is the
  only copy of who counts.
- **Each source counts as it is reported**, and so:
  - **Spotify would keep the screen awake for good.** Its driver is online
    whenever Spotify answers, whatever is or is not open. It is off
    ([CHG-0024](/docs/changes/2026/0024-spotify-turned-off-again/)); before it is turned
    on again, either its presence comes to mean a named device is active,
    or the screen stays lit while it runs.
  - **iTunes keeps it awake while the Music app is reachable**, paused or
    not. That is what was asked; sleeping after a long pause as well would be
    a further decision.
  - **A turntable whose beacon has never been heard counts**, its driver
    being online. That is ADR-0023's "never heard from, nothing changes",
    and the safe way round: awake, not dark.
- **A screen with older firmware stays lit**, as now.
- **The stand is not this.** Its dark and dim are lightd's, from the look
  ([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/));
  the screen's sleep is its own.

## What would settle it

Tested on the host: it sleeps five minutes after the source goes and not
before; a source, a touch, or losing the broker wakes it; a touch that wakes
it neither raises the buttons nor sends anything.

Not yet shown:

- [ ] iTunes quit and the turntable switched off: the screen dark five
      minutes later, and the board still on the broker.
- [ ] Switching the turntable on wakes it, showing the record, then the
      sleeve.
- [ ] A touch on the dark screen lights it without buttons, and it sleeps
      again five minutes later.

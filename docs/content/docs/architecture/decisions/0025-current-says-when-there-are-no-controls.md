---
title: "ADR-0025: Current Says When There Are No Controls"
weight: 25
---

# ADR-0025: The Current Track Says When There Is Nothing a Controller Could Do

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the screen has been touched with a record playing; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-12 |
| **Scope** | One optional field in `current`, said by nowplayd, and what the cover screen does with it. |
| **Extends** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), which noted that "a source that takes commands does not say so anywhere". Now the current one does. |
| **Related** | [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), which made the turntable current for as long as it is on, and so made the screen's buttons idle for as long |

---

## Context

Touch the cover screen and five buttons come up. With a record on the
turntable every one of them does nothing: a command goes to the source that
is current, and nothing can skip a record. Since
[ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)
the turntable is current the whole time it is switched on, so that is the
whole evening. The sleeve is what the screen is for then, and buttons over it
are wrong twice: they hide it, and they lie.

The screen cannot tell. It knows no player and no source, by decision
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)),
which is what lets another player take over without the screen being told.

## Options considered

**The firmware looks for `"source":"turntable"`.** One line. And the screen
then knows a source by name, a second source without controls is a firmware
release, and the rule is flashed into a device.

**The driver says whether it takes commands**, in its state or on a topic of
its own. The most truthful: the driver is what knows. It is also a change to
every driver and to the contract they all share, for a fact that today is
one word in one place.

**nowplayd is told which sources have no controls, and says so in
`current`.** A setting, like the ones that say which sources have no cover
and which play the owner's records.

## Decision

**`current` carries `"no_controls": true` when the current source is one
named in `NP_NO_CONTROLS_SOURCES`. A controller that reads it offers
nothing. The cover screen draws no buttons, and a touch does nothing.**

- **Left out, controls are offered.** That is every message from before the
  field and every reader that does not know it. The contract stays `v: 1`.
- **It is said on `current` only**, by nowplayd. A driver does not say it.
- **It is about offering, not refusing.** nowplayd passes on a command for
  such a source exactly as before, to nobody listening.
- **Taking the controls away takes down any buttons that are up**, and lets
  go of one that is held.
- **The list is what is in the room, not what could be true**: `turntable`.
  Spotify takes no commands either and is not listed, because it is off
  ([CHG-0011](/docs/changes/2026/0011-spotify-turned-off/)) and its owner
  wants its buttons when it is back; whether they should then do something
  is a different decision.

## Consequences

- **The screen is a sleeve while a record plays**, and a remote otherwise.
- **The screen still knows no source.** It is told there is nothing to
  offer, and not why.
- **A screen with older firmware shows buttons that do nothing**, as now.
- **nowplayd says something it does not check.** Nothing stops the setting
  naming a source that does take commands, and then its buttons are gone.
- **Idle is not this.** A track paused too long
  ([ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/))
  still has its controls, which is how it is played again.

## What would settle it

Tested on the host: with no controls a touch raises nothing and sends
nothing, and losing them takes the buttons down.

Not yet shown:

- [ ] A record playing, the screen touched: the sleeve and nothing else.
- [ ] iTunes playing after it: the buttons are back, and work.

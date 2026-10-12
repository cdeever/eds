---
title: "ADR-0024: Paused Long Enough Is Not Playing"
weight: 24
---

# ADR-0024: A Track Left Paused Is Not Playing for Ever, and a Room With Nothing Playing Is Dim

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until it has been seen in the room; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-12 |
| **Scope** | What nowplayd says when the current track has sat paused, or nobody is reporting; how that reaches the stand; what the stand shows then. |
| **Extends** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), which left "what a subscriber does with stopped" to the subscriber and "paused on B is more use than silence" without an end. [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/): an emptied cover topic now means something to lightd. |
| **Related** | [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/), whose "no cover sent" this must not be mistaken for; [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), whose held source is left alone; [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), whose play button must go on working |

---

## Context

Pause iTunes and walk away, and the stand goes on showing the album for as
long as nothing else plays: hours, overnight. The console says "Paused on
iTunes" over the same sleeve. Neither is false, and neither is what the room
is doing, which is nothing.

Nothing in EdS has a way to say "nothing is playing" to the stand. lightd
lights the room when a cover arrives and does nothing otherwise, on purpose:
that is how Spotify is reported without lighting it
([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).
So a cover not being sent already has a meaning, and it is "leave the room
as it is".

## Options considered

**lightd reads `current` and dims on paused.** lightd would know about
tracks and play states, which is the knowledge
[ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)
keeps on the other side of the cover topic.

**The driver stops reporting a paused track after a while.** Each driver
would grow the same timer, and iTunes would be misreported at its source:
the console's list of players should go on saying what each one is doing.

**A new field in `current`**, `idle`, leaving the track and the cover
there. Honest, and every subscriber would have to learn it, the firmware's
included; one that had not would show the track as before. And the stand
follows the cover topic, not `current`.

**nowplayd says stopped, and takes the cover away.** No new field: `current`
is a state every reader already reads, and the cover topic is emptied, which
no reader can take for a cover.

## Decision

**When the current track has been paused for `NP_IDLE_AFTER`, fifteen
seconds, nowplayd publishes `current` as stopped, with no track, and empties
the cover topics. lightd answers an emptied cover topic with an idle scene:
steady, warm, and dim.**

- **The source is still named** in that `current`, and commands still go to
  it. A pause is undone from where it was made: the screen's play button
  works on a track that is no longer shown.
- **Nobody reporting at all goes the same way** after the same wait, with no
  source named. Before the wait it is as it was, so an agent restarting does
  not dim the room.
- **Stopped and still current waits five minutes**
  (`NP_IDLE_AFTER_STOPPED`). That is a source being held
  ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)).
  A record between its sides keeps its sleeve; a turntable left on with
  nothing on it does not light the room all night. This was first written as
  "left alone", and was wrong within a minute of being deployed: the
  turntable was on and silent, a track paused in iTunes was not the winner,
  and the room stayed lit at full
  ([CHG-0016](/docs/changes/2026/0016-idle-the-screen-on-the-console-and-no-buttons-for-a-record/)).
- **The wait is counted from when the source says it paused**, where that is
  in the past. A track paused an hour ago is idle the moment nowplayd starts.
- **Emptied means no cover; not sent means no change.** The two are kept
  apart. Spotify's track sends no cover and the room stays as it was.
- **The idle scene is lightd's** (`scene.Idle`): solid, `255, 170, 90`, at
  0.15. It is the scene the firmware starts with, so an idle stand and one
  just switched on are the same sight. It is not dark: a dark stand cannot
  be told from a dead one.
- **lightd takes hearing no cover, five seconds after subscribing, as an
  emptied topic.** An emptied retained topic is not delivered to a late
  subscriber, and a lightd that started while the room was idle would
  otherwise never be told.
- **Playing again publishes everything again**, the same cover and its look
  included, and lightd lights the room from a cover it has already held.

## Consequences

- **The stand and the console say the same thing as the room.** The console
  needed no change: it already shows "Nothing is playing" for stopped, an
  emptied cover as none, and the stand from the scene it was sent.
- **"Stopped" on `current` now has two meanings**: nothing loaded, and
  paused too long. The per-source topics still tell them apart, and the
  console's list of players still says "paused".
- **The paused track cannot be seen on the cover screen**, only resumed from
  it. It shows the record, and its buttons.
- **Fifteen seconds is short.** A pause to answer the door dims the room.
  That is what was asked for, and it is a setting.
- **lightd knows one thing more**: that a topic can be empty. It still is
  not told what stopped, or that anything did.
- **A lightd restart with no cover retained dims the stand**, where before
  it left it alone. With the HTTP input only, nothing changes: the rule is
  the cover topic's.
- **An agent that is down for more than fifteen seconds dims the room.**

## What would settle it

Tested as sequences of events in nowplayd's hub and publisher, and in
lightd's server; the broker test for an emptied topic was turned round, from
"not handed on" to "handed on as no cover".

Not yet shown:

- [ ] iTunes paused: fifteen seconds later the stand is dim and the console
  says nothing is playing, with the stand drawn dim.
- [ ] Play pressed on the cover screen after that: iTunes plays, and the
  stand is lit from the same cover.
- [ ] The services restarted with iTunes paused long ago: the stand is not
  lit first.
- [ ] The turntable on and silent for five minutes: the stand is dim. Then a
  record: lit from its sleeve.

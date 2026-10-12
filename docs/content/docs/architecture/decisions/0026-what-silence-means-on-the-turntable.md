---
title: "ADR-0026: What Silence Means on the Turntable"
weight: 26
---

# ADR-0026: A Gap Between Tracks Is Listened For, and a Silent Turntable Is Idle in Twenty Seconds

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a side has been played to it; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-12 |
| **Scope** | What the turntable driver does with a short silence, and how long nowplayd keeps the cover of a source that is stopped and held. |
| **Extends** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/), whose "no single answer after that" now has one exception, and [ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/), whose five minutes for a stopped, held source is taken back |
| **Related** | [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), which is why a stopped turntable is still current; [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/) |

---

## Context

Two things were seen with records playing, on the day the driver went into
service.

**A new track took thirty to sixty seconds to be named.**
[ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/)
expected "about half a minute" and this is where it comes from. The driver
used silence for one thing, deciding the record had stopped, and the two to
four seconds between tracks meant nothing to it. Clips went by the clock,
every twenty seconds, wherever the gap fell:

- the first clip of a new track could be twenty seconds away;
- it often held the end of one track and the start of the next, and was
  named as either, or as nothing;
- a different name was believed only when a second clip agreed, twelve
  seconds later;
- a clip that came back unnamed in between threw the first name away, and
  the wait began again.

**The stand stayed lit for five minutes after the music ended.** The driver
says `stopped` after twenty seconds of silence, and nowplayd then waited
`NP_IDLE_AFTER_STOPPED`, five minutes, before taking the cover away
([ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/)):
long enough to turn a record over. Its owner would rather the lights were on
for music and not otherwise.

## Options considered

### For the track change

**Send clips more often.** Ten seconds is the least the driver allows. It
halves one of the waits, doubles what is asked of an endpoint nobody
promised, and leaves the clips straddling.

**Believe every answer at once.** Fast, and one wrong answer repaints the
room, which is what ADR-0021's second opinion is there to stop.

**Listen for the gap.** A gap says when a track began, so a clip can be
taken that is all one track, and as early as there are twelve seconds of it.
It costs about one more clip per track. It does nothing for tracks that run
together, which are then as slow as before, and a quiet passage looks the
same as a gap.

### How much to believe the clip after a gap

**Two answers, as for any change.** About twenty-five seconds.

**One answer.** About thirteen. The driver already believes one answer
after the needle comes down, for the same reason: the clip is known to be
of one thing.

**One answer, from a shorter clip.** About nine seconds. Only twelve-second
clips have been tried against this recognizer.

### For the lights

**Leave five minutes.** The sleeve is kept while a record is turned over,
and the room is lit for five minutes of nothing after every side.

**Idle as soon as the driver says stopped.** The lights follow the music.
Turning a record over dims the room and brings it back.

**Dark, not dim.** Considered and not chosen:
[ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/)'s
idle scene stays, because a dark stand cannot be told from a dead one.

## Decision

**A second of quiet with music after it is taken for the gap between two
tracks. The driver starts its listening over, sends a clip as soon as it has
twelve seconds from after the gap, and believes that one answer.**

- **Only that clip is believed alone.** A clip sent by the clock still needs
  a second to agree before it replaces a track.
- **A clip that was with the recognizer when the gap came is not that
  clip.** Its answer is about what was before the gap.
- **A quiet passage is asked about like a gap**, and the answer is the track
  already playing. Nothing changes and one clip was spent.
- **Gaps closer together than the clip interval are let pass.** Music that
  stops every few seconds would otherwise never fill a clip.
- **A gap soon after the needle comes down counts.** The thump, the lead-in
  and then the record: the first clip is of the record.
- **Quiet is the level that already means silence**
  (`NP_TURNTABLE_SILENCE_DB`, -55 dBFS). If the gaps on this input are not
  that quiet, none of this happens and the driver behaves as it did.

**A source that is stopped and held is idle after one second**
(`NP_IDLE_AFTER_STOPPED=1s`, and the default). The driver has heard twenty
seconds of silence before it says `stopped`, so the stand is dim about
twenty seconds after the last sound. The wait stays a setting of its own;
`0s` still means never.

## Consequences

- **A track that follows a gap is named about thirteen seconds in**, not
  thirty to sixty.
- **A wrong name can now be shown on one answer**, after a gap or a quiet
  passage, until the next clip. The cover is unlikely to move with it: the
  driver keeps the album a side began with, and the owner's collection names
  the record
  ([ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/)).
- **Tracks that run together are as slow as before.**
- **About one more request to the recognizer per track**, and never two
  clips less than twelve seconds apart.
- **Turning a record over dims the room and blanks the sleeve.** Both come
  back when the first track of the other side is named, about thirteen
  seconds after the needle is down, and with the same sleeve: the driver
  remembers the album for ten minutes.
- **ADR-0024's "a record between its sides keeps its sleeve" no longer
  holds.** Its other rules do.

## What would settle it

Tested as sides played to the follower in no time: a gap and a new track, a
quiet passage, music that keeps stopping, tracks that run together, an
answer that arrives after the gap for a clip sent before it, and a lead-in.

Not yet shown:

- [ ] The level of this input in the gap between two tracks, against
  -55 dBFS. Everything about the gap rests on it, and it has not been
  measured.
- [ ] A side played: each track named, and how long after its gap.
- [ ] A quiet passage that did not change the title.
- [ ] The end of a side: the stand dim and no sleeve on the console or the
  cover screen about twenty seconds later.
- [ ] The other side: the sleeve and the lights back with its first track.

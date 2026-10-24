---
title: "ADR-0044: The Music Is Turned Down While a Request Is Heard"
weight: 44
---

# ADR-0044: The Music Is Turned Down While a Request Is Heard, and Comes Back by Itself

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a request said over the music has been heard to its end and played; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-24 |
| **Scope** | What happens to what is playing between the wake word and the answer: who says the device woke, which command lowers the music, and what brings it back. |
| **Extends** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), whose commands this adds two to; [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/) and [ADR-0043](/docs/architecture/decisions/0043-the-respeaker-runs-firmware-of-edss-own/), whose request it is |
| **Related** | [ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/) |

---

## Context

The reSpeaker's XMOS chip cancels the echo of its own speaker, not of the
stereo. With an album playing beside it, a request asked over the music was
heard as speech until its six-second limit, sent at twice the size, and
written down with the music in it (CHG-0037). Smart speakers lower their
music while they listen. EdS's music comes from players EdS controls.

## Options considered

**Pause while listening.** One command, and silence is best for hearing.
A wake word heard by mistake stops the music, and resuming is only right if
nothing else was asked for.

**Turn the volume down** to a quarter, and back. The music never stops, and
a mistaken wake is a moment of quieter music.

**Do nothing**, and let requests over music run long and be misheard.

## Decision

**The device says it woke; voiced asks for the music to be ducked; the
player's driver turns it to a quarter and brings it back by itself.**

- **`voice/<device>/woken`**, `{"v":1}`, never retained, is published the
  moment the wake word is heard. The device still commands nothing.
- **voiced sends `duck`** on `nowplaying/command` when it hears it, and
  **`unduck`** once it has answered the request, understood or not.
  nowplayd passes both to the current source, as the buttons are passed.
- **The iTunes driver ducks** to a quarter of the volume it finds, and keeps
  what it was. It comes back on `unduck`; before a `play_album` starts, so
  the album is not quiet; and by itself ten seconds after the last `duck`,
  so a request that never comes cannot leave the room quiet. A `duck` while
  ducked lowers nothing further and waits ten seconds again.
- **A volume press while ducked ends the duck where it is**: whoever pressed
  it has taken the volume over, and nothing is restored over them.
- **A source that cannot duck** says so, as for any command it does not
  take. The turntable takes no command at all.

## Consequences

- **The music dips at every wake**, a mistaken one included, for up to ten
  seconds.
- **The first half second of a request is still over the music**: the duck
  is three hops away, through the broker.
- **Two more commands** on the bus, from voiced's account, which already may
  publish `nowplaying/command`.
- **Each account gains one grant**: the device's to publish `woken`,
  voiced's to read `voice/+/woken`.

## What would settle it

Tested against stand-ins: a duck lowers to a quarter and remembers; unduck,
play_album and ten seconds each bring it back; a duck while ducked lowers
nothing; a volume press ends it.

Not yet shown:

- [ ] On the real iTunes: a wake dips the music, and it comes back.
- [ ] A request said over the music ends at the quiet and plays.

---
title: "INC-0004: The Stand Went Dark on One Channel"
weight: 4
---

# INC-0004: The Stand Went Dark Mid-Side, Because the Turntable Was Heard on One Channel and That One Went Silent

| | |
|---|---|
| **Date** | 2026-10-26 |
| **Systems** | `npagent`'s turntable driver and `nowplayd` on the backend Pi; the UFO202 sound card; the Tape 1 output of the amplifier, an Onkyo A-8170 integrated amplifier from the mid-1980s; `lp-stand-01` |
| **Severity** | The stand dropped to standby for the rest of a side, and stayed there through a reboot. Nothing else was lost; every other source and device worked |
| **Status** | Open · {{< inc-status "Mitigated" >}} |
| **Times** | UTC |

---

## Summary

The turntable reaches the sound card through the Tape 1 output of an Onkyo
A-8170, an integrated amplifier from the mid-1980s, whose aged tape selector
contacts can drop a channel. Partway through a side, the
left channel went silent. `npagent` records one channel, and what it records
is the left, so it heard twenty seconds of silence, decided the record had
stopped, and the stand went to standby while the record played on.

Nothing reported a fault, because by every rule in place there was none: the
capture ran, the recognizer answered, and silence on the channel listened to
is what a stopped record sounds like. Turning the amplifier's Tape 1
selector back and forth brought the channel back.

## Impact

From 01:34:53, the stand showed standby's warm white in place of *Eagles
Live*'s colors, for the rest of side four and through the investigation that
followed. The screens and the console showed nothing playing on the
turntable. iTunes, Spotify and every device were unaffected.

## Detection

The owner, who saw the stand go back to standby before or during "Take It
Easy" with the record still playing. Nothing else detected it: the console's
"Song detector" line stayed green throughout, because it says the sound card
is heard, not that music is on both of its channels.

## Timeline

| When | What happened |
|---|---|
| 01:05:36 | The turntable's beacon reports it switched on |
| 01:06:24 | *Eagles Live* is recognized ("Seven Bridges Road"), and the stand is sent its colors |
| 01:21:00 | One track goes unrecognized; twenty seconds later the next is named. The stand keeps its colors, as it should |
| 01:29:47 | "All Night Long (Live)" is recognized: the last track named |
| 01:33:24–01:34:24 | Two calls to the recognizer in the minute, not three: clips have stopped being sent, because the channel listened to is below the silence level |
| ≈01:34:33 | The left channel is silent from here, by the twenty seconds that follow |
| 01:34:53 | `npagent`: "stopped on the turntable" |
| 01:34:54 | `nowplayd` takes the cover away; `lightd` sends the stand `standby` |
| 01:55:49 | The owner switches the stand's sources to all; nothing changes |
| 02:21:29 | The Pi is back from a reboot; the turntable driver is online. The stand goes back to standby at 02:21:35 |
| 02:23:52 | The owner switches the stand's sources to records only; still nothing |
| 02:28:32 | `turntable.capture_lost`: the sound card unplugged, to be tried on another computer, where the record is heard. Back at 02:28:39 |
| later | `arecord -c 2 -V stereo` on the Pi reads `+00%\|76%`: left silent, right playing |
| later | The amplifier's Tape 1 selector is turned through its positions several times and set back to phono; the meter reads `29%\|26%` |

## Symptoms

1. The stand went to standby mid-side, with the record audibly playing.
2. The console showed the turntable stopped, and its "Song detector" green.
3. A reboot of the Pi changed nothing.
4. The sound card played the record on another computer.

## Investigation

- **A cap on the recognizer**, the first suspicion: it is an endpoint nobody
  offered (ADR-0021) and could have stopped answering. Overturned by
  `npagent`'s own counts, `limited=0 failed=0` every minute, and by the code:
  a recognizer that fails only leaves a track unnamed
  (`services/nowplaying/internal/driver/turntable/turntable.go:177–192`); it
  never makes the turntable stop.
- **The side ending** while nobody was there to flip it. Overturned by the
  owner: the stop came before the last track.
- **`npagent` dying.** Overturned by the log: the "stopped" line is its own,
  from the process that had run all evening, and it came back after the
  reboot no better.
- **The wrong card, or a muted capture.** Overturned on the Pi:
  `NP_TURNTABLE_DEVICE=plughw:CODEC` names the only capture device, and the
  card has no capture control to mute.
- **The level, per channel**, which settled it: two channels in, one silent.
  The record played on another computer because a system playing both
  channels plays the right one too. In the other room the amplifier feeds a
  5.1 system, where one channel missing is not obvious.

## Root cause

Two faults combined:

1. **The amplifier's Tape 1 path dropped its left channel.** The Onkyo
   A-8170 is from the mid-1980s, and its tape selector contacts and tape
   jacks have aged: the Tape 1 selector is known to need working to carry
   both channels. It is never heard: listening is from phono, and Tape 1
   out feeds only the sound card.
2. **The turntable driver listens to one channel.** It records mono through
   ALSA's plug conversion (`arecord -c 1`,
   `services/nowplaying/internal/driver/turntable/capture.go:25–26`), and the
   evidence is that the one channel taken is the left: silence on it alone
   gave the driver silence. Twenty seconds below the silence level is a
   stopped record (ADR-0026, `follow.go`), and so the stand went to standby
   with music on the other channel.

## Recovery

The amplifier's Tape 1 selector was turned through its positions several
times and set back to phono, which restored both channels' contacts; the Pi
read both again (`29%|26%`), and `npagent` was restarted with them. The
contacts are only worked, not cleaned, and can fail again.

## Contributing factors

- **Nothing measures the channels apart**, so a one-sided input looks exactly
  like silence, and nothing could say otherwise.
- **The console's "Song detector" was green**, as ADR-0034 defines it: the
  driver was there. It does not say what it hears.
- **The fault is on an output nobody listens to**, so the owner had no way to
  hear it, and the other room's 5.1 system hid it.

## Corrective actions

| # | Action | Where | Status |
|---|---|---|---|
| 1 | Clean and treat both tape selector controls (Tape 1 and Tape 2, whose contacts have aged the same way) and the tape audio jacks | The amplifier | {{< action-status "Open" >}} |

## Preventive actions

| # | Action | Where | Status |
|---|---|---|---|
| 1 | Hear the turntable on both channels, decide music from the louder, and say on the console and in the log when one goes silent | [ADR-0048](/docs/architecture/decisions/0048-the-turntable-is-heard-on-both-channels/) | {{< action-status "Open" >}} |

## Follow-ups

None.

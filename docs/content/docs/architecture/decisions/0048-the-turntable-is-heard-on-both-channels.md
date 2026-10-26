---
title: "ADR-0048: The Turntable Is Heard on Both Channels"
weight: 48
---

# ADR-0048: The Turntable Is Heard on Both Channels, and One Gone Silent Is Said on the Console

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}}, until a side has played through with one channel unplugged; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-25 |
| **Scope** | How many channels the turntable's driver listens to, what it decides from each, and how a channel gone silent is reported: a retained topic beside the source's status, one event, and the console's "Song detector" line. Not how silence, gaps or stops are timed, which is unchanged. |
| **Extends** | [ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/), whose silence is now the louder channel's, and [ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/), whose "Song detector" pill gains a third color for an input heard on one side only |
| **Related** | [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/), the console that only looks; [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/), the driver; [ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/), the agent's events |

---

## Context

**On the evening of 2026-10-25 (about 01:34 UTC on the 26th) the stand went
to standby in the middle of a side.** A record was playing, the Eagles'
*Live*, and the music could still be heard in the room. The turntable's
driver had heard twenty seconds below -55 dBFS and said `stopped`, as
[ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/)
has it do, and nowplayd dimmed the room a second later.

Nothing else had failed. The driver logged no `turntable.capture_lost`: the
sound card was there and sending audio. The recognizer was well: the
minute's counts for the Shazam endpoint said `limited=0 failed=0`
([ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/)).

**The input had lost its left channel.** The driver hears the receiver's
tape output, Tape 1 out on an old Onkyo A-8170, whose jack is a known
intermittent contact. A stereo meter on the same card,
`arecord -c 2 -V stereo`, read `+00%|76%`: the left side silent and the
right side carrying the record. After the plug was reseated it read
`29%|26%`.

**The driver listens to one channel, and it was the silent one.** It asks
`arecord` for one channel (`-c 1`) from `plughw:CODEC`, so that ALSA's plug
layer converts the card's stereo to mono. What was seen is consistent with
that conversion taking the first channel, the left, and dropping the right,
rather than mixing the two: with the right channel at 76% the driver heard
silence. That is behavior observed on this card and this ALSA, and is
written down as such; no ALSA document has been checked for it.

**The console showed nothing wrong.** Its "Song detector" pill was green
throughout. That is what
[ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/)
says it means: the sound card is heard, not that the turntable is on, and
not that both channels arrive. A person looking at the console saw a green
pill, a dim stand and music playing, and nothing to say which part was
wrong.

## Options considered

### What the driver listens to

**One channel, as now.** Nothing changes, and a fault in one channel of a
cable that is known to be intermittent stops the stand whenever it is the
first channel, and is unseen whenever it is the second.

**Both channels, mixed to one, and nothing said.** `-c 2`, and the two added
together before anything is measured. The stand keeps working with a channel
gone, about 6 dB quieter, which is well above -55 dBFS for music. The fault
is hidden: the record sounds one-sided in the room and nothing in EdS says
so, and the next time it is noticed is when the other side goes too.

**Both channels, each measured, and the louder one decides.** The stand
keeps working with a channel gone, and the driver knows which one and can
say so. It costs a stereo capture, 64 KB a second from the card instead of 32, and a
second level per tenth of a second.

### Where a silent channel is said

**A richer status.** `online`, `offline`, and a third word such as
`degraded`. One topic, already on the console. But a one-sided input is not
offline, and the status topic is not only for people: nowplayd's arbiter
reads a source's status, and the broker sends its last will there. A third
word would have to be taught to every reader of presence, and a reader that
does not know it would take it for something else.

**In the source's state.** A field on `nowplaying/source/turntable/state`.
The state is the contract every source shares
([ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/)),
and a channel is not something any other player has. A change of channel
would also be a change of state for nowplayd to compare and pass on.

**A retained topic of its own, beside the status.** One more topic and one
more grant on each side of it. It says one thing, and only those who ask for
it hear it.

**An alert beyond the console**: a notification, or a dashboard panel with a
threshold. Not now: the console and the event in the log store are where
this would be looked for, and a phone being told about a cable is more than
the fault deserves.

## Decision

**The turntable's driver captures both channels and measures each. Music is
decided by the louder channel, so one dead channel no longer stops the
stand. A channel that stays silent while the other carries music is
published on a retained topic of its own, logged once, and shown on the
console.**

- **Both channels are captured** (`arecord -c 2`, interleaved, through the
  same `plughw` device), and each tenth of a second is measured once per
  channel with the same `Level`.
- **A block is music when the louder channel is at or above the silence
  level** (`NP_TURNTABLE_SILENCE_DB`, -55 dBFS). Everything ADR-0026
  decides from silence, the gap and the stop, is decided from that. Its
  timings do not change.
- **The clip sent to the recognizer is the two channels mixed**, or, while
  one channel is reported silent, the other alone, so that a dead channel
  does not halve what the recognizer is given.
- **A channel is silent** when it has been below the silence level for the
  stop time (`NP_TURNTABLE_STOP_AFTER`, 20 s by default) while the other has
  carried music. **It has returned** when both have carried music for the
  same time. The same twenty seconds each way is what keeps a contact that
  crackles from saying so twice a second. While neither channel carries
  music, between sides or with the record lifted, nothing can be told about
  either, and the word stands as it was.
- **It is published, retained, on
  `<prefix>/nowplaying/source/turntable/input`**, beside the source's
  `status`, when the word changes and at no other time:

  ```json
  {"v":1,"silent":"left","left_db":-90.3,"right_db":-24.1,"changed_at":"2026-10-26T01:34:12Z"}
  ```

  `silent` is `left`, `right` or `none`; the levels are each channel's over
  the last second when the word changed, for whoever reads it in
  `mosquitto_sub`; `changed_at` is the driver's clock, as on a state. Field
  names follow the contract's (`snake_case`, `v` first). The topic is
  published `none` each time a capture starts, so a stored word from before
  a restart does not outlive it.
- **The topic is the turntable's only.** Other drivers have no input to
  measure, and publish nothing there.
- **It is logged once each way**, as `turntable.recognizer_failed` is once
  for a run of failures: a warning with the event `turntable.channel_silent`
  and which channel when the word becomes `left` or `right`, and an info
  line with `turntable.channel_back` when it returns to `none`. Each is
  published to the agent's log topic like the agent's other events
  ([ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/)).
- **The console's "Song detector" pill goes amber**, with "left channel
  silent" or "right channel silent" beside it, while the topic says so and
  the driver's presence is green. A driver that is gray or red is shown as
  ADR-0034 has it, whatever its input last said: a stored word about a
  channel is no more believed than a stored `online`.
- **nowplayd does not read it.** Its subscription, `nowplaying/source/+/+`,
  already delivers the topic, and the hub ignores a source topic that is not
  `state`, `status` or `art`; the arbiter decides nothing from a channel.
- **The console still only looks**
  ([ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/)). It
  is granted one more subscription and nothing more to publish.

## Consequences

- **A record keeps the stand lit with one channel gone**, and is named as
  before: the recognizer hears the live channel.
- **A fault the room can hear is now one a person can see**, on the console
  within twenty seconds of the music, and in the log store as one line for
  each time it happens.
- **Two grants, in the tenant's Terraform and in the backend's copy of the
  accounts** (`images/backend/files/accounts.conf`): `np-relay` publishes
  `nowplaying/source/turntable/input`, and `console` subscribes to
  `nowplaying/source/+/input`, added to `view.Topics` as written. The
  console's account is not granted `nowplaying/source/+/+`; it is granted
  each kind of source topic by name, and the broker refuses a subscription
  it was not granted without saying so. Applying them is a change of its
  own, before the driver and the console that use them are deployed.
- **The capture interface changes from mono to stereo.** The driver's tests
  that feed it audio feed two channels, and gain cases for each channel
  going silent and coming back, and for both going silent at the end of a
  side.
- **A record with a channel truly silent for twenty seconds** (a passage
  hard to one side, or a mono record played through a cartridge wired to
  one channel) is shown amber until both channels carry music again. That
  is said, not acted on: the stand is not dimmed for it.
- **A stereo capture is twice the bytes from the card**, 64 KB a second
  instead of 32. On the agent Pi that is nothing.
- **A dead channel is still a dead channel.** EdS says which one; the
  contact is still cleaned or the cable replaced by a person.
- **What the plug layer does with `-c 1` stops mattering here**: the driver
  no longer asks it to choose.

## What would settle it

Not yet shown:

- [ ] The driver's tests, with stereo audio: a side with the left channel
  silent throughout is followed to the end and named; the word becomes
  `left` after twenty seconds and `none` after twenty seconds of both; a
  side's end with one channel silent leaves the word as it was.
- [ ] On the agent Pi, `arecord -c 2` through `plughw:CODEC` delivers both
  channels interleaved, left first, as the stereo meter shows.
- [ ] A side played with one side of the tape output unplugged: the stand
  stays lit, the track is named, `input` says which channel within about
  twenty seconds, and one `turntable.channel_silent` reaches the log store.
- [ ] The plug put back: `input` says `none`, one `turntable.channel_back`
  is logged.
- [ ] The console's "Song detector" pill amber with the channel named while
  that is so, green after, and gray, not amber, while the driver is not
  heard.

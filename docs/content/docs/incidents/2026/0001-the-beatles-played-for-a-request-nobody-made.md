---
title: "INC-0001: The Beatles Played for a Request Nobody Made"
weight: 1
---

# INC-0001: The Beatles Played for a Request Nobody Made

| | |
|---|---|
| **Date** | 2026-10-24 |
| **Systems** | `respeaker-01`, `eds-scribe` and `eds-voiced` on the services workload, nowplayd, the relay's iTunes driver |
| **Severity** | An album nobody asked for replaced what was asked for. Recovery needed no more than asking again |
| **Status** | Open · {{< inc-status "Hardening" >}} |
| **Times** | UTC |

---

## Summary

The first request sent from the reSpeaker played *Abbey Road*. Its owner had
asked for Aerosmith's first album. scribe, given five seconds it could not
make out, wrote back the prompt it had been given to spell names by - "Play
Abbey Road by the Beatles." - and voiced, which acted on any words scribe
returned, read that as a request and sent `play_album`.

The prompt is a list of names now, which is not a request, and voiced acts
only on words scribe says it is sure of.

## Impact

"Come Together" played in place of what was asked for, at 01:41:50, until
the owner noticed. Nothing else was affected. Any recording scribe could not
make out could have done the same since voiced went on the workload
(CHG-0034), 01:10 the same night.

## Detection

The owner heard the opening of "Come Together" and took it for Aerosmith's
cover, from their greatest hits: the right artist, the wrong album, picked by
mistake. Hence "it worked -- kind of." Only as the song played on was it
plain that it was the Beatles' own, from *Abbey Road*, and that nothing of
Aerosmith's had been played at all. The device's console and voiced's log
both showed the words, `"understood":true`, and the album: nothing anywhere
said it was wrong, because nothing could tell.

## Timeline

| Time | Event |
|---|---|
| 01:41:41 | Enter on the reSpeaker's console, from a script: five seconds heard. The owner, who could not see when it listened, spoke at about that time. |
| 01:41:46 | The recording sent. |
| 01:41:50.4 | voiced: `asked for an album`, text "Play Abbey Road by the Beatles.", written in 3.4 s. |
| 01:41:50.7 | The relay: "Come Together", *Abbey Road*, playing. |
| 01:42:08 | A second window, five seconds later on: "3D", answered "not a request". |
| 01:43 | The prompt recognized in the words, word for word. |
| 01:49:27 | CHG-0035 shipped: a prompt of names, and words scribe is not sure of not acted on. |
| 01:51:41 | The same test: two windows, written as other people's talk in the room; one not heard clearly, one not a request. Nothing played. |
| 01:54:30 | The owner's "Play Aerosmith's first album", said in the window, played *Aerosmith*. |

## Symptoms

- The device's console: `heard: {"text":"Play Abbey Road by the Beatles.","understood":true,...}`.
- voiced's log: the same words, asked for.
- The music.

## Investigation

The words were exactly the second sentence of scribe's prompt
(`services/scribe/src/eds_scribe/transcribe.py`, `PROMPT`), capitals and
full stop included. Whisper is known to write its prompt, or text like it,
for audio with no speech it can make out. Whether the owner's words were
outside the window or too quiet is not known: the recording was not kept.

## Root cause

Two faults together:

1. **The prompt was a request.** `PROMPT` was "Play Van Halen's first album.
   Play Abbey Road by the Beatles.", chosen to make names come out right
   (`1badd8a`).
2. **voiced acted on any words.** It had no way to know whether scribe was
   sure of them: scribe returned the text alone.

## Recovery

None was needed beyond asking again. The fix (CHG-0035) was shipped eight
minutes after the incident was understood.

## Contributing factors

- **The device cannot be seen listening.** In the voice kit the XIAO and its
  LED are under the reSpeaker board, and the reSpeaker's own buttons are the
  XMOS chip's; requests were started from a script on a count of seconds.
- **The room had other voices in it**: a video or a call, heard in two
  later windows as clear speech.
- **The test that would have caught it was not written**: silence and noise
  through scribe were only tried after.
- **The wrong album sounded nearly right.** Aerosmith recorded "Come
  Together" too, so its first bars passed for a near miss, Aerosmith on the
  wrong album, rather than a request nobody made.

## Corrective actions

| # | Action | Where | Status |
|---|---|---|---|
| 1 | A prompt that cannot be a request: names only, and a test that says so | `services/scribe`, `e9ed753` | {{< action-status "Done" >}} |
| 2 | scribe says whether its words are worth acting on, by Whisper's own thresholds (no speech above 0.6, mean log probability below -1.0) | `services/scribe`, `e9ed753` | {{< action-status "Done" >}} |
| 3 | voiced acts only on confident words, and treats a scribe that does not say as not sure | `internal/voice`, `e9ed753` | {{< action-status "Done" >}} |

## Preventive actions

| # | Action | Where | Status |
|---|---|---|---|
| 4 | Silence, noise and hum through the real model are part of trying a scribe change: none may come back confident | `services/scribe/README.md` | {{< action-status "Open" >}} |

## Follow-ups

| # | Follow-up | Where | Status |
|---|---|---|---|
| 5 | A light the room can see while the device listens: the reSpeaker's own LED, or a wake word, which needs no watching | `firmware/ears`, the wake word ([CHG-0036](/docs/changes/2026/0036-the-respeaker-wakes-to-hey-jarvis/)) | {{< action-status "Done" >}} |

## Lessons learned

- **A model's prompt is input it can give back.** Anything put in a prompt
  must be harmless if it comes out as the answer.
- **Words from speech-to-text are a guess, with a confidence.** A system that
  acts on them should read the confidence, as one that acts on a sensor reads
  its range.

## Related

- [CHG-0034](/docs/changes/2026/0034-voiced-and-scribe-on-the-workload/), which put the prompt on the workload.
- [CHG-0035](/docs/changes/2026/0035-only-words-scribe-is-sure-of/), which corrected it.
- [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/).

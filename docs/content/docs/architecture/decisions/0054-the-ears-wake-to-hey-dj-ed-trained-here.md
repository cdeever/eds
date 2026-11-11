---
title: "ADR-0054: The Ears Wake to \"Hey DJ Ed\", Trained Here"
weight: 54
---

# ADR-0054: The Ears Wake to "Hey DJ Ed", a Model EdS Trains Itself on the Owner's Mac

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the trained model has woken the ears for a week of ordinary listening; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-11-11 |
| **Scope** | The ears' wake word: what it is, how its model is made, and what of the making is kept. Not what follows the wake word, which is [ADR-0042](/docs/architecture/decisions/0042-a-request-is-heard-on-the-workload/)'s. |
| **Extends** | [ADR-0043](/docs/architecture/decisions/0043-the-respeaker-runs-firmware-of-edss-own/), whose "a stock model, then 'Hey Ed'" this settles |
| **Related** | [ADR-0044](/docs/architecture/decisions/0044-the-music-is-turned-down-while-a-request-is-heard/), [CHG-0036](/docs/changes/2026/0036-the-respeaker-wakes-to-hey-jarvis/) |

---

## Context

The ears wake to "Hey Jarvis", ESPHome's microWakeWord model, as a stand-in
(CHG-0036). ADR-0043 promised "Hey Ed" after it. No published model says
either, so one has to be trained.

"Hey Ed" is two syllables. microWakeWord does best with three or more, and
"Hey Ed" sounds like "ahead", "head", "said" and "hey dad". In this room
that matters more than in most: music with words plays nearly all the
time, and a false wake turns it down (ADR-0044) and sends what the room
said to the workload, which the docs promise happens only when asked.

## Options considered

**The phrase.** "Hey Ed", as promised; or **"Hey DJ Ed"**, four
syllables, the owner's choice on 2026-11-11. Songs say "Hey DJ" and "Mister
DJ", so its risk is lyrics, which the training can be shown.

**Who makes the model.** Espressif makes WakeNet models to order, for a
fee, for its own ESP-SR, which the ears do not run. openWakeWord runs on a
server, and would need the room streamed to the workload all day. **Training
a microWakeWord model ourselves** keeps the firmware's `app/wake` as it is and
the room on the device.

**Where it is trained.** Google Colab, as microWakeWord's notebook offers:
slow, and the owner's own recordings of the phrase would later go to Google.
builder: Linux, but for Deevnet's work, and not always reachable. **The
owner's Mac**, an M3 Pro, with its data on an external disk.

## Decision

**The ears wake to "Hey DJ Ed". Its model is trained by `tools/wakeword`
on the owner's Mac, from microWakeWord's own training at a pinned commit,
and only the trained model and the tool are kept in the repository.**

- **The tool is the notebook made repeatable**: a Makefile and a few
  scripts, `make venv work listen generate fetch features config train
  manifest install`, with microWakeWord, its generator and their forks
  pinned to commits.
- **The phrase is said by Piper** in hundreds of voices, spelled the ways a
  person says it ("hey dee jay ed", "hey deejayed"; `phrases.txt`), and
  listened to before thousands are made (`make listen`).
- **What must not wake it is trained in by name**: near misses said the
  same way (`near-misses.txt`: "hey DJ", "hey mister DJ", "hey Ed",
  "ahead"...), beside microWakeWord's ready-made speech, parties and noise.
- **The room's own music is trained in when given** (`make music
  MUSIC=...`), and false wakes an hour are measured on songs of it the
  model never trained on. The songs stay in the work folder.
- **The cutoff is chosen by false wakes an hour**: the one that misses
  fewest phrases while waking falsely at most `MAX_FAPH` an hour: 0.05, about one a day.
- **Kept in the repository**: the tool, its lists and settings, and the
  trained `hey_dj_ed.tflite` with its manifest, beside `hey_jarvis`.
  **Never kept**: the work folder, which holds the generated clips, the
  downloaded sets, the room's music and, later, the owner's own voice.
- **The model is for personal use.** microWakeWord's ready-made sets are
  under mixed licenses, and its author says models trained on them are for
  non-commercial use; the models README says so beside it. The repository
  is public; the owner chose to commit it marked so.

The firmware keeps "Hey Jarvis" until a model is switched to, by a change
record (the first, CHG-0049): the model embedded, Kconfig's cutoff from
the manifest, scribe's prompt and voiced's grammar told "DJ Ed".

## Consequences

- **A model can be made again**, from the same pins and lists, by anyone
  with the Mac's disk space; how good it is depends on the settings, which
  are expected to be tried several times.
- **The work folder is large** (about 35 GB, most of it the ready-made negatives unpacked) and **lives off the
  repository**, on the external disk the owner named.
- **The Mac's Python** is Homebrew's 3.12 in a venv in `tools/wakeword`;
  the external disk is exFAT, which holds no venv and collects macOS's `._`
  files, which the tool removes before each step reads.
- **Recordings of the owner** would improve it most, and are left for after
  the first model: they are a person's voice, and are kept like the music.

## What would settle it

- [x] A model trained, with its misses and false wakes an hour written here:
      the first, 2026-11-11, 10,000 steps on the M3 Pro, without the room's
      music. Tested quantized and streaming on clips it never trained on,
      with about five hours of microWakeWord's dinner-party audio: at cutoff
      0.96, 12% of phrases missed and no false wake; at 0.86, 7% and 0.19 an
      hour; at 0.53, 2.8% and 0.38. 0.96 was kept (`EA_WAKE_CUTOFF` 245).
      The misses are synthetic voices' and the hours too few to show one a
      day; the room is the test.
- [ ] It loads on the ears: its arena fits, as the boot log says.
- [ ] It wakes for the owner, across the room, nine times in ten.
- [ ] A week of the room's ordinary listening with no more than one false
      wake a day, counted from the ears' events.

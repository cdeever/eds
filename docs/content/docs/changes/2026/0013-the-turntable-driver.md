---
title: "CHG-0013: The Turntable Driver"
weight: 13
---

# CHG-0013: The Turntable Driver

| | |
|---|---|
| **Date** | 2026-10-11, run on 2026-10-12 UTC |
| **Change type** | Deployment |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-12 |
| **Systems** | the tenant (one broker account), the agent Pi (its agent, its unit, a recognizer installed, two lines of its settings) |
| **How** | `make apply` in the tenant; `make -C deploy/relay push`, `turntable`, `drivers` |
| **Risk** | Medium. Most likely to go wrong: the agent's unit, which changes how it is confined, not starting, which would stop iTunes being followed too |
| **Decisions** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/) |
| **Related** | [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/), which made the account; [CHG-0012](/docs/changes/2026/0012-the-cover-screen/), in progress on the same Pi |

---

## Summary

The agent Pi gains a third driver, which listens to the amplifier's tape
output through a USB audio interface and reports the record that is playing.

After this change a record on the platter is a source on the now-playing bus
beside iTunes, and the stand follows it.

## Goal

- The agent runs `itunes` and `turntable`, and both are online.
- A record playing is reported as `playing` with its artist and title within
  about half a minute, and `stopped` about twenty seconds after the needle
  lifts.
- nowplayd gives it an album and a cover, and the stand lights from it.
- iTunes is followed as before.

## Scope

**In scope:** three topics added to the `np-relay` account's `publish`; the
agent, its unit and its tools on the Pi; the recognizer and its Python
environment on the Pi; `NP_TURNTABLE_DEVICE` and `NP_DRIVERS` in the Pi's
settings.

**Out of scope:** nowplayd, lightd, the stand, the console's subscriptions
(all already cover any source). A new image is built from the same files but
no card is flashed. The Roland UA-4FX and its cable are already in place.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The account is replaced, not updated, and its password changes | the tenant | `make plan` is read first; a replacement stops this change until `make card` is planned for |
| The unit's new confinement keeps the agent from starting | the Pi | `push` reports whether the agent is active; iTunes is checked before the turntable is turned on |
| The recognizer will not install on the Pi | the Pi | wheels only, pinned to what the bench ran; the install checks the import; the agent is not restarted into the new driver until `drivers` |
| The turntable is reported and never granted | the bus | the grant is applied before the driver is turned on; the source's status on the console |
| The silence level is wrong for this input | the room | measured with the needle up before the driver is turned on; `NP_TURNTABLE_SILENCE_DB` |
| A stale `playing` is left retained if the driver is turned off | the bus | its status is a last will; nowplayd drops a source that is offline |

## Procedure

```bash
# 1. The grant. Expect one in-place update to the np-relay account.
cd infra/deevnet-tenant-eds && make plan && make apply

# 2. The agent, its unit and the recogniser. iTunes only, still.
make -C deploy/relay push
make -C deploy/relay status

# 3. Which input, and then the driver.
make -C deploy/relay turntable DEVICE=plughw:UA4FX
make -C deploy/relay drivers DRIVERS=itunes,turntable
```

## Verification

- `make plan` shows the account updated in place, with three topics added.
- After `push`: the agent is active, and iTunes is online on the console.
- After `drivers`: `make -C deploy/relay status` shows the turntable's input,
  its recognizer and the endpoint all `ok`.
- With a record playing, `eds/nowplaying/source/turntable/state` is `playing`
  with an artist and a title, and `eds/nowplaying/current` has an album and
  `art`.
- The stand changes to the record's colors.
- With the needle lifted, the source is `stopped` within half a minute.
- With the interface unplugged, the source is `offline` and iTunes carries on.

## Undo

```bash
make -C deploy/relay drivers DRIVERS=itunes
```

That stops the listening and every request to the recognizer. The grant, the
recognizer and the unit can stay as they are; the unit is put back by pushing
from the commit before this one.

## Outcome

| When (UTC) | What happened |
|---|---|
| 2026-10-12 03:35 | `make plan` showed one change: the `np-relay` account updated in place, with the three turntable topics added to `publish`. Nothing replaced. **Not applied.** |
| 03:36 | `make -C deploy/relay push` installed the agent from `45a90e3`, the new unit, and the recognizer (`shazamio==0.8.1`, wheels only). The agent came back active. |
| 03:37 | `eds-agent-status` passed. iTunes online under the new unit, and reporting its paused track. `npagent` is in the `audio` group. |
| 03:38 | The installed agent run by hand as `npagent`, under the unit's confinement (`systemd-run` with the same properties), in `watch` mode, publishing nothing: it opened the UA-4FX and named the record playing, *You Can't Hurry Love*, twenty-four seconds after starting. |
| 03:38 | `make turntable DEVICE=plughw:UA4FX`: the line is in the Pi's settings and the agent is active, still with `NP_DRIVERS=itunes`. |

| about 03:40 | `make apply`, by the owner. The account reads back with the three turntable topics granted, and the same password. |
| 03:41 | `make drivers DRIVERS=itunes,turntable`, by the owner. The agent started both drivers, and fourteen seconds later reported `playing on the turntable: Phil Collins - It Don't Matter to Me`. |
| 03:42 | The same target run a second time, from the session making this change, which had not been told it was done. It removed the `NP_DRIVERS` line and did not put it back; the agent restarted with the unit's default, `itunes,spotify`. **Spotify was polled for 38 seconds** and the turntable was not listened to. |
| 03:43 | The line put back by hand. The agent started `itunes,turntable`; the track was named again fourteen seconds later. Spotify reads as offline. |
| 03:44 | The target mended (below) and run twice running: `NP_DRIVERS=itunes,turntable` both times. |
| 03:45 | On the console: `current` is the turntable's track, playing, with an album and a cover nowplayd found in the iTunes catalog, and `lit_cover` is that cover, so lightd has sent a scene from it. **`lp-stand-01` was offline**, so the stand itself was not seen to change. |
| 03:45 | The stand back online. It reported the scene for the record's cover as applied: `chase`, four colors. |
| 03:45:33 | The needle lifted, by the owner. The turntable reported `stopped`, about twenty seconds after. |
| 03:45 | `current` then passed to iTunes, which was paused on another album, and the stand was lit from that album's cover. See the follow-ups. |
| 03:48:05 | The interface unplugged, by the owner. The turntable went `offline`, said once; iTunes stayed online and the agent stayed up. `eds-agent-status` failed on the missing input, by name. |
| 03:49:08 | Plugged back in. The driver found it again by itself and went `online`, with no restart. |
| 03:49 | Two faults seen in that, and mended: `eds-agent-status` went on failing after the input was back, and the reason logged for losing it was cut short. Pushed again; the status check passes with both drivers. |

Every goal was seen: both drivers online, a record named within half a
minute and `stopped` twenty seconds after the needle lifted, an album and a
cover from nowplayd, the stand lit from it, and iTunes followed throughout.

### Departures from the plan

- The grant was planned and not applied before the Pi was changed: the
  session making this change was not permitted to run `terraform apply`. The
  steps that do not depend on the grant went ahead, and the driver stays off
  until it is applied, since a publish to a topic not granted is dropped in
  silence.
- **The `drivers` target lost the setting when run twice.** It deleted the
  line and appended the new one in one `sed` script, `d` and then `$a` on the
  last line. Its first run leaves the line last; on the second, the line
  deleted is the last line, and the append never happens. It has been in the
  target since CHG-0011 and nobody had run it twice. The two steps are now two
  commands, in `drivers` and in the new `turntable` target, which had copied
  it.
- **Two faults found by pulling the cable.** `eds-agent-status` looked for the
  last line about the turntable's troubles and took no notice of it coming
  back online, so it failed for ever after one lost input. And the driver
  read what `arecord` said on its way out before `arecord` had finished
  saying it, so the reason logged ended at `pcm_read:2221:`. Both are mended
  and pushed. The second has not been seen with a cable pulled since: the fix
  is to wait for `arecord` to exit before reading what it said.
- The confinement was tried with `systemd-run` before the driver is turned
  on, which the plan did not have. It is the one part that could not be
  tested on a desk.

## Follow-ups

- [ ] **Lifting the needle lights the stand from whatever iTunes has
  paused.** A paused source outranks a stopped one
  ([ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/)),
  so when the record stops, a track paused in iTunes hours ago becomes
  current and the room is repainted for it. Nothing here is broken; whether a
  record ending should do that is a question for the arbiter, and a decision.

- [ ] The evidence [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/)
  waits on.
- [ ] The image built for the Pi starts `itunes,spotify` unless a card says
  otherwise; a card for this Pi is written with
  `DRIVERS=itunes,turntable TURNTABLE_DEVICE=plughw:UA4FX`.
- [ ] The owner's Discogs collection as a catalog in nowplayd, so the cover
  is the pressing on the platter: a decision of its own.
- [ ] A local index of the owner's records, so the recognizer is not needed:
  a decision of its own.

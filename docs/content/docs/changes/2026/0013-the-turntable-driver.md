---
title: "CHG-0013: The Turntable Driver"
weight: 13
---

# CHG-0013: The Turntable Driver

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "planned" "Planned" >}} |
| **Systems** | the tenant (one broker account), the agent Pi (its agent, its unit, a recogniser installed, two lines of its settings) |
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
agent, its unit and its tools on the Pi; the recogniser and its Python
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
| The recogniser will not install on the Pi | the Pi | wheels only, pinned to what the bench ran; the install checks the import; the agent is not restarted into the new driver until `drivers` |
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
  its recogniser and the endpoint all `ok`.
- With a record playing, `eds/nowplaying/source/turntable/state` is `playing`
  with an artist and a title, and `eds/nowplaying/current` has an album and
  `art`.
- The stand changes to the record's colours.
- With the needle lifted, the source is `stopped` within half a minute.
- With the interface unplugged, the source is `offline` and iTunes carries on.

## Undo

```bash
make -C deploy/relay drivers DRIVERS=itunes
```

That stops the listening and every request to the recogniser. The grant, the
recogniser and the unit can stay as they are; the unit is put back by pushing
from the commit before this one.

## Outcome

Not run yet.

### Departures from the plan

## Follow-ups

- [ ] The evidence [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/)
  waits on.
- [ ] The image built for the Pi starts `itunes,spotify` unless a card says
  otherwise; a card for this Pi is written with
  `DRIVERS=itunes,turntable TURNTABLE_DEVICE=plughw:UA4FX`.
- [ ] The owner's Discogs collection as a catalogue in nowplayd, so the cover
  is the pressing on the platter: a decision of its own.
- [ ] A local index of the owner's records, so the recogniser is not needed:
  a decision of its own.

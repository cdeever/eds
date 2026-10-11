---
title: "ADR-0017: An Agent Pi Is a Registered Device"
weight: 17
---

# ADR-0017: An Agent Pi Is a Registered Device, and Reports Its Events Like One

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-11 |
| **Scope** | How the now-playing agent's key events reach the tenant's log store, and what the Pi it runs on is to the tenant. |
| **Extends** | [ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/), which gave devices a way to report and said a program that is not a device "needs another route" |
| **Related** | [ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/), [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/) |

---

## Context

The stand reports its key events to the tenant's log store, and they are read
on a dashboard. The agent Pi reported nothing: what it saw playing, and
whether its drivers were working, was in its own journal, reached by SSH
through the Builder.

The agent's broker account had been declared with no device, on the reasoning
that the Pi is the substrate's host and not something the tenant registered.
An account with no device cannot publish under `log/`.

That reasoning was written while the Pi ran an image from the substrate's
factory. Since
[ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/)
it runs EdS's own.

## Options considered

**Write to the log store directly, over HTTPS**, as a workload does. Checked
from the Pi: the log store is not reachable from the IoT segment. Making it
so would be a change to the substrate's network for one tenant's convenience,
and would put an ingest token on a machine with a leg on a home network.

**Register the Pi as a device and report over MQTT**, as the stand does. The
substrate's trust class `iot` is for devices whose firmware their owner
controls. The Pi's image is EdS's. The route already exists, is already
confined to one topic, and needs nothing new on the network.

**Leave it in the journal.** Costs nothing and answers "what played this
afternoon?" only for someone with SSH.

## Decision

The agent Pi is registered as a device of the tenant, `np-relay`, in trust
class `iot`. Its one broker account belongs to that device and may publish
`log/np-relay`.

The agent reports a few events there, in the same shape as the stand's:

| Event | When |
|---|---|
| `agent.started`, `agent.stopping` | the agent itself |
| `driver.online`, `driver.offline` | a driver gains or loses its player, once per change |
| `driver.not_set_up`, `driver.stopped` | a driver is missing its pairing or authorization, or has given up |
| `track.playing`, `track.paused`, `track.stopped` | a driver reports a change, with the source, artist, album and title |
| `spotify.refused`, `spotify.authorization_ending` | Spotify's own refusals and the six-month warning |

A line becomes an event by being logged with an `event` attribute. The
journal still has every line; the store gets these.

## Consequences

- What played, on which player, and when, is on the dashboard beside what the
  stand did about it.
- **Track titles leave the Pi for the log store.** It is the tenant's own
  store, kept as long as the substrate keeps device logs. It is still a
  listening history, and it is now written down somewhere.
- Events never hold a driver up. They wait in a queue of 32; past that they
  are dropped and counted, and the next to arrive says how many were lost.
- The agent makes a third connection to the broker, for events alone, so
  that a line about a driver's connection does not depend on it.
- A device's name is per card, not per image: it is written to the card with
  the rest of its settings. An agent that is told no name reports nothing,
  which is how it runs on a desk.
- "Device" now covers two things in this tenant: a microcontroller and a Pi.
  The dashboard's device filter is how they are told apart.
- `nowplayd` and `lightd` on the workload still report only to their
  journals. They are workloads, and their route is the direct one this
  record turned down for the Pi.

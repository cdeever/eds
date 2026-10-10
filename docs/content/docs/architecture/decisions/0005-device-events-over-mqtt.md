---
title: "ADR-0005: Device Events Over MQTT"
weight: 5
---

# ADR-0005: A Device Reports Key Events Over MQTT, Not Its Serial Log

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | How an EdS device says what it is doing once the USB cable is gone, and how that is read. |
| **Related** | [CHG-0001](/docs/changes/2026/0001-lp-stand-joins-its-tenant/), Deevnet ADR-0027 and CHG-0021 ([deevnet-docs](https://deevnet.github.io/deevnet-docs/)), [MQTT Topics](/docs/reference/mqtt-topics/) |

---

## Context

A stand on a shelf has no console. It fails soft by design, which makes its
failures quiet: one that keeps showing the last good scene looks, from the
room, exactly like one that was sent nothing.

The substrate offers each tenant a log store. A device may publish to
`<tenant>/log/<device>`, the one topic under `log/` its account can write,
and a bridge carries what arrives into the tenant's device log partition. The
device needs no log store credential and no route to the store. The Ma Bell
gateway already reports this way.

## Options considered

**Forward the serial log.** Everything is captured. But it is a stream, most
of it is driver chatter, and it is free text that cannot be graphed.

**Publish a few structured events.** One JSON object per thing worth
knowing. Less is captured, deliberately, and each numeric field can be
graphed, which matters because the platform has no metrics service.

**Ship from the device straight to the store.** Needs a token on every
device and a route the IoT network does not have.

## Decision

A device publishes its **key events** as JSON to `eds/log/<id>` at QoS 1, not
retained. For the stand: `system.boot`, `wifi.connected` /
`wifi.disconnected`, `mqtt.connected` / `mqtt.disconnected`, `scene.applied`,
`scene.rejected`, and a `stand.health` report every five minutes carrying
signal and free heap.

## Consequences

- Events are a handful a day, not a stream. Adding one is a deliberate act.
- **The formatter is pure C and host-tested**, because a line that is not
  valid JSON is filed as text with none of its fields and the dashboard goes
  quiet rather than red.
- Recording an event never blocks on the network. Events wait in a queue of
  sixteen while the broker is away; when it overflows the oldest go and the
  next event carries a `dropped` count.
- A loss is recorded once, when it happens, not once per retry.
- The device has no clock: the store dates each event on arrival and
  `uptime_ms` orders the ones that queued.
- Events are read with `make logs` or on a Grafana dashboard declared in the
  tenant's Terraform, because a dashboard built by clicking is not kept.
- Only a registered device may publish under `log/`. A program that is not
  one — a service on the workload, an agent on a PC — cannot report this way
  and needs another route.

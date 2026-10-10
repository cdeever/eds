---
title: "ADR-0003: The Broker Is the Substrate's"
weight: 3
---

# ADR-0003: The MQTT Broker Is the Substrate's, Not a Tenant VM

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-09-07 |
| **Scope** | Where EdS's MQTT broker runs, and so how devices and services meet. |
| **Backfilled** | Decided when the tenant was first written and argued in `infra/deevnet-tenant-eds/README.md`; written up here on 2026-10-10. |
| **Related** | Deevnet ADR-0001, ADR-0003 and ADR-0012 ([deevnet-docs](https://deevnet.github.io/deevnet-docs/docs/architecture/decisions/)) |

---

## Context

EdS's services run on a workload in its own tenant on the Deevnet substrate.
Its devices are ESP32s on the substrate's IoT network. Both need a broker.

MQTT clients always initiate: the stand dials the broker, never the reverse.
So whichever network holds the broker must accept inbound connections. The
tenant fabric has no inbound path by design: the substrate delivers tenants
egress only, and its core router never learns tenant address space.

## Options considered

**A broker on the EdS workload.** EdS would own it outright. But devices
could not reach it without the substrate inventing an inbound path into a
tenant, which is a change to the substrate's architecture, not a setting.

**The substrate's broker.** The substrate runs one on its IoT Backend
segment, which exists to accept exactly this, and issues each tenant accounts
confined to its own topic prefix.

## Decision

EdS uses the substrate's broker, `mqtt.mobile.deevnet.net:8883`. The stand
reaches it from the IoT network; services reach it outbound from the tenant.

## Consequences

- **Everything meets at the broker.** Nothing can dial the workload, so a
  device or an outside program that has something to tell an EdS service says
  it on a topic. This shapes every later design
  ([ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/)).
- Every topic is under `eds/`. Accounts are declared in the tenant's
  Terraform with topics relative to that prefix, and a publish or subscribe
  that was not granted fails silently.
- The connection is TLS only, verified against the Deevnet Root CA.
- The broker lives at the mobile site, and the mobile rack travels. When it
  is away, the lights stop. Accepted, because mobile is the only site that
  exists.
- Moving the broker into the tenant would need a new record here and a change
  to the substrate; it is not a configuration change.

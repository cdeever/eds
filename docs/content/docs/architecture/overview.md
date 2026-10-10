---
title: "The First Vertical"
weight: 1
---

# The First Vertical

```
cover image ──▶ lightd ──▶ palette ──▶ lightd ──▶ mqtt01 ──▶ LP stand
                (Go)       (Python)               (VLAN 35)   (VLAN 30)
```

A cover image arrives at `lightd` over HTTP. `lightd` asks `palette` for the
colours in it, maps those onto a scene the stand can render, and publishes that
scene **retained** to the stand's MQTT topic. The stand animates it locally.

## Why the broker is not in the tenant

`mqtt01` lives on the substrate's **IoT Backend** segment (VLAN 35), not on a
tenant VM, and the stand sits on the **IoT** segment (VLAN 30).

That placement is forced rather than preferred. MQTT clients always dial the
broker — the ESP32 initiates, never the reverse — so whichever segment holds
the broker must accept **inbound**. The tenant fabric has no inbound path by
design: ADR-0003 delivered egress only, and ADR-0001's central promise is that
the core router never learns tenant address space. IoT Backend is the segment
already defined to accept exactly this, with `mqtt01` named in the model as its
typical inhabitant.

Putting the broker on a tenant VM would mean inventing an inbound path to
tenant address space. That is an ADR, not a config change.

## Why the device never receives frames

The stand subscribes to a **scene descriptor** and animates it locally at its
own rate. It is never sent pixel data.

Streaming per-pixel frames over WiFi at any useful frame rate turns latency
spikes into visible stutter, and a broker outage would freeze an animation
mid-sweep. Rendering on the device means a stand whose network has gone away
keeps doing something sensible, and it keeps the payload small enough to read
by eye in `mosquitto_sub`.

## Why the scene topic is retained

Retention is load-bearing, not an optimisation.

A stand that reboots — power blip, reflash, WiFi drop — pulls the current scene
down on reconnect instead of sitting dark until the next record. Without it the
failure mode is a black stand in the middle of an album. Receiving the retained
scene moments after subscribing is the normal case, not an edge one.

There is an integration test for exactly this: it publishes, *then* connects a
subscriber, and requires the scene to arrive.

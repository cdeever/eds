---
title: "MQTT Topics"
weight: 3
---

# MQTT Topics

The topic prefix defaults to `eds` and must match on both sides — `lightd`'s
`LIGHTD_TOPIC_PREFIX` and the firmware's `LP_TOPIC_PREFIX`.

| Topic | Direction | Retained | |
|---|---|---|---|
| `eds/lightstand/<id>/scene` | lightd → device | **yes** | what to render |
| `eds/lightstand/<id>/status` | device → broker | yes (LWT) | `online` / `offline` |
| `eds/lightstand/<id>/state` | device → broker | yes | what it is rendering now |
| `eds/lightd/status` | lightd → broker | yes (LWT) | the publisher's presence |
| `eds/log/<id>` | device → broker | no | the stand's key events, for the log store |
| `eds/nowplaying/source/<id>/…` | a driver → nowplayd | yes | what one player is doing; see [Now Playing](../now-playing/) |
| `eds/nowplaying/current` | nowplayd → subscribers | yes | the one current track |
| `eds/nowplaying/current/art` | nowplayd → subscribers | yes | its cover, as image bytes; lightd lights the stand from it |

## The log topic

`eds/log/<id>` is not EdS's own layout. `<tenant>/log/<device>` is the one
topic under `log/` the substrate lets a device account publish, and its log
bridge carries what arrives there into the tenant's device log partition. The
stand sends one JSON object per event — boot, network, scenes, health — at
QoS 1 and **not retained**: a log line replayed to every new subscriber would
be an event that never happened again.

The events and their fields are in the
[firmware's README](https://github.com/cdeever/eds/tree/main/firmware/lp-stand#events).
Read them back with `make logs` in `firmware/lp-stand`, or on the **LP Stand**
dashboard in the tenant's Grafana organization.

## Why the two status topics are separate

`lightd` publishes its own presence separately from the stands' because **a dark
stand and a dead publisher look identical from the room**. Without the
distinction, the first diagnostic step is a guess.

Both use a last will, so the room learns a participant died rather than merely
went quiet.

## Quality of service

The stand subscribes to `scene` at **QoS 1**. A dropped scene means the stand
shows the previous record's colours until the next one — exactly the sort of
quiet wrongness that is hard to notice and annoying to debug.

## Watching the tree

```bash
mosquitto_sub -h <broker> -t 'eds/#' -v
```

Because gamma is applied on the device and not on the wire, what you see here
is the colour actually intended.

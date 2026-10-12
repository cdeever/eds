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
| `eds/console/status` | console → broker | yes (LWT) | the console's presence; the only thing it publishes |
| `eds/log/<id>` | device or agent Pi → broker | no | key events, for the log store: the stand's, the cover screen's, and the now-playing agent's (`np-relay`) |
| `eds/nowplaying/source/<id>/…` | a driver → nowplayd | yes | what one player is doing; see [Now Playing](../now-playing/) |
| `eds/nowplaying/current` | nowplayd → subscribers | yes | the one current track |
| `eds/nowplaying/current/art` | nowplayd → subscribers | yes | its cover, as image bytes; lightd lights the stand from it |
| `eds/nowplaying/current/art/small` | nowplayd → subscribers | yes | the same cover at 240 pixels, as a baseline JPEG; the cover screen shows it |
| `eds/nowplaying/command` | a controller → nowplayd | **never** | a button press, for whatever is current; see [Now Playing](../now-playing/#commands) |
| `eds/nowplaying/source/<id>/command` | nowplayd → a driver | **never** | the same press, passed to the current source |
| `eds/screen/<id>/status` | device → broker | yes (LWT) | the cover screen's presence |
| `eds/nowplaying/current/look` | nowplayd → lightd | yes | whether that cover's album has a look of its own, by name; see [Now Playing](../now-playing/#looks) |

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

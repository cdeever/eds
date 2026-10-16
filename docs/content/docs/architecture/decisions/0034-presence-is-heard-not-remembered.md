---
title: "ADR-0034: Presence Is Heard, Not Remembered"
weight: 34
---

# ADR-0034: Presence Is Heard, Not Remembered: Everything Says It Is There Every Thirty Seconds

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a stand unplugged across a broker restart is seen gray; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-16 |
| **Scope** | What `online` on a `…/status` topic means to whoever reads it: the console, and nowplayd for sources and beacons. |
| **Extends** | [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), whose consequence "a will needs a broker to send it" this removes. [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/): the console now shows a part as up only if it has heard from it lately. |
| **Related** | [ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/), the broker that keeps retained messages across restarts |

---

## Context

Every part of EdS says it is there the same way: on connecting, it publishes
`online`, retained, to its status topic, and leaves the broker a last will
of `offline`, retained. The broker sends the will when it sees the
connection die.

On 2026-10-14 the broker went away at about the time `lp-stand-01` was
unplugged. The broker never saw the stand's connection die, so it sent no
will. It keeps retained messages on disk, and every restart since,
including the whole mobile kit's, has brought back the stand's `online`. On
2026-10-16 the console still showed the stand green, two days after it last
said anything.

ADR-0023 accepted this for the beacon, where it is worse: a beacon left
`online` holds the turntable, silent, until it is next switched on and off.

EdS speaks MQTT 3.1.1 everywhere: paho.mqtt.golang in the services, and
ESP-MQTT without its MQTT 5 option in the firmware. A retained message in
3.1.1 carries no time. What a subscriber can tell is whether a message came
to it as a stored copy, sent because it subscribed, or live, because it was
just published: the retain flag on delivery says which.

## Options considered

**A time in the message.** `{"online":true,"at":…}`. Every device needs a
clock it can trust (SNTP), and every reader of the plain words `online` and
`offline` changes. It answers "when" and still needs a rule for "too long
ago".

**MQTT 5 message expiry.** The broker deletes a retained `online` nobody
has renewed. The cleanest on paper. It means a different Go library in
three services, MQTT 5 in three firmwares, and Deevnet IoTaaS's broker
agreeing to it; and an expired message tells nobody it expired, so readers
still need a timer.

**Not keeping presence across a broker restart.** Mosquitto persists
retained messages or doesn't, and the stand's `scene` must survive a
restart. It would also be Deevnet IoTaaS's broker setting, not EdS's.

**Say it again, and believe only what is heard.**

## Decision

**Whatever publishes a status topic republishes its word, retained, every
thirty seconds while it is connected. Whoever reads one believes an
`online` for ninety seconds after hearing it live; a stored `online` is
only a claim, and lapses in ninety seconds unless a live one follows.**

- **The words do not change.** `online` and `offline` on the same topics,
  retained, with the same wills. The repeat is the only thing added on the
  publishing side: the stand, the cover screen and the beacon in their
  event loops; lightd, nowplayd, the console and each of `npagent`'s
  drivers on a ticker. A driver repeats whichever word it last said.
- **`offline` counts at once**, stored or live. A will is still how a part
  that dies with the broker up is known to be gone, in a few seconds rather
  than ninety.
- **Ninety seconds is three repeats missed.** A part is not grayed by one
  lost message, and not believed for long after it is gone.
- **The console draws a claim gray.** Green is a part heard from in the
  last ninety seconds. Gray is a claim not yet heard, or a part not heard
  for ninety seconds. Red is a part that said `offline`. After a broker
  restart every live part is green within thirty seconds, because the
  repeat arrives live.
- **nowplayd keeps a claim, for ninety seconds.** On its own restart it
  must not drop a source that is playing, or dim the room mid-record,
  while it waits for the repeat. A source's claim that lapses is offline;
  a beacon's is unknown, which ADR-0023 already treats as "nothing
  changes".
- **The console's header separates the kinds of part**, on its own lines:
  services, sources, devices and power switches. A source's pill is its
  driver's presence, and the turntable's driver is labeled "Song detector",
  because it says that the sound card is being heard and not that the
  turntable is on. The source is still `turntable` everywhere else.

## Consequences

- **A part that is up shows gray for up to thirty seconds** after the
  console or nowplayd starts. That is the price of not showing a part that
  is down as green.
- **Old publishers look dead to new readers.** Firmware and services that
  do not repeat go gray after ninety seconds, and a beacon on old firmware
  lapses to unknown. The devices are flashed and the publishers deployed
  before the readers.
- **About one message every thirty seconds per part**, retained, to the
  broker. There are ten or so parts.
- **The broker being down is now visible.** Every pill goes gray while it
  is, rather than staying as it last was.
- **A dev broker needs repeating too.** `dev/feed.sh` republishes its
  status topics on its loop, or the dev console grays them.

## What would settle it

- The stand unplugged, the broker restarted, and the console showing it
  gray, never green, afterward.
- A record playing through a nowplayd restart without the room dimming.
- The turntable's beacon unplugged while the broker is down: after ninety
  seconds the turntable no longer holds the stand.

---
title: "ADR-0023: A Source That Is Switched On Holds the Stand"
weight: 23
---

# ADR-0023: A Source's Equipment Can Say It Is On, and a Source That Is On Holds the Stand

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the beacon has been timed on the switch it shares and a record has been held through its silences; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-12 |
| **Scope** | How EdS knows the turntable is switched on: a small device on the same switched outlet, whose connection to the broker is the whole of what it says. And what nowplayd does knowing it. |
| **Extends** | [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), whose rule now has one thing before it. [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/): the console reads one more topic, and still only looks. |
| **Related** | [ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/), whose driver this is for and whose account of the wiring this corrects; [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/) and [ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/), the other settings that treat named sources differently |

---

## Context

The turntable driver ([ADR-0021](/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening/))
reports what it hears, and three things it cannot hear are in the way.

**It cannot hear that the turntable is off.** ADR-0021 has the agent Pi
beside the amplifier, on its tape output. That is not where things are going.
The turntable is upstairs, with a converter that turns it into S/PDIF, and
the S/PDIF runs to the basement, where the Pi and its sound card are. Down
there the digital feed is chosen between the media server and the turntable,
and S/PDIF is there whether or not anything is playing on it. So what the
driver hears is not proof of a record, and silence is not proof of none.

**It cannot say that a record between its sides is still what is on.**
nowplayd picks a playing source over a paused one over a stopped one
([ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/)).
Lift the needle and the turntable is stopped, a track left paused in iTunes
that afternoon outranks it, and the room is lit for an album nobody is
listening to ([CHG-0013](/docs/changes/2026/0013-the-turntable-driver/)
left this open, as "a question for the arbiter, and a decision").

**It cannot hold against another source that is playing.** Two sources
playing each win when their track changes.

One fact answers all three, and it is not in the sound: the turntable is
switched on. It stands on a power strip that switches its other outlets with
it, which is how the converter comes on. Anything else on that strip comes
on with it too.

## Options considered

**Infer it from the audio.** Nothing to add. But the audio is what cannot
tell: the feed is always present, and is not always the turntable's.

**Tap something on the audio path** - the converter's lock light, the
selector. It would be exact. It is also a wire into equipment that works, in
the path the music takes, to learn what the power strip already knows.

**A beacon that publishes the turntable source's own presence**,
`nowplaying/source/turntable/status`. No change to nowplayd at all: a source
that is offline already does not count. But that topic is the driver's, and
means its sound card is delivering samples
([ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/));
two writers on one retained topic, each with a will, would say whichever
spoke last. And it could only take the turntable away. It could not hold it.

**The driver on the Pi reads the beacon and gates itself.** The knowledge
would stay with the turntable's own code. But a driver subscribes to nothing
but its commands, and the half of the problem that matters most - a silent
turntable against a paused iTunes - is between sources, which no driver sees.

**A beacon with a topic of its own, read by nowplayd**, which is told which
source it speaks for. One new device, one new thing before the arbiter's
rule.

## Decision

**A device on the turntable's switched outlet is connected to the broker
while the turntable is on, and says nothing else. nowplayd is told which
source that switch belongs to, holds that source as current while the switch
is on, and does not hear it while the switch is off.**

### The beacon

- **Its message is its connection.** It publishes `online`, retained, to
  `eds/beacon/<id>/status` each time it connects, and its last will is
  `offline`, retained, on the same topic. Nothing runs when the power goes;
  the broker says it.
- **It knows nothing of turntables.** The firmware (`firmware/power-beacon`)
  is the same for anything on a switch. Its name, `turntable-power-01`, and
  nowplayd's setting are the only places that say what shares its outlet.
- **It asks for a keepalive of ten seconds**, where the stand and the screen
  leave the library's two minutes. A broker sends a will after one and a half
  keepalives, so the room learns the switch went off in about fifteen
  seconds, not three minutes.
- **It is a registered device like the others**
  ([ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/)),
  with its own account, which may publish its presence and its events
  ([ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/))
  and may read nothing. Its secrets are provisioned, never compiled in
  ([ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/)).
  The WiFi, credentials, event log and partition table are copies of the
  stand's, as the screen's are.

### nowplayd

- **`NP_SOURCE_POWER` names, for a source, its beacon**:
  `turntable=turntable-power-01`. nowplayd subscribes to those beacons'
  topics and to no others.
- **While the switch is on, that source is current**, before playing, paused
  and stopped are compared at all. A record between its sides is reported as
  what it is: the turntable, stopped, with no track.
- **While the switch is off, that source is not heard**, whatever its driver
  reports.
- **A beacon that has not been heard from changes nothing.** The source is
  arbitrated as it was before there was a beacon, the way a source is taken
  to be online until it says otherwise. So nowplayd can be given the setting
  before the beacon exists, and a broker account not yet granted the topic
  is a warning in the log and not a turntable that never plays.
- **The switch does not stand in for the driver.** A source that is switched
  on and whose driver is offline, or has never reported, holds nothing.

### The console

It shows each beacon it hears of beside the other parts, lit when the switch
is on. Off is not shown as a fault: it is a turntable that is off.

## Consequences

- **A side can be turned over without the room changing.** Stopped with no
  track publishes no new cover, and the cover topics are left as they were,
  so the stand stays lit from the sleeve.
- **Until a record is named, the stand shows what it showed before.**
  Switching the turntable on makes it current at once, but it has no track
  for the first half-minute, and nothing repaints the room for that.
- **While the turntable is on, the cover screen's buttons do nothing.** A
  command goes to the source that is current
  ([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)),
  and the turntable takes none. iTunes cannot be paused from the screen with
  the turntable on. That is the price of "holds", and it is paid knowingly.
- **iTunes playing with the turntable on is not shown.** The switch is taken
  at its word.
- **nowplayd knows a kind of device now**, where it knew only sources. It
  knows a topic and the word on it, and not what the device is.
- **A beacon is one more thing that can be wrong.** If it loses WiFi while
  the turntable plays, the record stops being heard after about fifteen
  seconds, and the room goes to whatever else is current until it is back.
  Its events say how often: more `mqtt.connected` than `system.boot` is a
  connection that came and went.
- **A will needs a broker to send it.** A beacon switched off while the
  broker is down leaves `online` retained until it next connects. The
  turntable would be held, silent, and the screen's buttons idle, until the
  turntable is next switched on and off.
- **"About fifteen seconds" is the broker's to keep.** One and a half
  keepalives is when it may send the will, and it looks every few seconds:
  on the bench it took between twelve and nineteen.
- **The strip decides what "on" means.** It switches on what the turntable
  draws. That has been reliable for the converter, by its owner's account,
  and the beacon inherits it.

## What would settle it

Shown on the bench, 2026-10-12: the firmware builds for the board to be
used, an ESP32-S2 with 4 MB of flash, starts on it, and reports itself
unprovisioned. nowplayd's rule is tested as sequences of events, the
arbiter's own way. Provisioned, and restarted three times from its reset pin
beside the access point, the board said `online` 3.0 to 3.1 seconds after
starting ([CHG-0015](/docs/changes/2026/0015-the-turntables-power-beacon/)).
That is a reset and not the strip switching on, and not where it will live.
Held in reset to stand in for its power going, it read `offline` on the
console after 12.3 and 18.6 seconds; let go, `online` after 7.6 and 3.1.
nowplayd, deployed, named the switch as it started. With the turntable's
driver online and silent and a track paused in iTunes, the turntable was
current while the beacon was on, and iTunes 15.5 seconds after it was held
in reset.

Not yet shown:

- [ ] The seconds from the switch going on to `online` on the bus.
- [ ] The seconds from the switch going off to `offline`.
- [ ] A day on the strip's outlet without the connection coming and going.
- [ ] A side turned over with iTunes left paused: the stand keeps the sleeve
  and `nowplaying/current` stays the turntable's.
- [x] The beacon off: `nowplaying/current` goes back to what else there is.
  Shown with the board held in reset; the turntable's own switch is the
  first two items.

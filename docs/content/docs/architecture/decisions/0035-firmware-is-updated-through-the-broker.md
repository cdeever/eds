---
title: "ADR-0035: Firmware Is Updated Through the Broker"
weight: 35
---

# ADR-0035: Firmware Is Updated Through the Broker, and a Build That Cannot Reach It Is Undone

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until each device has taken an update over the air and one has rolled back; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-16 |
| **Scope** | How a new build reaches the stand, the cover screen and the beacon once each is in place; what a device does with one; and how a bad one is undone. |
| **Extends** | [ADR-0029](/docs/architecture/decisions/0029-the-stand-starts-dark-and-blinks-what-is-wrong/): the stand that cannot reach the broker still blinks, and now also gives up a new build that caused it. |
| **Related** | [ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/), why the broker sits where devices can reach it and nothing else of EdS can; [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), the beacon whose connection is its message; [ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/), the change that showed why this is wanted |

---

## Context

Every change to a device's firmware has meant a USB cable: the stand
unscrewed from its shelf, the beacon taken off its outlet, and the cover
screen and the beacon taking turns on the Mac's one serial port.
ADR-0034 is the case in point: three devices to flash before three
services could be deployed.

The three partition tables have had two app slots of 1.5 MB each from the
start, so that this would never need a device repartitioned. The builds
are 0.87 to 0.97 MB. Nothing yet writes the second slot.

**Where the devices can reach.** They are on Deevnet IoTaaS's IoT segment
(VLAN 30). What they can reach of EdS is the broker, on IoT Backend
(VLAN 35); the tenant's own network accepts nothing inbound, by design
(Deevnet ADR-0001, Deevnet ADR-0003). The workload can reach the broker too,
which is how lightd and nowplayd talk to the devices at all.

**Whose this is.** The devices are in trust class `iot`, the class for
devices whose firmware their owner controls. Their firmware, its builds
and its updates are EdS's. Deevnet IoTaaS issues the broker accounts and
enforces the topics each may use, from the tenant's own Terraform; it has
no update mechanism of its own for a tenant's devices, and none is needed.

## Options considered

**Each device downloads its update over HTTPS.** What ESP-IDF's
`esp_https_ota` is for, and the usual answer. The device must reach a web
server, and nothing of EdS's that serves HTTP is reachable from the IoT
segment. Making one would mean a server on IoT Backend, which is Deevnet
IoTaaS's segment, or a new inbound path into the tenant, which its design
refuses. A new network path to save writing a receiver.

**The Mac pushes to a server on each device.** The ArduinoOTA way. Every
device listens on a port, the Mac must reach VLAN 30, which it is not
meant to, and anything else on the IoT segment that can reach that port
can offer a build.

**The image goes through the broker, in pieces.** No new network path and
no new listener: each device already holds a connection to the broker that
it verifies by its CA and logs into with its own account. The broker's
ACLs say who may offer a build, per device. The cost is a small receiver
in each firmware and a sender, and about a megabyte through the broker
per update.

**One message, the whole image.** ESP-MQTT hands a large message over in
pieces anyway, so the receiver would be simpler. But it depends on Deevnet
IoTaaS's broker accepting a megabyte in one message, which is its setting
and not EdS's, and a connection lost in the middle starts the whole image
again.

## Decision

**A build is sent to one device through the broker, eight kilobytes at a
time, by a sender on the workload that waits for the device to say where it
is before each piece. The device writes it to its spare slot and boots it.
The new build is kept only once it has reached the broker; one that has not
within two minutes is undone, by the device, without a person.**

- **Three topics per device**, beside its status topic
  (`eds/lightstand/lp-stand-01/…`, `eds/screen/…`, `eds/beacon/…`):
  - `…/ota`: the offer, JSON: `{"v":1,"project":"lp-stand","size":…,"elf":"…"}`.
  - `…/ota/chunk`: a piece: four bytes of offset, big-endian, then up to
    8 KB of the image.
  - `…/ota/state`: the device's answers, JSON:
    `{"v":1,"state":"ready","next":0}`, `receiving`, `rebooting`,
    `refused` with a `why`, and on every connection `running` with the
    version and ELF hash of what it runs.

  All at QoS 1, **none retained**: an offer that waits on the broker for a
  device that was off is a build nobody is watching arrive.
- **The device answers each piece with the next byte it wants**, and the
  sender sends that, and nothing else, next. A piece sent twice, or out of
  turn, is ignored and answered the same way. That is all the flow control
  and all the recovery there is.
- **The device refuses a build that is not its own**: a `project` other
  than its own, more than its slot holds, or an image ESP-IDF finds corrupt
  at the end. A new offer abandons the one before. A minute without a
  piece abandons it too.
- **Written as it arrives** (`OTA_WITH_SEQUENTIAL_WRITES`): erasing the
  whole slot first would hold the MQTT task for seconds, longer than the
  beacon's ten-second keepalive.
- **A clean disconnect before the reboot.** The broker sends no will for
  it, so the beacon does not say the turntable is off for the few seconds a
  restart takes; the retained `online` stands, and the new build repeats it.
- **Kept once it reaches the broker.** ESP-IDF's rollback is on in every
  bootloader. A new build is on probation until it has connected to the
  broker, its own login accepted; then it is marked good. One that has not
  within two minutes marks itself bad and reboots into the old one, and
  one that crashes before then is put back by the bootloader. **A build
  that cannot take the next update never stays**, because reaching the
  broker is exactly what the next update needs.
- **The sender is `eds-ota`** (`tools/ota`), run on the workload over SSH
  from the device's own Makefile (`make ota`), with a broker account of
  its own, `ota`. It may publish offers and pieces to every device and
  read their answers, and nothing else. It reads the image's own
  description before sending: a project name that does not match the
  device it is sent to is not sent. It reports `running` with the new hash
  as success and the old one as a rollback.

## Consequences

- **One more USB flash each.** The bootloader is not updated over the air,
  and the one on each device has no rollback. The first build with this is
  flashed by cable, and is the last that has to be.
- **The firmware's build refuses an `sdkconfig` without rollback.** An
  `sdkconfig` made before this keeps it off; delete it, and the defaults
  turn it on.
- **Anyone who can publish to a device's `ota` topics can run code on
  it.** That is the `ota` account and nobody else; its password lives
  where lightd's does, on the workload. The images are not signed. Signing
  (`CONFIG_SECURE_SIGNED_APPS_NO_SECURE_BOOT`) would make a stolen broker
  password not enough; it is a key to keep, and is left for later.
- **A build that is fine, sent while the broker is down for two minutes
  after it boots, is undone.** The device goes back to the build it had,
  which works too; the update is sent again.
- **The stand stutters while it writes.** Writing flash pauses the cache,
  and so the frames, a few milliseconds a piece, for the ten or twenty
  seconds an update takes.
- **Deevnet IoTaaS changes nothing.** The `ota` account, and each device's
  three topics, are declared in the tenant's Terraform as every other
  grant is; applying them changes the device accounts in place.

## What would settle it

- Each device updated over the air, its `running` answer carrying the new
  hash, and the turntable not dimmed by the beacon's restart.
- A build that cannot reach the broker sent on purpose, and the device back
  on its old one, by itself, within three minutes.
- A cover screen's image sent to the stand, and refused before anything is
  written.

---
title: "ADR-0015: EdS Builds Its Own Pi Images"
weight: 15
---

# ADR-0015: EdS Builds Its Own Pi Images, on the Substrate's Base

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | Where the definition of an EdS Raspberry Pi image lives, what it is built from, and what "the relay" is. |
| **Extends** | [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/), which decided there would be a dual-homed Pi and did not say whose image it would run |
| **Related** | [ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/), [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/) |

---

## Context

The Pi that carries now-playing reports from the home LAN to the broker needed
an image. The substrate has an image factory that builds Raspberry Pi images,
and the first relay image was built there, as a new variant beside the
substrate's own.

It worked, and it was in the wrong place. The variant held the EdS agent's
service, its settings file, its pairing port and a status tool that knew what
the agent needed. Nothing in it made sense outside EdS, and every change to
the agent's needs would have been a change to a substrate repository for an
EdS reason.

Two further things were clear by then:

- **More EdS Pi images are coming.** One that identifies what a turntable is
  playing from its line-level output, for instance.
- **"The relay" was two things bundled.** A Pi that runs the agent near the
  players is product: something has to sit near iTunes and MusicBee and
  report to the broker wherever the broker is. The *second network leg* is a
  development arrangement, needed only because the players are at home and
  the broker is in a rack that travels.

## Options considered

**Keep it in the substrate's image factory.** The build machinery is already
there. But the substrate is supposed to be ignorant of what tenants run, and
its one tenant-facing image, the take-home backend, is deliberately generic.

**EdS's own images, built from scratch.** No dependency on the factory. But a
Pi in the rack is reached by the site's automation user, and that user and
its key are the substrate's to put in an image.

**EdS's own images, on the substrate's base.** The factory builds what is the
substrate's: Raspberry Pi OS with the automation user. EdS applies everything
else.

For the relay: **its own image**, or **a way of configuring the agent image**.

## Decision

- **EdS's Pi images are defined in the EdS repository**, under `images/`, one
  directory per image. Each is the substrate's base image with an EdS
  playbook applied to it offline.
- **The factory keeps only the base.** `images/build.sh` builds it with the
  factory's own definition and adds nothing to the factory.
- **The first image is the agent Pi** (`images/agent`).
- **A relay is an agent Pi with a second leg**, not an image. A card becomes
  one by being given a Wi-Fi network when its settings are written.
- **An image carries no secret and no name.** Its broker account, its
  hostname and any Wi-Fi key arrive on the card's boot partition after
  flashing. This is [ADR-0004](/docs/architecture/decisions/0004-firmware-secrets-nvs-only/)'s
  rule for firmware, applied to images.

## Consequences

- A change to the agent, its service or its tools is a change to one
  repository.
- One image serves every agent Pi, relay or not.
- The next EdS image is a new directory, not a new mechanism.
- **EdS's build reaches into the factory's checkout** for the base: its
  Packer definition, its container, its key. That is the one seam, and it is
  by path rather than by contract. A base-image target or a published base
  from the factory would make it clean.
- Builds still run on the Builder, since they need Linux, sudo and loop
  devices; the image definition is copied there for each build.
- The Pi itself is still **the substrate's host**: in its inventory, with a
  reserved address and the automation user. Whether an EdS Pi should instead
  be a device the tenant registers — which would also let it report to the
  log store — is not decided here. It is better asked when there are two.
- The image's rules are checked by its build, as the firmware's are by its
  tests: no secret in the image, forwarding off in two places, the firewall
  rules parse.

## Evidence

**2026-10-10.** The image was first built as a factory variant, then moved
here unchanged in substance and rebuilt.

- The factory was left exactly as it was found: its branch deleted, nothing
  committed.
- **The first card found a bug the build's checks had not.** Bonjour was
  restricted to the Wi-Fi leg by a line written `allow-interfaces = wlan0`,
  and its parser reads the key up to the `=`, spaces included. The daemon
  refused to start, the pairing advertisement never went out, and iTunes
  never offered to pair. The playbook now writes the line without spaces and
  reads it back, and the pairing tool fails at once if its advertisement
  stops rather than waiting out its five minutes.
- The running Pi was updated in place to match the moved image, kept its
  pairing, and passes `eds-agent-status`.

---
title: "ADR-0010: A Dual-Homed Pi Relays the Home LAN"
weight: 10
---

# ADR-0010: Home-LAN Drivers Reach the Broker Through a Dual-Homed Mobile Pi

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | How a driver that must be near a player on the home LAN reaches the broker at the Deevnet Mobile site. |
| **Extended by** | [ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/): the image is EdS's own, and a relay is a way of configuring it |
| **Related** | [ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/), [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), the Deevnet network segmentation standard ([deevnet-docs](https://deevnet.github.io/deevnet-docs/)) |

---

## Context

The Windows PCs that run iTunes and MusicBee sit on a home LAN. The broker
is inside the Deevnet Mobile site. There is no path from the home LAN into
the site, and the site's workloads cannot reach private addresses outside
it. The iTunes and MusicBee drivers speak network protocols to their
players, and pairing with iTunes needs multicast, so they have to be on the
players' LAN.

Something has to sit on both networks.

## Options considered

**The PCs also join the tenant developer Wi-Fi.** No extra hardware, but
every music PC becomes dual-homed and has to be in range of the rack.

**Move the PCs onto the site's trusted network.** Simplest for the drivers,
but it changes what those PCs are, and they follow the rack's availability.

**A relay box.** One small machine on both networks runs the drivers. The
PCs are untouched.

## Decision

One of the mobile site's Raspberry Pis is given a leg on the home LAN and
runs `npagent` with the home-LAN drivers. It already reaches the broker from
inside the site. It doubles as a development box.

## Consequences

- The PCs stay purely on the home LAN, with nothing installed beyond what the
  players themselves need.
- **Only the iTunes and MusicBee drivers depend on the relay.** The Spotify
  and TIDAL drivers run on the workload and need only the rack's internet
  uplink, so they work wherever the rack is.
- **The Pi is dual-homed across the home LAN and the site.** It must not
  forward packets between them, and its default route stays where it is.
  Whether that is acceptable under the site's segmentation standard is the
  operator's call; here it is taken as accepted by the operator, who made
  this decision.
- The relay's broker account is not a device account, so it cannot publish
  under `log/`
  ([ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/)).
  It logs to its own journal for now.
- When the rack leaves the house, the relay leaves with it, and iTunes and
  MusicBee go unreported until it returns.
- A driver that had to run on a PC itself — none is planned — would publish
  to a small MQTT relay on this Pi rather than reach the site directly. That
  would extend this record.

## How it was built

**2026-10-10**, in [CHG-0002](/docs/changes/2026/0002-relay-pi-and-now-playing-accounts/).

- The Pi is `dv02rpi002p01`. Its **Ethernet** is its leg on the site: the IoT
  segment, where its address was already reserved, and from which the broker
  is reachable. Its **Wi-Fi** is its leg on the home LAN.
- "Must not forward" is held in two places, and each is checked: forwarding
  off in the kernel, and a firewall whose forward chain drops everything. The
  image build fails without both; `eds-relay-status` checks both on the
  running Pi.
- "Its default route stays where it is" is held by the Wi-Fi connection
  itself: no default route, no DNS and no routes taken from DHCP.
- One thing this record did not anticipate: **pairing needs something to come
  in from the home side.** iTunes has to see the relay's Bonjour
  advertisement and call it back when the code is typed. So the home leg
  admits Bonjour and one fixed port, and nothing else — not SSH.
- **The image carries no secret.** The home Wi-Fi key and the agent's broker
  account are written to the card after it is flashed and imported at boot.
- The image was first built as a variant in the substrate's image factory,
  and moved to EdS the same day
  ([ADR-0015](/docs/architecture/decisions/0015-eds-builds-its-own-pi-images/)):
  it is `images/agent`, and the card's settings are written by `deploy/relay`.
- `eds-relay-status` became `eds-agent-status` with that move.

---
title: "ADR-0018: A Console of EdS's Own"
weight: 18
---

# ADR-0018: EdS Has a Console of Its Own, Fed From the Bus, and It Only Looks

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} 2026-10-11, once it had been watched in a browser on the site with tracks changing |
| **Amended** | 2026-10-11, before it was accepted: served over TLS, not in the clear. When this was written nothing on the site would certify a name a tenant serves; Deevnet ADR-0037 has since given tenants a CA to ask. [CHG-0007](/docs/changes/2026/0007-the-console-over-tls/) |
| **Date** | 2026-10-11 |
| **Scope** | Where a person looks to see what EdS is doing: what is playing, what the stand is showing, and whether the parts are up. |
| **Related** | [ADR-0005](/docs/architecture/decisions/0005-device-events-over-mqtt/), [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/), [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/) |

---

## Context

What EdS is doing was visible in three places, none of them a picture: the
tenant's Grafana dashboard, which lists events; `mosquitto_sub`; and the
services' journals. The stand has no LEDs attached yet, so nothing at all
showed its colors.

A panel that drew the stand was tried in Grafana first. Grafana takes its
colors from thresholds fixed in a dashboard and cannot color anything from
a value in the data. The way around that worked: the log query assembled an
SVG, as a `data:` address, from the palette in the stand's own event, and a
table showed it as an image. It worked, and it was a four-kilobyte query
building markup by string formatting, in a tool that was being worked
against.

Everything such a picture needs is already on the bus, retained: the current
track and its cover, the scene sent to each stand, and each part's presence.

## Options considered

**Draw it in Grafana**, as above. No new service. Limited to what a table of
images can show, dependent on the stand reporting its palette in an event,
and every change is another fight with the tool.

**A page served by `lightd` or `nowplayd`.** No new service. But `lightd`
knows nothing about tracks and `nowplayd` nothing about scenes
([ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)),
and a page that shows both would teach one of them the other's business.

**A service of its own that subscribes to the bus and serves a page.** One
more thing to run. It reads the same two contracts every other subscriber
does and needs nothing from any other service.

## Decision

A new service, **`console`**, on the tenant workload. It subscribes to the
bus and serves one page.

- **It learns everything from the bus**: `nowplaying/current` and its cover,
  each source's state and presence, each stand's scene, state and presence,
  and the services' presence. It calls no other service and no other service
  calls it. Because those topics are retained, a restarted console knows the
  state of the world at once.
- **It only looks.** Its broker account is granted subscriptions, and one
  topic to publish: its own presence. It cannot change a scene or a track.
  Controls are expected later and are a decision of their own: they change
  what this account may publish, and who may reach the page.
- **The browser is pushed changes** over one long-lived request (server-sent
  events), not polled.
- **The stand is drawn in the browser** from the scene descriptor, animated
  by a port of the firmware's effects. It shows the colors as sent, before
  gamma, which is what the wire format is for
  ([ADR-0002](/docs/architecture/decisions/0002-gamma-in-firmware/)).
- **It is reached at `https://console.eds.mobile.deevnet.net/`**, from the
  site's trusted and management networks. Its certificate is from the site's
  Tenant Service CA and chains to the Deevnet Root CA, which the browsers that
  open it already trust. The key is the tenant's own: made in its Terraform,
  held in its state and on its workload, and sent nowhere. The plain port only
  sends people to the secure one. The workload's own firewall admits the two
  ports. Nothing on the guest or IoT networks, or outside, can reach a
  workload at all.
- Grafana stays what it is good at: the events, over time.

## Consequences

- There is one place to look, and it shows the stand before the stand has
  LEDs.
- **The effects are now written a fourth time**: firmware, and a browser.
  A change to an effect in the firmware has to be made in the console too,
  or the picture lies. The console says which scene it is drawing, not what
  the LEDs are doing; only the stand's own `state` and presence say that it
  took the scene.
- **Anyone on the site's trusted networks can see what is playing.** There
  is no login. That is acceptable for a page that only looks, on a network
  that already carries the same information unencrypted to anyone with the
  Grafana login; it is not acceptable for a page with controls.
- **The certificate lasts a year and a person renews it.** A plan shows the
  renewal from sixty days before, and the console warns each time it starts
  inside thirty. A console nobody applies for a year is refused by every
  browser, and says why in its log.
- A certificate cannot be withdrawn. If the key is lost, a new one is made
  and the old certificate stays valid until it expires.
- The first port on the workload opened to anything but SSH. It is opened by
  the deployment, and a rebuilt workload gets it back with everything else.
- "Recently played" is what the console has seen since it started. The
  durable history is the log store's
  ([ADR-0017](/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device/)).
- While Spotify plays, the console shows a track with no cover beside a stand
  lit from an earlier one, and says so
  ([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).
- The Grafana panels that drew the stand were not kept. The stand's
  `scene.applied` event still gained its palette and speed, which is the
  record of what each stand was sent.

## What would settle it

1. The page, in a browser on the site, shows the track playing, its cover,
   and the stand in the colors `lightd` logged for it. **Settled 2026-10-11:**
   seen by its owner.
2. A track change reaches the page without a reload. **Settled 2026-10-11:**
   watched changing as tracks were played.
3. A restart of the console, and of the broker connection, leaves the page
   correct. **Settled 2026-10-11:** it has been redeployed four times, and
   each time came up with the track, cover, sources and scene from the
   retained messages alone. A dropped broker connection has not been forced.

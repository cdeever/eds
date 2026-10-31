---
title: "ADR-0040: A Backend Pi's Services Are Named Under Its App"
weight: 40
---

# ADR-0040: A Backend Pi's Services Are Named Under Its App, and nginx Answers for Its Pages

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}}: until it runs on the Pi in service; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-20 |
| **Scope** | The names the backend Pi's services answer to, what the devices dial, and how a browser reaches the console and Grafana |
| **Extends** | [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/): its console "serves the kit's server certificate, on 443", its firewall admitting Grafana on 3000, and item 6's `https://<hostname>.local/` |
| **Related** | [ADR-0018](/docs/architecture/decisions/0018-a-console-of-eds-own/), [ADR-0035](/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker/), [CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/), [CHG-0031](/docs/changes/2026/0031-the-backend-pi-named-under-eds/), Deevnet ADR-0039 (Proposed: tenants beside sites in the PKI) |

---

## Context

The backend Pi is home (CHG-0028). The home router's DNS names it
`dv01rpi001p01.home.deevnet.net`, and the services on it are named
directly under `home.deevnet.net`: `mqtt`, `grafana` and `logs`.
`console.eds` is the exception. The devices dial `mqtt.home.deevnet.net`.

Those names assume there is one backend in the house. There will be one
per app: Deevnet's take-home image is a whole broker, log store and
Grafana for one tenant, and a second app gets a second Pi. Two Pis
cannot both be `mqtt.home.deevnet.net`.

Two things were also left over from the take-home image, which assumes no
site DNS and names a card `<hostname>.local` by avahi:

- EdS's tools reached the Pi as `eds-backend.local` by default, and the
  README sent people to `https://<hostname>.local/`.
- Grafana is served by itself on `https://…:3000`. The console has 443,
  so Grafana cannot have it too, and its name needs a port after it.

A device's broker address is in its `creds` partition (`mqtt_uri`), which
overrides the build's default. An update through the broker (ADR-0035)
replaces the program and leaves `creds` alone, so it cannot move a device
to a new name. Provisioning by cable does.

## Options considered

**Names:**

- **Under the app: `<service>.eds.home.deevnet.net`.** Each app's Pi is
  a domain of its own, and the console's name already has this shape.
- **Directly under the site, as now.** That leaves one backend per house.
- **The app in the host part (`eds-mqtt.home…`).** This is a flat list
  with no domain to delegate, and nothing else in Deevnet is named this way.

**Ports for the pages:**

- **nginx on 80 and 443, choosing by name.** Grafana and the console go
  behind it on loopback. One more program on the Pi, from Debian, which
  presents the certificate the console and Grafana present now.
- **Leave the ports.** Grafana stays at `:3000`.
- **An address each**, so that Grafana and the console can each hold 443.
  That means a second reservation and a second address in the
  certificate, all to avoid a proxy.

## Decision

- **Every service of a backend Pi is named under its app's domain**:
  `mqtt.eds.home.deevnet.net` (8883), `logs.eds.home.deevnet.net` (8427),
  `grafana.eds.home.deevnet.net` and `console.eds.home.deevnet.net` (443).
  The machine keeps its site name, `dv01rpi001p01.home.deevnet.net`, for
  SSH and EdS's tools.
- **These names are interim.** Deevnet ADR-0039 (Proposed) gives each tenant
  CAs of its own, under the Deevnet Root, with names that drop the site:
  `*.eds.deevnet.net`. Until it is built, the Pi presents the card's own CA,
  and nobody's computer is asked to trust it: a browser warns, and tools pass
  the CA by file.
- **The devices are not moved yet.** They keep dialing
  `mqtt.home.deevnet.net` with the card's CA. Moving a device means
  provisioning it again by cable, with no rebuild (as in CHG-0028), so it is
  done once, with the final name and the CA bundle Deevnet ADR-0039 gives
  them. `make devices` defaults to `mqtt.eds.home.deevnet.net` for a device
  provisioned before then.
- **No name is written into the image.** The names are the card's (`names=`)
  and the router's. nginx picks by the first label only.
- **nginx holds 80 and 443.** Port 80 sends every request to HTTPS on
  the same name. A name beginning `grafana.` reaches Grafana; every other
  name or address reaches the console.
- **Grafana listens on loopback only**, through a drop-in to the kit's
  unit, which is otherwise left as Deevnet wrote it. The firewall no
  longer admits 3000.
- **The console serves plain HTTP on `127.0.0.1:8080`** and no longer
  holds a certificate or any privilege. nginx sends the one-day
  `Strict-Transport-Security` header that the console sent itself over
  TLS, and does not buffer its event stream.
- **nginx reads the kit's certificate** through the `deevnet-kit` group,
  as the console did. `eds-backend-status` fails when nginx presents a
  certificate other than the one on disk.
- **The broker and the log store keep their ports.** They are dialed by
  devices and tools, not typed into a browser.

## Consequences

- **A second app's Pi needs nothing of EdS's.** It gets
  `<app>.home.deevnet.net` and its own names under it.
- **Moving the devices is a step of its own**, three provisionings by
  cable, after Deevnet ADR-0039 is built. Until then the certificate carries
  both sets of names, and `mqtt.home.deevnet.net` stays EdS's.
- **Grafana's own URL does not appear in links it makes** (`root_url` is
  left at its default, since the image carries no name). Logging in and
  moving around work by relative paths. A link that Grafana writes out in
  full, such as a share link, names `localhost`.
- **More things have to be restarted after `regen-certs`.** The kit
  restarts the broker and the log store; Grafana and nginx are a person's
  to restart.
- **A Pi already in service gets nginx through `make push`.** The
  firewall's change waits for the next image. Until then 3000 is still
  admitted, and Grafana is not listening on it.

## What would settle it

On the Pi in service, with the new names in DNS and in the certificate,
and after `make push`:

1. `eds-backend-status` passes, and `deevnet-kit selftest` too, or says
   it expects 3000 on the network. If it does, record that here.
2. `https://grafana.eds.home.deevnet.net/` signs in from a browser, and a
   dashboard's live panels update.
3. `https://console.eds.home.deevnet.net/` shows the track change as it
   happens, without reloading.
4. `http://` to either redirects to `https://`; `:3000` does not answer
   from the Mac.
5. The next image builds with its checks, including `nginx -t`.

The devices' names are settled with Deevnet ADR-0039, not here.

## Evidence

**2026-10-20.** Written, with the image, `push.sh` and `make devices`
changed. Nothing has been run yet.

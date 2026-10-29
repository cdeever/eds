---
title: "CHG-0042: The Home Pi Becomes a Deevnet Pi"
weight: 42
---

# CHG-0042: The Home Pi Becomes a Deevnet Pi, and the Devices Hold Both Sites

| | |
|---|---|
| **Date** | 2026-10-26 |
| **Change type** | Migration |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The home backend Pi, `192.168.2.40` on DVNT-CORE, with a new card named `dv01rpi001p01` (the card in it now calls itself `dv02rpi003p01`); `turntable-power-01`, `cover-screen-01`, `lp-stand-01`, `respeaker-01`; the tenant's Terraform (one `deevnet_certificate`); the home network's DNS |
| **How** | `make -C deploy/backend image fetch card`; `deevnet-kit cert request` and `cert install`; `terraform apply` in `infra/deevnet-tenant-eds`; `make pair spotify turntable drivers status devices`; `provision_tenant.py --site home --also mobile=terraform` on each device |
| **Risk** | Medium. Most likely to go wrong: a device refusing the broker's new certificate. No ESP32 has yet verified a server through Deevnet's Tenant Service CA to the Root |
| **Decisions** | [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/), [ADR-0049](/docs/architecture/decisions/0049-a-device-holds-two-sites-and-is-told-which/); Deevnet ADR-0040 (site-less names) and ADR-0041 §2 (a Deevnet Pi) |
| **Related** | [CHG-0028](/docs/changes/2026/0028-the-back-end-on-one-pi-on-dvntm/), which built the card this replaces; Deevnet CHG-0054 and CHG-0056, which built the names and the kit's `cert` commands |

---

## Summary

The home Pi runs EdS on a card whose certificate is signed by that card's own
CA, for names entered by hand in the home DNS. The devices dial its address
and trust that CA, and hold one site's settings.

After: a new card, whose certificate is Deevnet's, from the Tenant Service
CA, for the tenant's site-less names (`mqtt.eds.deevnet.net` and its
siblings) and for each of them at home and mobile. Everything on it trusts
the Deevnet Root. The devices dial `mqtt.eds.deevnet.net`, trust the Root,
and hold both sites: home in use, mobile beside it, for the switching
firmware to come (ADR-0049).

## Goal

- `sudo deevnet-kit status` on the Pi names `mqtt.eds.deevnet.net`, and the
  certificate's issuer is the Tenant Service CA.
- `make status` says ok for every check.
- `/etc/eds/lightd.env` on the Pi has `MQTT_HOST=mqtt.eds.deevnet.net`.
- Each device's presence is green on the console, from this Pi's broker.
- A record on the turntable lights the stand.

## Scope

**In scope:** the new card, its certificate, the home DNS names, and the four
devices' `creds` partitions.
**Out of scope:** the switching firmware (ADR-0049), the devices' firmware,
the tenant's workload and the relay Pi.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| Home is dark from the swap until step 6 | The whole back end | Done in one sitting; the old card is kept unchanged |
| A device refuses the new broker | The first device moved | The beacon first: nothing is attached to it. Its old settings are kept to put back |
| `eds-backend-setup` on Deevnet names, never run on hardware | The services on the Pi | `make status` before any device is moved |
| The old card's device settings overwritten | `deploy/backend/devices.json`, `deevnet-kit-ca.pem` | Copied aside before `make devices` (step 7) |

## Procedure

### Step 1: Build the image

**Run:** `make -C deploy/backend image fetch`

**Verify:** the build's check of the base passes: its `deevnet-kit` has
`cert install` (0.15.0).

### Step 2: The card

Flash `eds-pi-backend.img.xz` with no customization, then:

**Run:** `make -C deploy/backend card PI_HOSTNAME=dv01rpi001p01`

### Step 3: Swap and first boot

Power off the Pi, keep the old card, put in the new one, and power on. Allow a
few minutes for both boots.

**Verify:** `ssh eds@dv01rpi001p01.local sudo deevnet-kit status` answers.

### Step 4: The certificate

**Run**, on the Pi: `sudo deevnet-kit cert request`. Then a
`deevnet_certificate` in the tenant's Terraform with that request, name
`mqtt`, names `logs`, `grafana` and `console`, and sites `home` and `mobile`;
`terraform apply`. Copy the full chain and the Deevnet Root to the Pi:

```bash
sudo deevnet-kit cert install fullchain.pem deevnet-root-ca.pem
sudo systemctl restart eds-backend-setup
```

**Verify:** `sudo deevnet-kit status` names the Deevnet names, and
`/etc/eds/lightd.env` has `MQTT_HOST=mqtt.eds.deevnet.net`.

### Step 5: Drivers and checks

**Run:** `make pair`, `make spotify`, `make turntable DEVICE=plughw:CODEC`,
`make drivers DRIVERS=itunes,spotify,turntable`, `make status`

**Verify:** every check ok.

### Step 6: Home DNS

`mqtt`, `logs`, `grafana` and `console`, under `eds.deevnet.net` and
`eds.home.deevnet.net`, answer `192.168.2.40` on the home network.

**Verify:** from a computer at home, `https://console.eds.deevnet.net/`
loads with no warning where the Deevnet Root is trusted.

### Step 7: The devices

Copy `deploy/backend/devices.json` and `deevnet-kit-ca.pem` aside: they are
the old card's. Then `make devices`, and on each device, the beacon first, in
its firmware directory:

```bash
python3 tools/provision_tenant.py -p <port> \
    --outputs-json ../../deploy/backend/devices.json \
    --ca ../../deploy/backend/deevnet-kit-ca.pem \
    --site home --also mobile=terraform
```

**Verify:** before each write, the two sites printed are home (DVNT-CORE,
`mqtt.eds.deevnet.net`) in use and mobile beside it; after it, the device's
presence is green on the console. Only after the beacon is, the others.

## Verification

Every check under Goal.

## Undo

Before step 7: power off, put the old card back. Nothing else has changed but
a certificate issued, which is left to expire.

After a device is moved: the old card back in, and that device provisioned
from the copies kept in step 7 (`provision_tenant.py` with them, and no
`--site`).

## Outcome

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| 2026-10-27 | 1–3 | The image built, the card written and booted; `make status` answered |
| 2026-10-27 | 4 | `cert request` on the Pi; `deevnet_certificate.backend` applied with provider 0.8.0, serial `55:c1:0d:cb:…:9d:0a`, valid to 2027-10-28, for `mqtt`, `logs`, `grafana`, `console` at `eds`, `eds.home` and `eds.mobile`. It chains to the Deevnet Root and is for the request's key |
| 2026-10-29 02:20 UTC | 4 | The pending request's key checked against the certificate's, then `cert install` and `restart eds-backend-setup`. `deevnet-kit status`: "trusts the Deevnet Root (a Deevnet Pi)", broker `mqtts://mqtt.eds.deevnet.net:8883`; `lightd.env` and `nowplayd.env` have `MQTT_HOST=mqtt.eds.deevnet.net` |
| 2026-10-29 02:22 UTC | 4 | `deevnet-kit selftest`: 13 of 15. Grafana was still starting, and its log data sources still dialed `https://localhost:8427`, which the new certificate does not name. `systemctl restart deevnet-kit-dashboards` rewrote them as `logs.eds.deevnet.net`; then all 15 passed |
| 2026-10-29 02:25 UTC | 5 | `make status`: every check ok but iTunes, whose PC at `192.168.2.104` did not answer (asleep or not running; the pairing is on file) |
| 2026-10-29 02:27 UTC | 6 | Home DNS already answered `mqtt`, `logs`, `grafana` and `console` under `eds.deevnet.net` and `eds.home.deevnet.net` with `192.168.2.40`. `https://console.eds.deevnet.net/`, `https://console.eds.home.deevnet.net/` and Grafana answered 200 verified against the Deevnet Root alone, and the Mac's keychain trusts it |

### Departures from the plan

- The tenant's provider pin went from `~> 0.7` to `~> 0.8`: a certificate's
  `sites` first came in 0.8.0.
- The same day's applies also created `grafana_dashboard.covers`, waiting
  since ADR-0050 was merged without one.
- Before the certificate, the Pi warned that its card certificate did not
  name its IPv6 address. A Deevnet certificate names no address.
- The card is named `eds-backend`, not `dv01rpi001p01`: it was written
  with `make card`'s default `PI_HOSTNAME`.
- `cert install` restarted the services but not `deevnet-kit-dashboards`,
  so Grafana's log data sources kept the card's `localhost` until that was
  restarted by hand.
- The Pi has two addresses on the home network, `192.168.2.40` (wired) and
  `192.168.2.228`.

## Follow-ups

- [ ] Deevnet: `deevnet-kit cert install` should rerun the dashboards step,
      so Grafana's data sources dial the new names.
- [ ] Deevnet: on a Deevnet Pi, `status` and `selftest` warn that the
      certificate names none of the Pi's addresses and suggest
      `regen-certs`, which then refuses. A Deevnet certificate names no
      address by design; the check should not run in that mode.
- [ ] Step 7, the devices, the beacon first.

- [ ] The switching firmware (ADR-0049), over the air to the stand, the
      screen and the beacon; by cable to the reSpeaker.

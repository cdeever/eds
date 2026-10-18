---
title: "CHG-0028: The Back End on One Pi, Tried on DVNTM"
weight: 28
---

# CHG-0028: The Back End on One Pi, Tried on DVNTM

| | |
|---|---|
| **Date** | 2026-10-18 |
| **Change type** | Deployment |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `dv02rpi003p01` (a Pi 4, wired to DVNTM's IoT segment on `Gi1/0/11`, `10.20.30.13`, and on DVNT-CORE by Wi-Fi), with a new card. Nothing of the tenant's, the workload's, the relay Pi's or the devices' |
| **How** | `make -C deploy/backend image fetch`; the card flashed by its owner; `make card`; first boot; `make status` |
| **Risk** | Low. Most likely to go wrong: the card's first boot not finishing (`deevnet-kit` or `eds-backend-setup`), which leaves a Pi to reflash and nothing else touched |
| **Decisions** | [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/), whose evidence this is |
| **Related** | Deevnet CHG-0048, which gave `dv02rpi003p01` its switch port; Deevnet CHG-0049, which rebuilt the take-home image this is built on |

---

## Summary

The whole of EdS on one Pi, beside the tenant: its own broker, log store,
Grafana, the four services and the agent. It is tried on the kit's network
first, where the devices are, and goes to the home network only once it
works.

Nothing that runs today changes. The devices stay on the tenant's broker
until the owner chooses to move one, which is a later step of its own.

## Goal

- The image builds on the Builder, and its own checks pass.
- On first boot: `deevnet-kit selftest` and `eds-backend-status` pass.
- `free -m` with everything running leaves room.
- The console opens at `https://10.20.30.13/` from the Mac on DVNTM, with
  the card's CA trusted.
- A scene posted by hand to `lightd` is on the Pi's broker, retained, and is
  still there after the plug is pulled and put back.

## Scope

**In scope:** the image, the card, the Pi's first boot and the checks above.
**Out of scope:** moving any device to the Pi's broker; dashboards; going
home to DVNT-CORE alone. The agent's drivers are in scope: the Pi's Wi-Fi is
on DVNT-CORE, where iTunes is.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| First boot does not finish | the new Pi | Reflash; nothing else touched |
| The Pi is mistaken for the tenant's broker | the devices | No device is given its address in this change |
| The card's CA is trusted on the Mac | the Mac | Trusted for this card only; removed if the card is thrown away |

## Procedure

### Step 1: Build the image

**Run:**

```bash
make -C deploy/backend image fetch
```

**Verify:** the build's assertions pass; the checksum matches.

### Step 2: Write the card

The owner flashes `deploy/backend/eds-pi-backend.img.xz` with Raspberry Pi
Imager, as a custom image, and leaves the card in.

**Run:**

```bash
make -C deploy/backend card PI_HOSTNAME=dv02rpi003p01 PI_USER=<login>
```

DVNT-CORE and its key for the Wi-Fi, which is where iTunes is; the cable is
DVNTM's IoT segment.

### Step 3: First boot

The card in `dv02rpi003p01`, on its cable. Raspberry Pi OS makes the login
and grows the filesystem, then `deevnet-kit` and `eds-backend-setup` run once.

**Verify:**

1. `make -C deploy/backend status PI=10.20.30.13`.
2. `sudo deevnet-kit selftest`; `free -m`.
3. The server certificate names both addresses; if not,
   `sudo deevnet-kit regen-certs` and restart the console and the broker.

### Step 4: The console and the retained scene

**Verify:** the Goal's last two checks.

## Verification

The Goal's checks; ADR-0033's items 1 to 3, 5 and 6.

## Undo

Power the Pi off and take the card out. Nothing else was changed.

## Outcome

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| 2026-10-18 10:47 | before | Deevnet CHG-0049: `pi-backend` rebuilt with `deevnet-kit` v0.11.0 and republished. |
| 2026-10-18 11:04 | 1 | `make -C deploy/backend image fetch` from `b4d7594`: the published image taken and checked against its checksum; `backend/config.yml` `ok=54 changed=27 failed=0`, all seven assertion tasks passing (kit new enough, no secret, firewall parses and admits the broker and console, palette and the recognizer import, the agent may open a sound card). `eds-pi-backend.img.xz`, 1.02 GB, SHA-256 `f518c076…`, fetched and verified. |
| 2026-10-18 | 2 | The card's settings written through a folder on the Mac and copied on in Finder: the Mac's terminal is not allowed to write to removable volumes. Wi-Fi DVNT-CORE, the `eds` login with the owner's key. |
| 2026-10-18 15:50 UTC | 3 | First boot finished: `deevnet-kit` and `eds-backend-setup` ran; the broker and the four services active; `npagent` stopped with 78, as it should with nothing paired. **Wi-Fi did not join**: the 4-way handshake failed, the key mistyped at the card step. Set again on the Pi with `nmcli`: DVNT-CORE as `192.168.2.228/23`. |
| 2026-10-18 15:59 UTC | 3 | **The kit issued its CA and certificate before the clock was set** (`notBefore` 2025-11-24, the image's saved clock), and without the Wi-Fi address. `deevnet-kit regen-certs`, then Grafana and the console restarted (regen restarts only the broker and vmauth): the certificate now names `10.20.30.13`, `192.168.2.228` and the home IPv6 address, to 2029. The CA keeps its 2025 start; it is valid to 2035. |
| 2026-10-18 16:00 UTC | 3 | `deevnet-kit selftest`: 15 of 15, once Grafana had started. `eds-backend-status`: every service running and answering; all of `accounts.conf` on the broker. `free -m`: 513 MB used of 7808. Default route by the cable (metric 100), Wi-Fi 600; the internet reached. |
| 2026-10-18 16:00 UTC | 3 | Discogs: the two lines of `deploy/discogs.env` put in the boot partition's `eds/` over SSH and `eds-backend-setup` run: imported, `nowplayd.env` rewritten, nowplayd restarted; the collection listed, 226 records. |
| 2026-10-18 16:02 UTC | 4 | A scene posted to `lightd` on the Pi: on `eds/lightstand/lp-stand-01/scene`, and read by a new subscriber as the console, `retained=1`. From the Mac on DVNTM, with the card's CA: the console at `https://10.20.30.13/` and Grafana's health, both 200, verified. |
| 2026-10-18 | after | The owner reserved DVNT-CORE addresses in the home OPNsense: the Pi's **eth0** (`dc:a6:32:c5:a1:4d`) at `192.168.2.40` as `dv01rpi001p01.home.deevnet.net`, with `mqtt`, `grafana`, `logs`, `console.eds` and `services.eds` on it; `lp-stand-01` `.41`, `cover-screen-01` `.42`, `turntable-power-01` `.43`; `.44` a second Pi, `.45`–`.47` spare. |
| 2026-10-18 17:56 UTC | 3 | Deevnet CHG-0050: `deevnet-kit` 0.12.0, which takes extra names, copied onto the Pi (0.11.0 kept beside it); no reflash. `names set` with `192.168.2.40` and both sites' names, `regen-certs`, the console restarted. From the Mac on DVNTM, dialing `10.20.30.13`: `mqtt.home…`, `dv02rpi003p01.mobile…` and `192.168.2.40` verify on 8883, `console.eds.home…`, `grafana.home…` and `logs.home…` on their ports; `mqtt.mobile.deevnet.net` is refused. Selftest 15 of 15. |
| 2026-10-18 15:23 UTC | 1 | Rebuilt on kit 0.12.0: Deevnet CHG-0051 republished `pi-backend`, and the image's kit check now also requires `deevnet-kit names`. `make image fetch` from `9215f79`: `ok=54 changed=27 failed=0`, all seven assertion tasks passing; `eds-pi-backend.img.xz`, 1.04 GB, SHA-256 `6321e451…`, verified. pi003 to be reflashed with it, its card given `names=` for both sites. |

### Departures from the plan

- The card's settings went on through Finder, not `make card` directly.
- The Wi-Fi key was set on the Pi, after the first boot.
- The certificate was reissued, which the plan expected only if the Wi-Fi
  address was missing; it also corrected the dates.
- Discogs was added on the Pi: `make card` from a worktree did not find
  `deploy/discogs.env` (ADR-0037).

## Follow-ups

- [ ] The agent's drivers on the Pi (ADR-0033 item 4): pair with iTunes over DVNT-CORE.
- [ ] A device on the Pi's broker (items 7 and 8), when the owner chooses.
- [ ] Home to DVNT-CORE: the cable moves, the Wi-Fi connection is deleted,
      `.228` comes off the certificate; the devices are built and provisioned
      for DVNT-CORE and `192.168.2.40` or `mqtt.home.deevnet.net`.
- [ ] Rebuild `pi-backend` and this image on kit 0.12.0, raise the image's
      kit check, and give the prod Pi's card `NAMES=` (`deploy/backend/Makefile`).
- [ ] For Deevnet: `deevnet-kit init` issues the CA and certificate before the
      clock is set, and `regen-certs` does not restart Grafana.
- [ ] `make card` can write to a staging folder (`BOOT=`), but checks for
      the two marker files the image puts on the card; say so in the README.

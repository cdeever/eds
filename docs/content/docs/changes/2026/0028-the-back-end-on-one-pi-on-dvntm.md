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

### Departures from the plan

-

## Follow-ups

- [ ] The agent's drivers on the Pi (ADR-0033 item 4): pair with iTunes over DVNT-CORE.
- [ ] A device on the Pi's broker (items 7 and 8), when the owner chooses.
- [ ] Home to DVNT-CORE.

---
title: "CHG-0027: The Beacon on an ESP32"
weight: 27
---

# CHG-0027: The Beacon on an ESP32

| | |
|---|---|
| **Date** | 2026-10-18 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-18 |
| **Systems** | `turntable-power-01`: a new board, a plain ESP32, in place of the ESP32-S2 |
| **How** | `make build flash provision` in `firmware/power-beacon`, by cable |
| **Risk** | Low. Most likely to go wrong: the old S2 powered at the same time as the new board. Both log in as `turntable-power-01`, the broker takes the account from one for the other, and each time says `offline` for the turntable |
| **Decisions** | [ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/), unchanged: any ESP32 with WiFi and 4 MB will do |
| **Related** | [CHG-0015](/docs/changes/2026/0015-the-turntables-power-beacon/), the first beacon; [CHG-0025](/docs/changes/2026/0025-presence-repeated-and-updates-through-the-broker/), where the S2 needed an external antenna to join from the turntable |

---

## Summary

The beacon moves from an ESP32-S2 to a plain ESP32 that fits the case its
owner bought. The case has no opening for an external antenna, and the S2
could not join from the turntable without one. The new board's antenna is
on the board.

Before anything was built for it, the board was flashed with ESP-IDF's
WiFi scan example, from a scratch copy, and read at the turntable, on a
cable beside it: DVNTM-IOT at −58 to −59 dBm, DVNT-CORE at −45 to −48 dBm,
over four scans. It stays on DVNTM-IOT and the mobile broker for now.

It is the same beacon, `turntable-power-01`: the same account, the same
topics, the same `ota` grant. Nothing in the tenant changes.

The firmware's target changes, and so does `make chip`. It used to stop
unless the board was an S2, which is what kept it from writing over the
cover screen. Every board EdS has is now a plain ESP32, so it checks the
MAC instead.

## Goal

- `make chip` stops for any board but `70:4b:ca:27:06:14`.
- `make flash` writes the bootloader with rollback, the partition table and
  the app; `make provision` writes the `creds` partition.
- At the turntable, on its outlet, in its case: `system.boot` and
  `wifi.connected` from `turntable-power-01`, with the RSSI, and the console
  showing it `online`. nowplayd logs it switched on.
- Switched off with the turntable: `offline` within about fifteen seconds.

## Scope

**In scope:** the new board, and the firmware's target and `make chip`.
**Out of scope:** moving the beacon to DVNT-CORE and the home Pi
(`PLAN-one-pi-backend.md`); the stand and the screen.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| The old S2 powered with the new board | the broker, nowplayd, the stand | The S2 off its outlet before the new board is provisioned; erased afterwards |
| The wrong board written | the Mac's serial port | `make chip` checks the MAC |
| It does not join from the outlet in its case | the beacon | Read at the turntable already; its `wifi.connected` carries the RSSI. The S2 back on its outlet undoes it |

## Procedure

### Step 1: Flash

Nothing in service changes: the board has never held anything of EdS's.

**Run:**

```bash
cd firmware/power-beacon
. ~/.espressif/tools/activate_idf_v6.1.sh
make build flash
```

**Verify:**

1. `make chip` names the board as this beacon; the flash verifies.
2. The serial log: the app starts, unprovisioned.

### Step 2: Provision

Needs the tenant's Terraform outputs, so the Mac on the kit's network. The S2 is
unplugged first.

**Run:**

```bash
make provision
```

**Verify:**

1. `make monitor`: WiFi joined, the broker reached, `online` published.

### Step 3: On its outlet

The board in its case, on the turntable's outlet.

**Verify:**

1. The turntable switched on: `wifi.connected` with its RSSI; `online` on
   the console; nowplayd logs the source's equipment switched on.
2. Switched off: `offline`, and the stand as ADR-0028 says.

## Verification

The Goal's checks, and a day on the outlet without the connection coming and
going.

## Undo

The S2 was erased after the change, so undo is a build for it and a flash
and provision by cable: `CONFIG_IDF_TARGET` back to `esp32s2`, and
`CHIP=esp32s2 MAC=7c:df:a1:3c:d6:62 PORT=/dev/cu.usbserial-110` to make. Then
it goes on the outlet, with its external antenna, and the new board off it.

## Outcome

*Completed after the change has run.*

| When | Step | What happened |
|---|---|---|
| 2026-10-18 | before | ESP-IDF's scan example, repeating every three seconds, flashed from a scratch copy; nothing of it committed. The board had come with a factory image, no EdS partition table. At the Mac: DVNT-CORE −36 to −40 dBm, DVNTM-IOT −70. At the turntable, on a cable: DVNT-CORE −45 to −48, DVNTM-IOT −58 to −59. |
| 2026-10-18 | 1 | `make build flash` from `522e5fc`, on `/dev/cu.usbserial-0001` at the turntable: `make chip` named it this beacon; bootloader, partition table, `otadata` and app written and verified. `make chip MAC=00:11:22:33:44:55` refused it. It boots `522e5fc` from `ota_0`, says `not provisioned`, and keeps trying DVNTM-IOT with no key. |
| 2026-10-18 | 2 | First try not run: the Mac was on the home network, where the tenant's state cannot be read (`tfstate.mobile.deevnet.net` does not resolve). Waits for the Mac on DVNTM-TD. |
| 2026-10-18 13:25 UTC | 2 | The S2 unplugged; the Mac on DVNTM. `make chip`, a dry run, then `provision_tenant.py` with the main checkout's tenant directory: the `creds` partition written. At the turntable, on the Mac's cable: joined DVNTM-IOT on the first attempt, −65 dBm, as 10.20.30.201; `online` published. In the log store: `system.boot` (firmware `522e5fc`), `wifi.connected`, `mqtt.connected`. The console: `turntable-power-01` online. |
| 2026-10-18 13:27 UTC | 3 | Taken off the Mac's cable: nowplayd went idle with nothing switched on (`switched_on=false`, 13:27:32). |
| 2026-10-18 13:29 UTC | 3 | In its case, on the turntable's outlet, switched on: up 2.7 s after power (`mqtt.connected` at that uptime), first attempt, −66 dBm joining and −64 at the broker. nowplayd: `a source's equipment was switched source=turntable on=true` at 13:29:14.460, `current` stopped with the turntable named, the room waiting for a record. The console: online. |
| 2026-10-18 13:30 UTC | 3 | Switched off at the outlet: the broker sent the will, and nowplayd logged `source=turntable on=false` at 13:30:51.7; `current` stopped with no source; the console `offline`. Fifteen seconds later (`NP_IDLE_AFTER`) nowplayd went idle with nothing switched on, and lightd sent the stand `look=""`, brightness 0, at 13:31:06.7: dark, from standby at 0.15. |
| 2026-10-18 | Verification | Every Goal check met but the day on the outlet, left as a follow-up. |
| 2026-10-18 | after | The old S2 on the Mac, `/dev/cu.usbserial-110`: ESP32-S2 revision v0.0, MAC `7c:df:a1:3c:d6:62`. Its whole flash erased with `esptool erase-flash`; the `creds` partition's range read back all `0xFF`. It had not reached the broker before the erase: no event from it, nothing switched in nowplayd's log. No backup of its factory image was ever taken, so erased is as near factory as it gets. |

### Departures from the plan

-

## Follow-ups

- [ ] A day on the outlet without the connection coming and going.
- [x] The old S2's MAC recorded, and the board erased, credentials and all.

---
title: "CHG-0025: Presence Repeated, and Updates Through the Broker"
weight: 25
---

# CHG-0025: Presence Repeated, and Updates Through the Broker

| | |
|---|---|
| **Date** | 2026-10-16 (planned) |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The tenant's broker accounts; `cover-screen-01`, `turntable-power-01` and `lp-stand-01`, by cable; lightd, nowplayd and the console on the services workload; `npagent` on the agent Pi |
| **How** | `make plan apply` in `infra/deevnet-tenant-eds`; `make build flash` in each firmware; `make -C deploy deploy WHERE=builder`; `make -C deploy/relay push`; `make ota` |
| **Risk** | Medium. Most likely to go wrong: the plan replacing a device's broker account instead of changing it, which issues a new password and strands that device until it is reprovisioned |
| **Decisions** | [ADR-0034](/docs/architecture/decisions/0034-presence-is-heard-not-remembered/) and [ADR-0035](/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker/), which this is the evidence for |
| **Related** | [CHG-0024](/docs/changes/2026/0024-spotify-turned-off-again/), the day's other change |

---

## Summary

Two decisions go out together, because the firmware for both goes on by the
same cable.

ADR-0034: everything that says it is there says it again every thirty
seconds, and the console and nowplayd believe only what they hear. Old
firmware never repeats, so the devices are flashed before the services are
deployed, or the new readers gray them out.

ADR-0035: a build reaches a device through the broker. The bootloader that
can undo a bad build only goes on by cable, so this flash is the last one by
cable each device needs. The devices' accounts gain their three `ota`
topics, and a new account, `ota`, is the only one that can send a build.

## Goal

- `terraform plan` showed three device accounts changed in place, one
  account added, and nothing replaced; it was applied.
- `make ota` in `firmware/power-beacon` ends `kept:`, and the turntable was
  not dimmed by the beacon's restart.
- The same in `firmware/cover-screen`, and in `firmware/lp-stand` once the
  stand is plugged in.
- The console shows every part green, and `lp-stand-01` gray while it is
  unplugged.

## Scope

**In scope:** the grants and the `ota` account; the three firmwares, by
cable; the services and the agent; one update over the air to each device.

**Out of scope:** signed images (ADR-0035's consequences); the stand, if it
is not plugged in today, which follows when it is.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| A device account replaced, not changed: a new password | the tenant | the plan is read before apply: `~ update in-place` for `lp_stand_01`, `cover_screen_01` and `turntable_power_01`, `+ create` for `ota`, and no `-/+` |
| An old `sdkconfig` with rollback off | each firmware directory | the build stops with an `#error` that says so; delete `sdkconfig` and build again |
| New readers before new devices gray the devices out, and the beacon lapses to unknown | console, nowplayd | the devices are flashed first; until then a gray beacon holds nothing, which is ADR-0023's "never heard from" |
| The beacon's flash takes the turntable's power word away for a minute | the room | done with the turntable off |
| A build sent over the air that cannot reach the broker | a device | it goes back to the build before it, by itself, within two minutes |

## Procedure

### Step 1: the grants and the `ota` account

**Run:**
```bash
cd infra/deevnet-tenant-eds
make plan     # read it: see Risk
make apply
```

**Verify:**
1. `terraform output -json ota_broker` has a username, and the three
   device accounts' `granted_subscribe` include their `ota` topics.

### Step 2: the cover screen and the beacon, by cable

One at a time on the Mac's serial port. The creds partition is not
written, so neither is reprovisioned.

**Run:**
```bash
. ~/.espressif/tools/activate_idf_v6.1.sh
cd firmware/cover-screen && rm -f sdkconfig && make build flash
cd ../power-beacon && rm -f sdkconfig && make build flash
```

**Verify:**
1. Each logs `system.boot` and `mqtt.connected` (`make -C firmware/lp-stand logs`).

### Step 3: the services and the agent

**Run:**
```bash
make -C deploy deploy WHERE=builder   # also installs ota-broker.env
make -C deploy/relay push
```

**Verify:**
1. The console's header: every part green within thirty seconds.
2. `lp-stand-01` gray, while it is unplugged.

### Step 4: one update over the air to each device

With the turntable off for the beacon's.

**Run:**
```bash
make -C firmware/power-beacon ota
make -C firmware/cover-screen ota
```

**Verify:**
1. Each ends `kept:` with the ELF hash it offered.
2. `ota.offered`, `ota.applied`, `system.boot` and `ota.kept` in the device's events.

### Step 5: the stand, when it is plugged in

`rm -f sdkconfig && make build flash` by cable, then `make ota`.

## Verification

- The Goal, each item.
- Optional, and what settles ADR-0035's rollback: a build that never dials
  the broker (the call to start MQTT taken out) sent to the beacon with
  `make ota`, and `eds-ota` reporting `undone:` within three minutes.

## Undo

- **The services:** the previous images (`git checkout` the commit before,
  `make -C deploy deploy WHERE=builder`).
- **The firmware:** the previous build by cable. Over the air works too,
  once a device has this build: `make ota` from the older checkout, so long
  as it carries ADR-0035.
- **The grants:** take them out of `main.tf` and apply. The `ota` account's
  password goes with it.

## Outcome

| When (UTC) | Step | What happened |
|---|---|---|
| 2026-10-16 | 1 | `make plan`: 1 to add, 3 to change in place, 0 to destroy; no password among the changes. Applied by the owner. `ota_broker` grants as declared. |
| 2026-10-17 | 2 | `cover-screen-01` flashed by cable with `cb65a7a` (rollback bootloader). It was on `/dev/cu.usbserial-2110`, not the Makefile's default `2120`. Booted from `ota_0`, reached the broker, subscribed. |
| 2026-10-17 | 4 | Ahead of Step 3: `ota-broker.env` alone installed on the workload, so the screen could be updated before the services are. A test build (the screen app drawing color bars, version `ota-demo-bars`) sent with `make -C tools/ota send`: `kept` in 16 s, color bars seen on the screen. Then `cb65a7a` sent back: `kept` in 17 s. Two updates over the air, one into each slot, no cable. |
| 2026-10-17 | 2 | `turntable-power-01` flashed by cable with `cb65a7a`, on `/dev/cu.usbserial-110`. Far from the kit and with no antenna it could not join: `auth -> init (0x200)`, eleven tries and more. With an external antenna: joined on the first try, −57 dBm. |
| 2026-10-17 11:25 | 4 | `make ota` to the beacon: `kept` in 21 s, `ota.offered`, `ota.applied`, `system.boot` (`software`), `ota.kept` in its events. **But nowplayd logged `on=false` at the reconnection, and `on=true` 153 ms later**: the broker sent the old connection's will when the new build took its client id over. `esp_mqtt_client_disconnect` only asks the client's task to disconnect, and the restart came first. |
| 2026-10-17 11:28 | 4 | Fixed (`esp_mqtt_client_stop` before the restart) and sent twice. The first restart, by the old build, sent the will again, and nowplayd went idle with the cover taken away; the second, by the fixed build `e5b302a`, did not: `on=true` only. |
| 2026-10-17 11:41 | 5 | `lp-stand-01` flashed by cable with `43c5c23` (the clean restart included), on `/dev/cu.usbserial-0001`; MAC checked against the screen's first, both being ESP32s. First try on WiFi, −39 dBm; the retained scene within a second. |
| 2026-10-17 11:42 | 5 | `make ota`: **no answer to the offer.** The stand had a power-on reset at the instant it was published (offer 11:42:31.87, boot ~11:42:31.7), and an offer is not retained. Not the firmware: the owner was handling the stand at that moment, and the next two offers were answered at once. A device that restarts while an offer is in flight never sees it, and `eds-ota` says so. |
| 2026-10-17 11:43 | 5 | `make ota` again: `kept` in 15 s, written in 10 s, into `ota_1`. Then once more, without touching the serial port: `kept` in 16 s, back into `ota_0`. |
| 2026-10-17 11:48 | 4 | `cover-screen-01`, a third update, to `563d113` with the clean restart: `kept` in 17 s. All three devices now restart cleanly after an update. |

### Departures from the plan

- The screen's update was tried before Step 3, as a demonstration, with only
  `ota-broker.env` installed on the workload. The services are not yet
  deployed; the beacon is not yet flashed.
- The beacon's restart was not clean in the build that went on by cable.
  Every device needs one update past `cb65a7a` (`e5b302a` or later) for its
  restarts to send no will; the beacon has had it, the screen and the stand
  have not.
- The test build was the screen's own firmware with its idle screen drawn as
  color bars, built from a scratch copy and not committed. It had to be the
  screen's firmware: anything else would not take the next update back.

## Follow-ups

- [x] The stand, when it is plugged in again (Step 5).
- [x] The cover screen, one more update, to a build with the clean restart.
- [ ] ADR-0034 and ADR-0035's evidence, into the records.

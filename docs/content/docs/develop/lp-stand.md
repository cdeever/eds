---
title: "Firmware: lp-stand"
weight: 1
---

# Firmware: lp-stand

ESP-IDF in C, following the conventions of `esp32-ma-bell-gateway`: `main/app/`,
`main/config/`, `main/hardware/`, `main/network/`, with a Makefile wrapper over
`idf.py`.

## The split that matters

Scene validation, effect rendering, gamma and event formatting are pure C with
no ESP-IDF dependency, because that is where a bug is subtle: it shows up only
as *the light looks wrong*, or, for an event that is not valid JSON, as nothing
at all. They are tested with a compiler and nothing else:

```bash
make test        # needs gcc; no ESP-IDF, no hardware
```

The ESP-IDF glue is thin by design and needs a device to trust.

## Toolchain

```bash
# EIM-style install (what this project is developed against)
. ~/.espressif/tools/activate_idf_v6.1.sh

# or the classic layout
. $IDF_PATH/export.sh
```

> [!WARNING]
> The ESP-IDF Installation Manager exposes `idf.py` as a **shell function**, not
> as something on `PATH`. A `make` recipe runs in a subshell that cannot see
> shell functions, so the Makefile resolves `$IDF_PATH/tools/idf.py` directly
> rather than calling `idf.py`. If you are wiring up new tooling, do the same.

## Build and flash

```bash
idf.py set-target esp32
make build
make flash PORT=/dev/cu.usbserial-0001    # macOS
make flash PORT=/dev/ttyUSB0              # Linux
make monitor PORT=/dev/cu.usbserial-0001
```

`make menuconfig` covers the stand identifier, topic prefix, strip length and
GPIO. It does **not** cover credentials — see
[Provisioning a Stand](../../guide/provisioning/).

## Seeing what a stand is doing

Once it is off the USB cable, a stand reports its key events — boot, network,
scenes, health — to `eds/log/<id>`, and the substrate carries them into the
tenant's log store:

```bash
make logs                    # the last hour
make logs ARGS=-f            # follow
make logs ARGS='--since 1d'
```

The same events are on the **LP Stand** dashboard in the tenant's Grafana
organization. To add one, call `lp_event_log_with()` with a dotted name and any
numbers as JSON fields; `event_format.c` is the pure half and has the tests.

## Board requirements

A 4 MB flash part is required, not merely recommended: the partition table's
two 1.5 MB OTA slots need 3.25 MB, and the ESP-IDF default of 2 MB fails the
build at partition-table generation. `sdkconfig.defaults` declares
`CONFIG_ESPTOOLPY_FLASHSIZE_4MB=y` for this reason.

Verified on an **ESP32-D0WD-V3 rev v3.1**, dual core at 240 MHz, 4 MB flash, no
PSRAM. Headroom on that part:

| | |
|---|---|
| App image | ~933 KB in a 1.5 MB slot — 41% free |
| DRAM | ~39 KB static, ~141 KB remaining |
| IRAM | ~69% used |

The absence of PSRAM is not a constraint here, and that is the architecture
paying off: because the stand renders named effects locally and never receives
frames, there is no image buffer to hold. A 60-LED frame is 180 bytes and a
scene is capped at 2 KB.

## ESP-IDF 6.x notes

The project targets IDF **6.0+**. Moving from 5.x required three changes, all
of which are already in the tree:

- `mqtt` and `json` were **removed from core IDF** in 6.0 and moved to the
  component registry. They are now managed dependencies, `espressif/mqtt` and
  `espressif/cjson`. Note the component name is `cjson`, not `json`.
- `espressif/led_strip` 2.5.x **does not compile against IDF 6.1** — its SPI
  backend lost a transitive heap header. The project uses `^3.0.3`, which is
  API-compatible with `led_strip_out.c` as written.
- Nothing in the firmware's own sources needed changing.

## Failure behaviour to preserve

Everything below is a deliberate choice. If you change one, change it knowingly:

- A **malformed or unparseable scene is ignored**, and the previous one keeps
  rendering.
- A scene with a **different contract version is refused**, not guessed at.
- An **unknown effect falls back to `solid`**.
- **WiFi never gives up.** An unattended stand that stops retrying is one that
  needs a person to walk over and power-cycle it.
- **The strip lights before the network exists** — a dim warm white — so an
  unconfigured or unreachable stand looks deliberate rather than broken.
- **A frame is skipped rather than stalled** if the render task cannot take the
  scene lock. A skipped frame is invisible; a stalled strip is not.
- **Last will on `status`**, so the room learns a stand died rather than merely
  went quiet.

A healthy first boot, unprovisioned, looks like this:

```
I (88)  boot:  5 creds            WiFi data        01 02 00320000 00004000
I (587) creds: not provisioned; ssid 'DVNTM-IOT', wifi key empty, ...
I (590) strip: 60 LEDs on GPIO 18
I (593) lp-stand: rendering at 16 ms/frame on 60 LEDs
W (30792) wifi: no address yet; continuing and retrying in the background
I (30815) lp-stand: subscribed; awaiting a retained scene on eds/lightstand/lp-stand-01/scene
```

The strip driver initialising and the render task starting **before** the
network is touched is the designed order, not a coincidence.

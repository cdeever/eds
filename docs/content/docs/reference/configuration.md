---
title: "Configuration"
weight: 4
---

# Configuration

## lightd

All environment, no config file: the broker credentials must never land on disk
in a repository, and the same binary has to work under a systemd unit or a
container without knowing which.

| Variable | Default | |
|---|---|---|
| `LIGHTD_ADDR` | `:8732` | listen address |
| `LIGHTD_PALETTE_URL` | `http://127.0.0.1:8731` | palette service |
| `LIGHTD_STAND_ID` | `lp-stand-01` | default stand |
| `LIGHTD_TOPIC_PREFIX` | `eds` | root of the topic tree |
| `LIGHTD_MQTT_URL` | `tcp://127.0.0.1:1883` | `tls://mqtt01…:8883` in production |
| `LIGHTD_MQTT_USERNAME` / `_PASSWORD` | — | broker credentials |
| `LIGHTD_MQTT_CA_FILE` | — | CA for the broker's certificate |
| `LIGHTD_MQTT_INSECURE` | `false` | bring-up only; never with a real cert |
| `LIGHTD_EFFECT` | `breathe` | default effect |
| `LIGHTD_SPEED` / `LIGHTD_BRIGHTNESS` | `0.4` / `0.8` | |
| `LIGHTD_SWATCHES` | `6` | swatches requested from palette |
| `LIGHTD_MIN_CHROMA` | `0.05` | below this a swatch is a grey |
| `LIGHTD_SATURATION` | `1.25` | chroma multiplier for the strip |
| `LIGHTD_MAX_COLORS` | `4` | palette entries per scene |

**Invalid values stop the daemon** rather than lighting the room wrong quietly.

## lp-stand

Build-time settings live in `idf.py menuconfig` under *LP jacket stand*:
stand identifier, topic prefix, strip length and GPIO.

**Credentials are not build-time.** WiFi and broker credentials are provisioned
into a dedicated `creds` NVS partition with `make provision` (or
`make wifi-config` by hand), so the firmware image carries no secret. The WiFi
key and the broker password are NVS-only, with no Kconfig option; the SSID,
broker URI and username keep Kconfig defaults for an unprovisioned stand. See [Provisioning a Stand](../../guide/provisioning/).

## Flash layout

| Partition | Type | Offset | Size |
|---|---|---|---|
| `nvs` | data/nvs | `0x9000` | 24K |
| `otadata` | data/ota | `0xf000` | 8K |
| `phy_init` | data/phy | `0x11000` | 4K |
| `ota_0` | app | `0x20000` | 1.5M |
| `ota_1` | app | `0x1A0000` | 1.5M |
| `creds` | data/nvs | `0x320000` | 16K |

Two OTA slots exist from the start because retrofitting them means
repartitioning a device that is glued to a shelf. The table needs 3.25 MB, so
the build requires a 4 MB flash part.

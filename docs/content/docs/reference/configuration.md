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
| `LIGHTD_COVER_TOPIC` | unset | a topic of cover images to light the stand from, relative to the prefix, e.g. `nowplaying/current/art` |
| `LIGHTD_LOOK_TOPIC` | unset | a topic that says which covers have a look of their own, e.g. `nowplaying/current/look`. Needs `LIGHTD_COVER_TOPIC` |
| `LIGHTD_SPECTRUM_EFFECT` | `sweep` | how the `spectrum` look moves: `sweep`, the rainbow round the strip, or `morph`, each colour in turn. Anything else stops the daemon |
| `LIGHTD_TOPIC_PREFIX` | `eds` | root of the topic tree |
| `LIGHTD_MQTT_URL` | `tcp://127.0.0.1:1883` | `tls://mqtt01…:8883` in production |
| `LIGHTD_MQTT_USERNAME` / `_PASSWORD` | — | broker credentials |
| `LIGHTD_MQTT_CA_FILE` | — | CA for the broker's certificate |
| `LIGHTD_MQTT_INSECURE` | `false` | bring-up only; never with a real cert |
| `LIGHTD_EFFECT` | `breathe` | default effect: `solid`, `breathe`, `sweep`, `morph` or `chase` |
| `LIGHTD_SPEED` / `LIGHTD_BRIGHTNESS` | `0.4` / `0.8` | |
| `LIGHTD_SWATCHES` | `6` | swatches requested from palette |
| `LIGHTD_MIN_CHROMA` | `0.05` | below this a swatch is a grey |
| `LIGHTD_SATURATION` | `1.25` | chroma multiplier for the strip |
| `LIGHTD_MAX_COLORS` | `4` | palette entries per scene |

**Invalid values stop the daemon** rather than lighting the room wrong quietly.

## nowplayd

Environment only, with the same fallbacks to the platform's `kit.env` names.
A value that cannot be used stops the program.

| Variable | Default | |
|---|---|---|
| `NP_MQTT_URL` | `tls://$MQTT_HOST:$MQTT_PORT`, else `tcp://127.0.0.1:1883` | broker |
| `NP_MQTT_CLIENT_ID` | `eds-nowplayd` | must be unique on the broker |
| `NP_MQTT_USERNAME` / `NP_MQTT_PASSWORD` | `$MQTT_USERNAME` / `$MQTT_PASSWORD` | `kit.env` carries lightd's login, so nowplayd names its own |
| `NP_MQTT_CA_FILE` | `$MQTT_CA_FILE` | the Deevnet Root CA |
| `NP_MQTT_INSECURE` | `false` | bring-up only |
| `NP_TOPIC_PREFIX` | `$DEEVNET_TENANT`, else `eds` | one topic level |
| `NP_CATALOGUES` | `deezer,itunes,musicbrainz` | where a missing album or cover is looked up, in order; empty switches lookups off |
| `NP_LOOKUP_TIMEOUT` | `10s` | for one catalogue request |
| `NP_CONTACT` | this repository's URL | goes in the User-Agent sent to catalogues |
| `NP_COVERLESS_SOURCES` | none | sources reported without a cover: none is published for them and none is looked up ([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)). `spotify` on the workload, which is what keeps the stand from following it |
| `NP_SOURCE_POWER` | none | `source=beacon` pairs: a source whose equipment has a beacon on its switch. While the beacon is connected that source is current; while it is not, the source is not heard; a beacon never heard from changes nothing ([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)). `turntable=turntable-power-01` on the workload |

## console

Environment only. On the workload the broker's settings come from `kit.env`;
a `CONSOLE_*` variable wins.

| Variable | Default | |
|---|---|---|
| `CONSOLE_ADDR` | `:8080` | where it listens; `:80` on the workload |
| `CONSOLE_MQTT_URL` | `tls://$MQTT_HOST:$MQTT_PORT`, else `tcp://127.0.0.1:1883` | broker |
| `CONSOLE_MQTT_CLIENT_ID` | `eds-console` | must be unique on the broker |
| `CONSOLE_MQTT_USERNAME` / `CONSOLE_MQTT_PASSWORD` | `$MQTT_USERNAME` / `$MQTT_PASSWORD` | the console names its own login |
| `CONSOLE_MQTT_CA_FILE` | `$MQTT_CA_FILE` | the Deevnet Root CA |
| `CONSOLE_MQTT_INSECURE` | `false` | bring-up only |
| `CONSOLE_TOPIC_PREFIX` | `$DEEVNET_TENANT`, else `eds` | one topic level |
| `CONSOLE_STRIP_LEDS` | `18` | how many LEDs the page draws for a stand, 1 to 300. The firmware's `LP_LED_COUNT`, which nothing on the bus reports |
| `CONSOLE_DASHBOARD_URL` | none | where the page's link to Grafana goes |
| `CONSOLE_DOCS_URL` | this site | where the page's link to these records goes |

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

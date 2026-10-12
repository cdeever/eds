# power-beacon

A board with nothing attached, on an outlet that is switched with something
that cannot say for itself whether it is on. The first one is on the
turntable's power strip.

```
 turntable on ──▶ strip powers its outlets ──▶ beacon boots ──▶ WiFi ──▶ broker
                                                                          │
                              eds/beacon/turntable-power-01/status  ◀─────┘
                              "online", retained; "offline" is its last will
```

The decision, and what else was considered, is
[ADR-0023](../../docs/content/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand.md).

## Status

Builds with ESP-IDF 6.1 for an ESP32-S2 and starts on one. Still to be
shown, and listed in the ADR: how long it takes to say `online`, how long
the broker takes to say `offline`, and a day on the outlet without its
connection coming and going.

## The board

Any ESP32 with WiFi and 4 MB of flash. The one in use is an ESP32-S2
(revision v0.0, single core) behind a CP2102N bridge. No pin is used. A
different chip is one line, `CONFIG_IDF_TARGET` in `sdkconfig.defaults`, and
`CHIP=` to make.

## How it works

Its connection is its message. On every connection it publishes `online`,
retained, at QoS 1. Its last will is `offline`, retained, on the same topic.
Nothing in the firmware runs when the power goes; the broker says it.

It subscribes to nothing and decides nothing. It does not know what shares
its outlet: the firmware is the same for anything on a switch, and its
name, with `nowplayd`'s `NP_SOURCE_POWER`, is all that says "turntable".

### How soon

- **On.** Boot, WiFi, an address, a TLS handshake, then the word. Nothing is
  started before the network, and WiFi power saving is off. The `uptime_ms`
  of the first `mqtt.connected` event after a `system.boot` is the measure.
- **Off.** The keepalive is ten seconds (`PB_MQTT_KEEPALIVE_S`), where the
  stand and the screen leave the library's two minutes. A broker sends a
  will after one and a half keepalives without a word: about fifteen
  seconds.

Shorter is not simply better. Each keepalive is a round trip over WiFi, and
one that is missed is a turntable reported off while a record plays.

### Failure behaviour

All of it is the stand's, copied.

- **WiFi never gives up**, retrying every two seconds.
- **The broker is retried every five seconds**, and `online` is said again
  on every connection: after a broker restart, or a will sent for a
  connection that was only slow, that is what puts the topic right.
- **An unprovisioned beacon starts**, says so on its serial port, and
  reaches nothing.
- **With no CA the TLS handshake fails**, and is not gone around.

What it cannot do: a beacon switched off while the broker is down leaves
`online` retained until it next connects.

## Host tests

```bash
make test
```

The event lines are the only part that is pure C, and their tests are the
stand's, copied with the code. Built with `-Wall -Wextra -Werror`. WiFi,
MQTT and the credentials need the board.

## Build, flash, provision

```bash
. ~/.espressif/tools/activate_idf_v6.1.sh
make build
make flash                    # PORT defaults to /dev/cu.usbserial-2120
make provision                # from the tenant's Terraform outputs
make monitor
```

`flash` and `provision` run `make chip` first, which stops unless the board
on the port is an ESP32-S2. The port is the one the cover screen takes when
it is the board plugged in.

`make provision` writes the `creds` partition: the WiFi key, the broker's
address, this beacon's account and the broker's CA, all read from the
tenant's outputs. It refuses if the account was not granted the two topics
the firmware publishes, because the broker would not say. The two secrets
have no Kconfig option and cannot be compiled in
([ADR-0004](../../docs/content/docs/architecture/decisions/0004-firmware-secrets-nvs-only.md)).

## Events

One JSON object each, to `eds/log/<id>`, QoS 1, not retained
([ADR-0005](../../docs/content/docs/architecture/decisions/0005-device-events-over-mqtt.md)):

| Event | When | Fields |
|---|---|---|
| `system.boot` | every start, which is every time the switch goes on | `reason`, `firmware`, `idf` |
| `wifi.connected` / `wifi.disconnected` | the network coming and going | `ssid`, `ip`, `rssi`, `attempts` / `reason` |
| `mqtt.connected` / `mqtt.disconnected` | the broker coming and going | `ip`, `rssi` |
| `beacon.health` | every five minutes | `rssi`, `heap_free`, `heap_min` |

More `mqtt.connected` than `system.boot` is a connection that came and
went, and each of those was up to fifteen seconds of the turntable being
taken for off.

## Shared with the stand

`config/pb_credentials.*`, `network/wifi/*`, `app/events/*`,
`partitions.csv` and `tools/provision_tenant.py` are copies of
`firmware/lp-stand`'s (the WiFi is the cover screen's copy, which has power
saving off), renamed. A fix to one is owed to the others.

# eds-ota

Sends a firmware build to one device through the broker
([ADR-0035](../../docs/content/docs/architecture/decisions/0035-firmware-is-updated-through-the-broker.md)).
Usually run for you by `make ota` in a firmware directory:

```bash
cd firmware/power-beacon
make build ota
```

```
offering power-beacon 1a2b3c4 (7371d4fde0058f9f, 895232 bytes) to beacon/turntable-power-01
    0%
   10%
  ...
  100%
kept: beacon/turntable-power-01 runs 1a2b3c4 (7371d4fde0058f9f), 41s after the offer
```

## Why through the broker

The devices are on the IoT segment, and what they can reach of EdS is the
broker. Nothing of EdS's that serves HTTP is reachable from there, and the
tenant's network accepts nothing inbound, so the usual `esp_https_ota` would
need a new network path. The broker is already verified by each device and
already says who may publish what to it.

## Where it runs

On the services workload, which reaches the broker as lightd does. `make
send` (and so `make ota`) builds it for linux/amd64, copies it there, and
runs it as root with the services' broker settings (`/opt/eds/kit.env`) and
its own login (`/opt/eds/ota-broker.env`, written by `make -C deploy units`
from the tenant's `ota_broker` output). The image goes over the SSH
connection's standard input.

The `ota` broker account may offer a build to each device the tenant's
`main.tf` names, and read their answers, and nothing else.

## What it does

1. Reads the image's own description (`esp_app_desc_t`): its project, its
   version, and its ELF's hash. A build for another kind of device is not
   sent: `lightstand` runs `lp-stand`, `screen` runs `cover-screen`,
   `beacon` runs `power-beacon`.
2. Subscribes to the device's `…/ota/state`, then offers the build on
   `…/ota`: `{"v":1,"project":…,"size":…,"elf":…,"version":…}`.
3. Sends what the device asks for, on `…/ota/chunk`: four bytes of offset,
   big-endian, and up to 8 KB of the image. The device answers every piece
   with the next byte it wants; a piece not answered in ten seconds is sent
   again, six times at most.
4. After `rebooting`, waits up to three minutes for `running`. The new ELF
   hash is **kept**; any other is **undone**: the device went back to the
   build before, because the new one did not reach the broker.

Every message is QoS 1 and none is retained.

## Development

```bash
make check              # gofmt, vet, unit tests
make test-integration   # and a build through the dev broker (nowplaying's)
```

The unit tests send builds to a stand-in that behaves as the firmware's
receiver does, with pieces lost, answers lost and pieces delivered twice.
The firmware's own half is host tested in each firmware (`app/ota`).

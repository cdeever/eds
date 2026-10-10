---
title: "Provisioning a Stand"
weight: 2
---

# Provisioning a Stand

Credentials live in a dedicated `creds` NVS partition, not in the firmware
image. The WiFi key and the broker password have no build-time option at all:
NVS is the only place they can come from. A built binary therefore carries no
secret and can be cached, copied and shared without care, and `sdkconfig`
cannot hold one.

## From the tenant

A stand on the Deevnet substrate is flashed with what the EdS tenant issued:

```bash
cd firmware/lp-stand
. ~/.espressif/tools/activate_idf_v6.1.sh
make provision PORT=/dev/cu.usbserial-0001
```

This reads the tenant's Terraform outputs and writes the WiFi key, the broker
address, the stand's own broker account and the Deevnet Root CA. Nothing is
typed. It needs `infra/deevnet-tenant-eds` able to read its state, and it
refuses to write if the account was not granted the topics the firmware uses —
the broker drops an ungranted publish without saying so.

## By hand

For a stand that is not on the substrate — a local broker on the bench:

```bash
make wifi-config PORT=/dev/cu.usbserial-0001
```

You are prompted for the WiFi SSID and password, the broker URI, and the broker
username and password. **Both password prompts are silent** — nothing echoes.
Leaving a field blank omits it. For the SSID, broker URI and username the
firmware then uses the `menuconfig` default; a password left blank is simply
absent, because it has no other source.

Reset the board afterwards. A provisioned stand logs:

```
I (587) creds: provisioned from creds; ssid 'MyNetwork', wifi key set, mqtt user 'lp-stand-01', mqtt key set, broker CA provisioned
```

An unprovisioned one says `not provisioned` instead. Either way it
boots — a stand that refuses to start without credentials is harder to diagnose
than one that starts and cannot associate.

## Why it is prompted rather than passed

Credentials are read interactively rather than taken from `make` variables or
the environment: a variable on the command line is visible in the process list
and lands in shell history. The generated NVS image is written to a temporary
directory outside the repository with a restrictive umask and removed on exit.

## What is logged

The firmware reports *whether* a key is set, never the key. That is the only
question worth answering from a console when a stand will not associate, and
the console is the first place a secret leaks.

## Reprovisioning

Run `make provision` or `make wifi-config` again. Either rewrites only the `creds` partition — the
application, the OTA slots and the WiFi driver's own stored state are all
untouched, so there is no need to rebuild or reflash the firmware.

---
title: "EdS on One Pi"
weight: 4
---

# EdS on One Pi

Everything EdS needs can be put on one Raspberry Pi on the home network: the
broker, the log store, Grafana, the four services and the now-playing agent.
Nothing of Deevnet IoTaaS is behind it.

> **Not yet proven.** The image is defined and has not run on a Pi.
> [ADR-0033](/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi/)
> is `Proposed` and lists what would settle it.

```
stand, screen, beacon ──TLS 8883──▶ ┌ Pi ────────────────────────────────┐
iTunes PC ◀── remote protocol ────▶ │ Mosquitto · log store · Grafana    │
a browser ──────── 443 ───────────▶ │ palette · lightd · nowplayd        │
turntable ── tape out, sound card ─▶│ console · npagent                  │
                                    └────────────────────────────────────┘
```

It is beside the tenant on Deevnet IoTaaS, not in place of it. Building the
image and running the Pi change nothing there, and the stand, the screen and
the beacon stay with the tenant until each is provisioned again.

## What it is made of

The image is Deevnet IoTaaS's take-home image, `pi-backend`, with EdS added.
The take-home image keeps the tenant's contract on a Pi: the same ports, the
same topic prefix, the same log partitions and the same Grafana data sources.
A program on it, `deevnet-kit`, makes the card's own CA and tokens the first
time it boots.

EdS adds its four services and the agent, each as a unit under its own user,
and one more step on first boot: it makes the tenant's eight broker accounts,
each with a password made on the card. No secret is in the image, and none
comes from the tenant.

## What you need

- A Raspberry Pi 4 or 5 with 4 GB or more, and a card of 8 GB or more.
- A reserved address for it on your router. The devices are given that
  address, and the card's certificate names it.
- The sound card the turntable driver listens to, if the turntable is to be
  followed.

## Doing it

The steps, in order, are in the repository's `deploy/backend/README.md`:
build the image, flash it, write the card's settings, boot it, check it, pair
iTunes and authorize Spotify. Each is a `make` target there.

Moving the stand, the screen and the beacon over is the last step, and is
done with the same provisioning tool as on Deevnet IoTaaS
([Provisioning a Stand](../provisioning/)), given what the card made in place
of the tenant's outputs. The firmware is not rebuilt.

## What is different from Deevnet IoTaaS

| On Deevnet IoTaaS | On the Pi |
|---|---|
| The broker is the site's, `mqtt.mobile.deevnet.net` | The broker is the Pi, at its address |
| Certificates chain to the Deevnet Root CA | Everything chains to the card's own CA, which each computer has to be told to trust |
| Services are containers on a workload | Services are units on the Pi |
| Accounts are Terraform | Accounts are a list in the image, checked against the Terraform |
| The Pi that runs the agent is reached through the Builder | The Pi is reached directly, as your own user |
| Dashboards are applied with the tenant | Dashboards are not carried over yet |

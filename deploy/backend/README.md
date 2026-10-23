# deploy/backend

The backend Pi: the whole of EdS on one Raspberry Pi, with nothing of Deevnet
IoTaaS behind it
([ADR-0033](../../docs/content/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi.md)).

```
home LAN
stand, screen, beacon ──TLS 8883──▶ ┌ Pi ────────────────────────────────┐
iTunes PC ◀── remote protocol ────▶ │ Mosquitto · log store · Grafana    │
a browser ──────── 443 ───────────▶ │ palette · lightd · nowplayd        │
turntable ── tape out, sound card ─▶│ console · npagent                  │
                                    └────────────────────────────────────┘
```

The image is EdS's own, [`images/backend`](../../images): the take-home image
Deevnet IoTaaS publishes, `pi-backend`, with EdS's services and its agent
added. This directory is the rest: getting it built, writing a flashed card's
settings, and looking after the Pi.

**It is beside the tenant, not in place of it.** Building this image and
running this Pi change nothing on the tenant, its workload or the relay Pi.
The stand, the screen and the beacon stay with the tenant until they are
provisioned again, which is the last step and the owner's choice.

**Not yet proven.** ADR-0033 is `Proposed`, and lists what would settle it.

## Before the first build

The build takes the image Deevnet IoTaaS publishes, and refuses one whose
`deevnet-kit` is too old to take a hostname from the card or keep a password
from a file. The one published on 2026-09-24 is. Rebuilding and publishing it
is Deevnet IoTaaS's, on the Builder:

```bash
make -C /srv/dvnt/deevnet-provisioning-api stage-pi
make -C /srv/dvnt/deevnet-image-factory pi-backend pi-backend-publish
```

## A new card

A Pi 4 or 5, with 4 GB or more, and a card of 8 GB or more.

```bash
make image     # cross-compile everything and build the image on the Builder
make fetch     # copy it here and check its checksum
```

Flash `eds-pi-backend.img.xz` with Raspberry Pi Imager, as a custom image.
**Apply no customization.** Leave the card in.

```bash
make card      # asks for the Wi-Fi key and a password for your login
```

It writes the card's name and Wi-Fi for `deevnet-kit`, your login, and a
folder `eds/` with your public key and your Discogs settings
(`deploy/discogs.env`, if it is there). `PI_HOSTNAME=` and `PI_USER=` name the
card and the login; the defaults are `eds-backend` and `eds`.

Eject the card, put it in the Pi, and power it on. The first boot makes your
login and restarts once; the second makes the card's CA, its tokens and its
broker accounts. Allow a few minutes.

```bash
make status    # every check should say ok; a driver not yet set up is noted
make pair      # prints a four-digit code; type it into iTunes
make spotify   # a browser opens here for Agree; the authorization goes to the Pi
make turntable DEVICE=plughw:CODEC
make drivers DRIVERS=itunes,spotify,turntable
```

`make spotify` reads the app's Client ID and the devices that count from
`deploy/relay/spotify.conf`. It makes this Pi an authorization of its own and
never copies the relay's.

Give the Pi a reserved address on the router before going further. The
devices are given that address, and the card's certificate names it.

On the Pi, `sudo deevnet-kit status` and `sudo deevnet-kit selftest` are the
thorough look at the broker, the log store and Grafana.

## The console

`https://<hostname>.local/`. It presents the card's own certificate, so a
browser warns until the card's CA is trusted on that computer:

```bash
make devices   # also fetches deevnet-kit-ca.pem
```

On macOS, open `deevnet-kit-ca.pem` in Keychain Access and trust it for SSL.
It is one card's CA: a card flashed again has another.

## Moving the devices over

Last, and only when the Pi has been running as it should for as long as you
want to watch it. A device provisioned for the Pi has left the tenant.

```bash
make devices   # asks which Wi-Fi the devices join; writes devices.json
```

`devices.json` and `deevnet-kit-ca.pem` are what the firmware's provisioning
tools read in place of the tenant's Terraform outputs. Both are gitignored;
the first holds passwords and the Wi-Fi key. With ESP-IDF activated and a
device on its cable, in its firmware directory:

```bash
python3 tools/provision_tenant.py -p <port> \
    --outputs-json ../../deploy/backend/devices.json \
    --ca ../../deploy/backend/deevnet-kit-ca.pem
```

The beacon first: nothing is attached to it. The firmware is not rebuilt; the
topics and the account names are the tenant's.

**Not known yet:** whether the firmware accepts a certificate matched by an
address. If a device will not connect, `make devices BROKER_HOST=<name>`
gives it a name to dial instead: the name your router has for the Pi, which
the certificate also carries.

To move a device back, provision it from the tenant as before
(`make provision` in its directory).

## Afterward

```bash
make push      # update the services, the agent, their units and tools in place
make logs      # follow the services and the agent
```

`make push` does not change the firewall, the kernel settings or the broker's
configuration; those change with an image.

If the Pi's address changes: `sudo deevnet-kit regen-certs`, then
`sudo systemctl restart eds-console`, and provision the devices again with
the new address.

## What is where on the Pi

| | |
|---|---|
| `/etc/eds/accounts.conf` | the eight broker accounts, as the tenant has them |
| `/etc/eds/accounts/` | each account's password, made on the card; root only |
| `/etc/eds/*.env` | each service's broker login; written by `eds-backend-setup` |
| `/etc/eds/settings/` | the settings that are not secret: `deploy/*.env`, as built |
| `/etc/deevnet-kit/` | the card's CA, certificate and tokens; `deevnet-kit`'s |
| `/etc/eds-backend-release` | what this image is, and each push since |

## Not done yet

- **Dashboards.** They are in the tenant's Terraform root and are not applied
  to this Pi's Grafana. The console's link to one is left off.
- **One script for both Pis.** `spotify.sh` here is a copy of the relay's with
  the hop through the Builder removed, and the backend's playbook has its own
  copy of the agent's tasks. They are folded together once this is proven.

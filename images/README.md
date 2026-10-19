# images

EdS's Raspberry Pi images.

An EdS image is **Deevnet IoTaaS's base image with an EdS layer applied**
([ADR-0015](../docs/content/docs/architecture/decisions/0015-eds-builds-its-own-pi-images.md)).
Deevnet IoTaaS's image factory builds the base: Raspberry Pi OS with the site's
automation user. Everything after that is EdS's and lives here, one directory
per image. Nothing of EdS's is in the factory.

The `backend` image has a different base: the take-home image Deevnet IoTaaS
publishes, `pi-backend`, taken as the file it offers
([ADR-0033](../docs/content/docs/architecture/decisions/0033-the-whole-back-end-on-one-pi.md)).

| Image | What it is |
|---|---|
| [`agent`](agent) | a Pi that runs the now-playing agent near the players, and reports to the broker |
| [`backend`](backend) | the whole of EdS on one Pi: broker, log store, Grafana, the four services and the agent |

More may follow the same shape. The Pi that identifies what a turntable is
playing, once expected to be the second image, is this one: listening is a
driver in the same agent, and the image carries what it needs
([ADR-0021](../docs/content/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening.md)).

## Building

Builds need Linux, sudo and loop devices, so they run on the Builder
([why, and what would change that](#why-builds-run-on-the-builder-and-what-would-change-that)):

```bash
make -C deploy/relay image    # copies this directory there and runs build.sh agent
make -C deploy/relay fetch    # brings the image back, and checks its checksum
```

`build.sh <image>` on the Builder itself does the work: it builds the base with
the factory's own definition (and keeps it; `BASE=fresh` rebuilds it), copies
it, grows it to 8 GB, applies `<image>/config.yml` to it offline, and
compresses it.

For `backend` the commands are `make -C deploy/backend image fetch`, and
`build.sh` takes the published take-home image in place of building a base:
it checks the file against the checksum beside it and unpacks it.

## The agent Pi

| | |
|---|---|
| `eth0` | the Deevnet site, by DHCP. The default route, DNS and SSH are here |
| `wlan0` | unused, unless the card is given a second leg; see below |
| Routing | none. `ip_forward` is 0 and the firewall's forward chain drops everything |
| Firewall | nftables, loaded before any interface comes up. In on `eth0`: SSH. In on `wlan0`: Bonjour and the pairing port. Nothing else |
| Agent | `npagent`, as its own unprivileged user, sandboxed by its unit |
| Users | the base image's automation user; `npagent`, with no login |
| Clock | UTC |
| Identity | `/etc/eds-agent-release` |

**The image carries no secret and no name.** One image serves every agent Pi.
What makes a card *this* card is written to its boot partition after it is
flashed, in a folder `eds-agent/`, imported at the next boot and deleted:

| File | |
|---|---|
| `hostname` | what the card calls itself |
| `npagent.env` | the agent's broker account, which drivers it runs, and the device name it reports its events under |
| `deevnet-root-ca.pem` | the CA the broker's certificate is verified against |
| `wifi.conf` | optional: a second network to join |

The build fails if it finds a secret in the image.

### A relay is an agent Pi with a second leg

The agent has to be where it can reach both its players and the broker. When
those are on one network, the Pi needs only its wired leg. When they are not —
the players at home, the broker in a rack that travels — a card is given
`wifi.conf`, and its Wi-Fi joins the players' network
([ADR-0010](../docs/content/docs/architecture/decisions/0010-home-lan-relay.md)).
That is a relay: a development arrangement, not a different image.

The second leg is held to a subnet. It takes no default route, no DNS and no
routes from DHCP; nothing is forwarded; and from that side the Pi admits
Bonjour and one pairing port, not SSH. `eds-agent-status` checks all of it on
the running Pi.

### On the Pi

```
eds-agent-status     each leg, the broker, each driver, the agent; non-zero if anything is wrong
eds-agent-pair       pair with iTunes, once; prints a code to type in
eds-agent-spotify    install a Spotify authorization or the devices that count; list the account's devices
eds-recognize        name the track in a WAV file on standard input; what the turntable driver asks
eds-agent-turntable-install   (root) install the recogniser again, from its pinned requirements
journalctl -u npagent -f
```

## The backend Pi

Everything EdS needs, on one Pi on the home network, with nothing of Deevnet
IoTaaS behind it. **Not yet proven**: ADR-0033 is `Proposed`.

| | |
|---|---|
| From the base | Mosquitto (8883), the log store (8427), the bridge from `eds/log/<device>` into it, Grafana (3000), and `deevnet-kit`, which manages them |
| Added | `palette`, `lightd`, `nowplayd` and the console, each a unit under its own user; `npagent`, its tools and the recognizer, as on the agent Pi |
| Network | one leg, a cable or Wi-Fi. No routing: `ip_forward` is 0 and the firewall's forward chain drops everything |
| Firewall | nftables, loaded before any interface comes up. In: SSH, 8883, 443 and 80, 3000, 8427, Bonjour and the pairing port. `palette` and `lightd`'s own ports are not opened |
| Console | 443, with the card's one server certificate, which names `<hostname>.local` |
| Users | the owner's, made on first boot from the card. No automation user: the base removes it |
| Clock | UTC |
| Identity | `/etc/eds-backend-release` |

**The image carries no secret and no name**, as the agent's does not. But
where an agent card is handed its broker account, a backend card makes its
own:

1. `deevnet-kit` runs once on first boot. It reads the tenant, the card's
   name and its Wi-Fi from `deevnet-kit.txt` on the boot partition, and makes
   the card's CA, its certificate and its log tokens.
2. `eds-backend-setup` runs after it, and on every boot. It adds the eight
   broker accounts in `/etc/eds/accounts.conf`, each with a password it makes
   there, and writes each service the login it reads. It does only what is
   missing.

`backend/files/accounts.conf` is a copy of the accounts in
`infra/deevnet-tenant-eds/main.tf`. `make -C deploy/backend check` reads both
and fails when they differ; the image is not built until it passes.

The services' settings that are not secret are not copied: the build takes
`deploy/lightd.env`, `deploy/nowplayd.env` and `deploy/console.env` as they
are.

The build fails if the base's `deevnet-kit` is too old for the card, if it
finds a secret in the image, or if the site's automation user is on it.

### On the Pi

```
eds-backend-status   each service, the broker's accounts, each driver; non-zero if anything is wrong
eds-backend-setup    (root) make any account or settings file that is missing
eds-backend-setup devices   (root) what the devices are provisioned with, as JSON
eds-agent-pair       pair with iTunes, once; prints a code to type in
eds-agent-spotify    install a Spotify authorization or the devices that count
deevnet-kit status   the broker, the log store and Grafana; `selftest` proves them end to end
```

[`deploy/backend`](../deploy/backend) is how a card is written and the Pi
looked after.

## Why builds run on the Builder, and what would change that

The image is defined entirely here. The Builder is not where it lives; it is
where three things happen to be:

| What | Why the Builder | Whose concern |
|---|---|---|
| A Linux machine to run `build.sh` on | attaching an image as a disk, mounting its filesystem and running commands inside it needs Linux and sudo; macOS cannot mount the filesystem | convenience only |
| The base image | it is built by the image factory's definition, and the factory's checkout is there. `build.sh` reaches into it by path: its Packer file, its container, the automation user's key | Deevnet IoTaaS's |
| The way in to a Pi | a Pi admits SSH on its site side only, to the automation user, and from a desk the Builder is the hop | the site's |

Only the first is an accident of where a script runs. The other two are about
what belongs to Deevnet IoTaaS and how the site is administered, and would
stay whatever built the image.

The `backend` image already has the cleaner arrangement for its base: it
takes a published file, not the factory's checkout.

### Future enhancement: build anywhere

Not done, and not needed yet. Two changes would let an EdS image be built from
a developer's machine alone:

1. **The factory publishes its base image** as an artifact, beside the images
   it already publishes. `build.sh` would download a named, checksummed base
   instead of building one from the factory's checkout. That turns today's
   dependency on the factory's *layout* into a dependency on something it
   *offers*, and it is the half that matters: a change to the factory's
   internals could break EdS's build today without anyone meaning it to.
2. **`build.sh` runs in a Linux container.** On an ARM Mac that would also
   drop the emulation the Builder needs, since the Mac and the Pi share an
   architecture.

The first is a change to Deevnet IoTaaS and wants its own record there. Until
then the seam is noted in
[ADR-0015](../docs/content/docs/architecture/decisions/0015-eds-builds-its-own-pi-images.md)
and in [CHG-0002](../docs/content/docs/changes/2026/0002-relay-pi-and-now-playing-accounts.md)'s
follow-ups.

## Adding an image

A directory with a `config.yml` that takes `chroot_root`, and a `files/`
beside it. Hold to the same rules: nothing secret and nothing particular to
one Pi in the image, and a check in the playbook for anything that would be
dangerous to get wrong.

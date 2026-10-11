# images

EdS's Raspberry Pi images.

An EdS image is **the substrate's base image with an EdS layer applied**
([ADR-0015](../docs/content/docs/architecture/decisions/0015-eds-builds-its-own-pi-images.md)).
The substrate's image factory builds the base: Raspberry Pi OS with the site's
automation user. Everything after that is EdS's and lives here, one directory
per image. Nothing of EdS's is in the factory.

| Image | What it is |
|---|---|
| [`agent`](agent) | a Pi that runs the now-playing agent near the players, and reports to the broker |

More will follow the same shape — a Pi that identifies what a turntable is
playing from its line-level output, for one.

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
| `npagent.env` | the agent's broker account and source id |
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
eds-agent-status     each leg, the broker, the player, the agent; non-zero if anything is wrong
eds-agent-pair       pair with iTunes, once; prints a code to type in
journalctl -u npagent -f
```

## Why builds run on the Builder, and what would change that

The image is defined entirely here. The Builder is not where it lives; it is
where three things happen to be:

| What | Why the Builder | Whose concern |
|---|---|---|
| A Linux machine to run `build.sh` on | attaching an image as a disk, mounting its filesystem and running commands inside it needs Linux and sudo; macOS cannot mount the filesystem | convenience only |
| The base image | it is built by the image factory's definition, and the factory's checkout is there. `build.sh` reaches into it by path: its Packer file, its container, the automation user's key | the substrate's |
| The way in to a Pi | a Pi admits SSH on its site side only, to the automation user, and from a desk the Builder is the hop | the site's |

Only the first is an accident of where a script runs. The other two are about
what belongs to the substrate and how the site is administered, and would
stay whatever built the image.

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

The first is a change to the substrate and wants its own record there. Until
then the seam is noted in
[ADR-0015](../docs/content/docs/architecture/decisions/0015-eds-builds-its-own-pi-images.md)
and in [CHG-0002](../docs/content/docs/changes/2026/0002-relay-pi-and-now-playing-accounts.md)'s
follow-ups.

## Adding an image

A directory with a `config.yml` that takes `chroot_root`, and a `files/`
beside it. Hold to the same rules: nothing secret and nothing particular to
one Pi in the image, and a check in the playbook for anything that would be
dangerous to get wrong.

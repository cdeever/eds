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

Builds need Linux, sudo and loop devices, so they run on the Builder:

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

## Adding an image

A directory with a `config.yml` that takes `chroot_root`, and a `files/`
beside it. Hold to the same rules: nothing secret and nothing particular to
one Pi in the image, and a check in the playbook for anything that would be
dangerous to get wrong.

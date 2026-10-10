# deploy/relay

The relay: an agent Pi with a second leg. Its Ethernet is on the Deevnet site
and its Wi-Fi is on the home LAN, and it runs the now-playing agent
([ADR-0010](../../docs/content/docs/architecture/decisions/0010-home-lan-relay.md)).
It is how what plays in iTunes on a home PC reaches a broker the home network
cannot reach.

```
home LAN                              Deevnet Mobile
iTunes PC ◀── remote protocol ──▶ wlan0 [ Pi: npagent ] eth0 ──▶ broker
                                        no route between
```

The image is EdS's own, [`images/agent`](../../images), and it is not a relay
image: one image serves every agent Pi. What makes a card a relay is the Wi-Fi
network this directory gives it. This is a development arrangement, for as
long as the players are at home and the broker is in a rack that travels.

## A new card

```bash
make image     # cross-compile the agent and build the image on the Builder
make fetch     # copy it here and check its checksum
```

Flash `eds-pi-agent.img.xz` to a microSD with Raspberry Pi Imager. **Apply no
customization**: the image needs none. Leave the card in.

```bash
make card      # asks for the home Wi-Fi key; BOOT=/Volumes/<name> if not bootfs
```

Eject the card, put it in `dv02rpi002p01`, connect its Ethernet to the IoT
segment, power it on, and give it a minute.

```bash
make status    # every check should say ok, except "not paired"
make pair      # prints a four-digit code; type it into iTunes
make status    # now all of it
make logs      # the agent, as it publishes
```

## Nothing secret is in the image

The image file can sit on the Builder, be copied and be kept. This card's
name, the home Wi-Fi key and the agent's broker account are written to the
**card's** boot partition by `make card`, and the Pi moves them into place at
its next boot and deletes them from the card.

- The Wi-Fi key is asked for, never stored on this machine, and never passed
  on a command line.
- The broker account goes straight from the tenant's Terraform state to the
  card.
- `card.sh` refuses to write a card whose account was not granted the topics
  the agent uses, because the broker would not say so.

To change a setting later — a new Wi-Fi key, a reissued account — put the card
back in the computer and run `make card` again.

## It is not a router

A host on two networks that are otherwise kept apart is only acceptable
because nothing crosses at the network layer. The one thing that passes from
the house to the site is an MQTT publish the agent makes.

- Forwarding is off in the kernel, and the firewall drops forwarded packets.
- The Wi-Fi leg has no default route, takes no DNS and takes no routes from
  DHCP: the Pi reaches the home subnet it is on and nothing beyond.
- From the home side the Pi admits Bonjour and one pairing port. No SSH.
- `make status` checks all of this on the running Pi and fails if any is off.

## Reaching the Pi

SSH is admitted on the wired side only, to the site's automation user, so
`make status`, `pair` and `logs` go through the Builder.

## When no Remote button appears

iTunes offers to pair only when it sees the agent advertise itself over
Bonjour. Two things have stopped that, one at each end:

- **On the PC.** Under *Allow an app through Windows Firewall*, **Bonjour
  Service** and **iTunes** must be ticked for the network type the PC is on —
  *Private*, usually. From another machine on that network,
  `dig @224.0.0.251 -p 5353 -x <the PC's address>` answers with the PC's name
  when Bonjour is getting through.
- **On the Pi.** `systemctl status avahi-daemon`. The pairing tool now says so
  itself if its advertisement stops.

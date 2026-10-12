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
make status    # every check should say ok; a driver not yet set up is noted
make pair      # prints a four-digit code; type it into iTunes
make spotify   # a browser opens here for Agree; the authorization goes to the Pi
make status    # now all of it
make logs      # the agent, as it publishes
```

## Spotify

`make spotify` needs `spotify.conf` in this directory, which it writes a
template of the first time and which is **not committed**:

```
SPOTIFY_CLIENT_ID=<the Client ID of the app at developer.spotify.com>
SPOTIFY_DEVICES=<the devices that count, as Spotify names them, with commas>
```

The app must be owned by an account with Premium, use the Web API, and have
the redirect URI `http://127.0.0.1:8898/callback` exactly. Its client secret
is not used.

The authorization is made here, where there is a browser, and **moved** to
the Pi: Spotify may replace the token whenever it is used, so there can be
one holder. With no devices named yet, the Pi prints the account's devices
(Spotify lists only those with Spotify open); name the ones in the room and
run it again. `make spotify AGAIN=1` authorizes afresh, which is needed every
six months.

### Turning it off, and on

```bash
make drivers DRIVERS=itunes           # Spotify off
make drivers DRIVERS=itunes,spotify   # and on again
```

It changes one line of the Pi's settings and restarts the agent. A driver
that is off asks its player nothing and reads as offline on the bus. Its
authorization stays on the Pi for when it is turned back on. **Spotify is
off at present** (CHG-0011); the Pi runs `itunes,turntable` (CHG-0013).

## The turntable

The turntable driver listens to a sound card plugged into the Pi, fed from
the amplifier's tape output
([ADR-0021](../../docs/content/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening.md)).
Which input is the Pi's to be told:

```bash
make turntable DEVICE=plughw:UA4FX       # which input; arecord -L on the Pi lists them
make drivers DRIVERS=itunes,turntable    # and the driver on
make drivers DRIVERS=itunes              # and off: nothing is listened to, nothing is asked
```

A card for such a Pi is written with both:
`make card DRIVERS=itunes,turntable TURNTABLE_DEVICE=plughw:UA4FX`.

The input must be line level and after the phono stage. Set the interface so
its peaks stay well below full scale; the driver takes anything under
-55 dBFS for silence. `make status` checks the input is plugged in, the
recogniser installed and its endpoint in reach.

## Changing a Pi in service

```bash
make push      # the agent, its unit and its tools, onto the running Pi
make image fetch   # afterwards, so the next card flashed is not older than the Pi
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
  the agent uses, because the broker would not say so. The one topic the
  agent listens on, its commands (ADR-0020), only warns: the agent reports
  what is playing without it.

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

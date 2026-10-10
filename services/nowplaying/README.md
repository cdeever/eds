# nowplaying

What is playing, on the bus.

Small **drivers** each watch one player and say what it is doing. **nowplayd**
reads all of them, picks the one current track, fills in the album and the
cover where the source had none, and publishes the result for whatever wants
to react. The first thing that does is `lightd`, which lights the stand from
the cover.

```
drivers ──▶ eds/nowplaying/source/<id>/… ──▶ nowplayd ──▶ eds/nowplaying/current ──▶ subscribers
 one per player         (retained)          arbitrate       + …/current/art          lightd, …
                                            + enrich          (retained)
```

The design, and what was turned down, is recorded:
[ADR-0006](../../docs/content/docs/architecture/decisions/0006-now-playing-bus-of-drivers.md)
(a bus of small drivers),
[ADR-0007](../../docs/content/docs/architecture/decisions/0007-drivers-are-portable-go.md)
(portable Go),
[ADR-0008](../../docs/content/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches.md)
(arbitration and enrichment). The contract is on the docs site under
*Reference → Now Playing*.

## Status

**The iTunes driver is deployed and reporting. nowplayd is built and tested, and not yet deployed.**

| | |
|---|---|
| `contract`, `arbiter`, `resolve`, `current`, `config` | unit-tested, no network |
| `bus` | tested against a real broker with `-race` (`make test-integration`) |
| `nowplayd` | run locally end to end: a source state published by hand became a current track with a looked-up album and cover, and `lightd` lit a scene from it |
| the three catalogues | each checked once against the live service on 2026-10-10 |
| `dacp`, `driver/itunes` | unit-tested, including a replay of a session recorded from the real iTunes |
| `npagent` | paired with iTunes 12.13.10.3 on Windows and run end to end on a development broker: iTunes → driver → nowplayd → lightd → scene ([ADR-0011](../../docs/content/docs/architecture/decisions/0011-itunes-by-remote-pairing.md)) |
| the agent Pi | running as a relay on `dv02rpi002p01`: paired with iTunes on the home LAN and publishing to the real broker ([`images`](../../images), [`deploy/relay`](../../deploy/relay)) |
| broker accounts | declared and checked against the real broker (`eds-nowplayd`, `eds-np-relay`) |
| `nowplayd` on the workload | not deployed. Until it is, nothing acts on what a driver reports |

## The iTunes driver

`npagent` pairs with iTunes, or the Music app, the way a phone remote does: a
four-digit code typed into the player once. No Apple ID and no Home Sharing.
It is one more remote beside any already paired.

```bash
bin/npagent pair     # advertises a remote called EdS; type the code it prints into iTunes
bin/npagent watch    # print every change the player pushes; publishes nothing
bin/npagent run      # the same, published to the bus
```

The player pushes changes: the driver holds a request open and iTunes answers
it when the track changes or playback pauses. The cover comes from the
player's own library.

- iTunes answers several times for one action. The driver publishes a change
  once.
- A new cover is fetched when the album changes, not for each track on it.
- The driver is online when it has a session with its player, not merely when
  it is running, and says so on its status topic.
- After five minutes of silence it asks the player outright, which is how a
  sleeping PC is told from a quiet evening.
- A player that no longer knows the pairing is not retried: someone has to
  pair again.

Settings are in `npagent`'s usage text (`bin/npagent`).

## Running it

```bash
make check             # gofmt, vet, unit tests
make broker            # a development mosquitto on tcp://127.0.0.1:21883
make test-integration  # everything, with -race, against that broker
make run               # nowplayd on the development broker, under edsdev/
make broker-watch      # every edsdev/nowplaying topic; covers shown by size
```

With `make run` and `make broker-watch` going, play the part of a driver:

```bash
mosquitto_pub -h 127.0.0.1 -p 21883 -r -t edsdev/nowplaying/source/dev/status -m online
mosquitto_pub -h 127.0.0.1 -p 21883 -r -t edsdev/nowplaying/source/dev/state -m \
  '{"v":1,"source":"dev","player":"tidal","state":"playing",
    "track":{"title":"So What","artist":"Miles Davis"},"changed_at":"2026-10-10T16:00:00Z"}'
```

`edsdev/nowplaying/current` appears with the album filled in and an `art`
block, and the cover itself on `edsdev/nowplaying/current/art`.

To see the stand's side of it, run `palette` and `lightd` beside it with
`LIGHTD_TOPIC_PREFIX=edsdev` and `LIGHTD_COVER_TOPIC=nowplaying/current/art`,
and watch `edsdev/lightstand/+/scene`.

## How it decides

**Most recent wins.** The source whose track most recently started or changed
is current. When it pauses, stops or goes offline, another source that is
still playing takes over. With nothing playing, the most recent track stays
current, paused: "paused on B" is more use than silence.

- A source that only repeats itself — a driver reporting progress — does not
  become more recent by talking.
- Pausing is not doing something. A source cannot become current by stopping.
- A driver that dies leaves "playing" retained on the broker. Its last will is
  what takes it out.
- "Most recent" is judged by nowplayd's own clock, because drivers run on
  different machines. The one exception is start-up, when every source's
  retained state arrives at once: then the senders' own times decide, held to
  the present so a fast clock cannot win forever.

## What it guarantees

While something is playing, `current` carries a **title, an album and art**,
whichever driver reported it.

- **The player's own cover always wins.** It is the right cover; a catalogue's
  is a good guess. It counts only when its hash matches the one the state
  names, so a track change never goes out with the previous track's image.
- Otherwise the cover is looked up, in order: **Deezer**, the **iTunes Search
  API**, then **MusicBrainz** with the Cover Art Archive. None needs a key.
- A missing **album** is looked up by artist and title. A known album is never
  renamed to a catalogue's "…(Legacy Edition)".
- An answer about a different artist, album or track is **refused**. A
  confident wrong cover lights the room for a record that is not playing.
- **One lookup per album**, remembered, misses included. That is what keeps it
  far inside the catalogues' limits; the tightest is about twenty calls a
  minute.
- When nobody has the cover, the track is published **without one** rather
  than withheld.
- The cover is published **before** the state that names it, and never twice
  running: the next track on the same album does not repaint the room.

## Configuration

Environment only, like `lightd`. Each broker setting falls back to the
platform's name from `kit.env`; an `NP_*` variable wins. A value that cannot
be used stops the program.

| Variable | Default | |
|---|---|---|
| `NP_MQTT_URL` | `tls://$MQTT_HOST:$MQTT_PORT`, else `tcp://127.0.0.1:1883` | broker |
| `NP_MQTT_CLIENT_ID` | `eds-nowplayd` | must be unique on the broker |
| `NP_MQTT_USERNAME` / `NP_MQTT_PASSWORD` | `$MQTT_USERNAME` / `$MQTT_PASSWORD` | `kit.env` carries lightd's login, so nowplayd names its own |
| `NP_MQTT_CA_FILE` | `$MQTT_CA_FILE` | the Deevnet Root CA |
| `NP_MQTT_INSECURE` | `false` | bring-up only; anything but a boolean is an error |
| `NP_TOPIC_PREFIX` | `$DEEVNET_TENANT`, else `eds` | one topic level |
| `NP_CATALOGUES` | `deezer,itunes,musicbrainz` | where to look, in order; empty switches lookups off |
| `NP_LOOKUP_TIMEOUT` | `10s` | for one catalogue request; 1s to 1m |
| `NP_CONTACT` | this repository's URL | goes in the User-Agent; catalogues require a way to reach the caller |

## Things worth knowing

- **Deezer's field search does not work for tracks.** `artist:"…" album:"…"`
  works on its album search; `artist:"…" track:"…"` finds nothing or something
  unrelated. Track lookups use plain text and check every result. Found by
  running against the live service, after tests against a stand-in had passed.
- **Catalogues do not always agree on the album.** Asked which album a track
  is on, Deezer may name the single and iTunes the album.
- **A catalogue's cover is not byte-stable.** The same image fetched twice can
  differ, so its hash can too. Within one run that is hidden by the cache; a
  restarted nowplayd may publish the "same" cover as a new one, once.
- **`bus.Connect` does not return until its subscriptions are acknowledged.**
  Without that, a message published the moment after it returned was missed.
  A test caught it.
- Paused or stopped playback leaves the cover topic alone, so the stand keeps
  the last scene. What "stopped" should look like is a later decision.

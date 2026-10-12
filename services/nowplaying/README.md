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

One thing travels the other way. A controller says what it wants of whatever
is playing, nowplayd passes it to the source that is current, and that
source's driver does it (ADR-0020):

```
a controller ──▶ eds/nowplaying/command ──▶ nowplayd ──▶ eds/nowplaying/source/<id>/command ──▶ the driver
                    (never retained)      the current source        (never retained)
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

**Deployed end to end: the agent on its Pi following iTunes and Spotify, nowplayd on the workload, and the stand following iTunes's covers.**

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
| `nowplayd` on the workload | deployed ([`deploy`](../../deploy)). An album change in iTunes reaches the stand in about a second |
| commands, the small cover | unit-tested, and run on a development broker: a cover published by hand came back as a 240-pixel baseline JPEG, and a command reached the current source's topic. **Not yet tried against iTunes**: the five requests are the ones the phone remote is documented to send, and nothing here has pressed them on the real player (ADR-0020) |
| `spotifyauth`, `driver/spotify` | unit-tested against a stand-in for Spotify. Running on the agent Pi against the real account since 2026-10-11: tracks on the named computer are reported within a poll, playback on the phone is not, and the stand does not follow either ([CHG-0004](../../docs/content/docs/changes/2026/0004-spotify-on-the-agent-pi.md)) |
| the agent with two drivers | running on the Pi: each driver has its own connection and presence, and iTunes reports while Spotify is not set up |

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

### Commands

The remote iTunes was paired with can press buttons as well as watch, and the
driver now does (ADR-0020): `play_pause`, `next`, `previous`, `volume_up` and
`volume_down`, heard on `eds/nowplaying/source/itunes/command`.

- Each is one short request to the player, beside the one held open. The
  driver says nothing on the bus about having done it: the player reports the
  change it caused, as it reports any other, and that is the acknowledgement.
- Volume moves by 5 of 100 a press, and stops at its ends. The player is asked
  where it is first, because its own slider moves it too.
- A command that arrives with no session is refused and not kept. A "next"
  carried out a minute late is a fault of its own.
- **A retained command is dropped**, by nowplayd and again by the agent. A
  command is a moment; one the broker kept would be carried out at every
  reconnect.
- Each command is logged with an event (`driver.command`,
  `driver.command_failed`), so the dashboard shows what a controller asked
  for and whether it happened.
- The subscription is optional. An agent whose account has not been granted
  the topic says so (`driver.commands_refused`) and reports what is playing
  regardless.

The Spotify driver takes no commands and subscribes to nothing. Its
authorization only reads, and widening it is a decision.

## The Spotify driver

Spotify has nothing local to pair with. The driver asks Spotify's servers what
the account is playing, every ten seconds
([ADR-0012](../../docs/content/docs/architecture/decisions/0012-spotify-by-web-api.md)).

```bash
NP_SPOTIFY_CLIENT_ID=… bin/npagent spotify-auth   # a browser opens; click Agree, once
bin/npagent spotify-devices                       # the account's devices, by name
NP_DRIVERS=spotify NP_SPOTIFY_DEVICES="Study PC" bin/npagent watch
```

- **It asks to read playback and nothing else**, by PKCE, so the app's secret
  is never used. The app's redirect URI must be exactly
  `http://127.0.0.1:8898/callback`.
- **Only named devices count.** Playback anywhere else on the account is
  reported as `stopped`. So are advertisements, podcast episodes, and
  anything in a private session.
- **It never asks for a picture.** Spotify is reported without its cover, and
  the stand does not follow it
  ([ADR-0016](../../docs/content/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover.md)).
- **The authorization has one holder.** Spotify may replace the refresh token
  whenever it is used, and the old one stops working. Authorize on a desk,
  then move the file to where the driver runs; `deploy/relay` does this.
- **It ends after six months** whatever is done with it. The driver warns
  from two weeks before, and when it ends reports itself offline and stops.
- **The app's owner needs Premium.** Without it Spotify answers `403` and
  says why; the driver reports itself offline and asks again every five
  minutes.

## More than one driver

`NP_DRIVERS=itunes,spotify` runs both in one agent. Each has its own
connection to the broker, because a connection has one last will and each
driver's presence is its own. A driver that is missing what it needs - a
pairing, an authorization - is reported offline and the other runs. With
nothing able to run the agent exits 78, which its unit does not restart.

## What the agent reports about itself

With `NP_LOG_DEVICE` set to the name the Pi is registered under, the agent
publishes a few events to `<tenant>/log/<device>`, which the substrate carries
into the tenant's log store, the same way the stand's go
([ADR-0017](../../docs/content/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device.md)):
`agent.started`, `driver.online` and `driver.offline`, `track.playing`,
`track.paused` and `track.stopped` with the artist, album and title, and
Spotify's refusals. They are on the tenant's dashboard under *What played*
and *Device events*.

A log line becomes an event by carrying an `event` attribute; the journal
still has every line. Events wait in a queue and are dropped, and counted,
before they hold a driver up. Without the setting nothing is reported, which
is how the agent runs on a desk.

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
- With nothing playing, a paused track comes before a source with nothing
  loaded, whichever reported last. A driver that starts up and says
  "stopped" does not take the place of a track paused elsewhere.
- "Most recent" is judged by nowplayd's own clock, because drivers run on
  different machines. The one exception is start-up, when every source's
  retained state arrives at once: then the senders' own times decide, held to
  the present so a fast clock cannot win forever.

## Albums with a look of their own

A few albums are lit by something other than their cover's colours
([ADR-0019](../../docs/content/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own.md)).
The list is in `internal/looks`, matched on the artist and album the track
already carries, and it is short on purpose: the one entry is *The Dark Side
of the Moon*, whose look is `spectrum`.

nowplayd publishes the look's **name** on `…/current/look`, retained, with
the hash of the cover it belongs to, just before that cover. It says what an
album is called and nothing about colours or effects: what `spectrum` looks
like is lightd's. A cover whose album has no look gets a message that says
so, so a look cannot outlive its album.

## The small cover

A cover is published as it came: up to 600 pixels a side, a JPEG of either
kind or a PNG, often a hundred kilobytes. A subscriber with a hundred
kilobytes for everything cannot hold that, and one with a small decoder cannot
read a progressive JPEG at all. So each cover is published a second time on
`…/current/art/small` (ADR-0020), retained:

- a **baseline JPEG**, three components of eight bits;
- scaled so its **longer side is 240 pixels**, shape kept, and scaled up if
  it started smaller: a subscriber is always handed the same size;
- **32 KB at most**. Quality starts at 80 and is given up, to 30, to stay
  under; a typical sleeve is around 10 KB;
- after the cover and **before the state that names it**;
- **emptied**, not left, when a cover cannot be made small: a subscriber
  holding the previous album's would have nothing to tell it so. An empty
  small cover means "this cover has no small form";
- left alone when no cover is published at all, exactly as `…/current/art` is.

It is `internal/smallcover`, standard library only. nowplayd still knows
nothing about who wants it: the size is "small", not the size of a screen.

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
- A source's state and its cover are two messages, and a broker handing over
  retained messages promises no order. A state that names a cover nowplayd
  does not yet hold is **held for up to two seconds** for it, rather than
  completed from a catalogue and then corrected.

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
| `NP_COVERLESS_SOURCES` | none | sources reported without a cover: none is published for them and none is looked up ([ADR-0016](../../docs/content/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover.md)). `spotify` on the workload, which is what keeps the stand from following it |

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
- **The client library calls a refused subscription a success.** The refusal
  is a return code in the broker's answer, and `bus` looks at it for the
  optional topics, which is how a program can say "I was not granted this"
  instead of waiting for messages that will never come.
- **nowplayd hears the commands it passes on.** Its one filter for every
  source, `source/+/+`, matches `source/<id>/command` too. The hub takes
  those for what they are and ignores them.
- Paused or stopped playback leaves the cover topic alone, so the stand keeps
  the last scene. What "stopped" should look like is a later decision.

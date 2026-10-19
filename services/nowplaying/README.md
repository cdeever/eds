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
| the three catalogs | each checked once against the live service on 2026-10-10 |
| `discogs` | unit-tested against a stand-in. Asked of the real collection on 2026-10-12: 226 records listed, and the tracks played on the bench each found on the record they were played from, with its sleeve. Being told to wait by Discogs was met once, and waited out. Deployed on the workload on 2026-10-12 ([CHG-0014](../../docs/content/docs/changes/2026/0014-the-owners-collection.md)): a track the recognizer put on a greatest hits was shown as on the owner's *Cosmo's Factory*, with that sleeve, and the stand lit from it |
| `dacp`, `driver/itunes` | unit-tested, including a replay of a session recorded from the real iTunes |
| `npagent` | paired with iTunes 12.13.10.3 on Windows and run end to end on a development broker: iTunes → driver → nowplayd → lightd → scene ([ADR-0011](../../docs/content/docs/architecture/decisions/0011-itunes-by-remote-pairing.md)) |
| the agent Pi | running as a relay on `dv02rpi002p01`: paired with iTunes on the home LAN and publishing to the real broker ([`images`](../../images), [`deploy/relay`](../../deploy/relay)) |
| broker accounts | declared and checked against the real broker (`eds-nowplayd`, `eds-np-relay`) |
| `nowplayd` on the workload | deployed ([`deploy`](../../deploy)). An album change in iTunes reaches the stand in about a second |
| commands, the small cover | unit-tested, and run on a development broker: a cover published by hand came back as a 240-pixel baseline JPEG, and a command reached the current source's topic. **Not yet tried against iTunes**: the five requests are the ones the phone remote is documented to send, and nothing here has pressed them on the real player (ADR-0020) |
| `spotifyauth`, `driver/spotify` | unit-tested against a stand-in for Spotify. Running on the agent Pi against the real account since 2026-10-11: tracks on the named computer are reported within a poll, playback on the phone is not, and the stand does not follow either ([CHG-0004](../../docs/content/docs/changes/2026/0004-spotify-on-the-agent-pi.md)) |
| `driver/turntable` | unit-tested: a whole side is played to it in tests, in no time, because its clock is the audio. Running on the agent Pi since 2026-10-12: a record was named within half a minute and lit the stand, the needle lifting was reported as stopped twenty seconds later, and the sound card unplugged and put back was lost and found without a restart. Not yet tried: a needle put down mid-track, a record its recognizer does not know, a whole side ([ADR-0021](../../docs/content/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening.md), [CHG-0013](../../docs/content/docs/changes/2026/0013-the-turntable-driver.md)) |
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
  change it caused, as it reports any other, and that is the acknowledgment.
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

The Spotify driver takes the same five
([ADR-0032](../../docs/content/docs/architecture/decisions/0032-spotify-takes-commands.md)),
through Spotify's Web API, and only while its last answer was a track on a
device that counts: a press here does not pause a phone somewhere else. A
device whose volume Spotify may not set is told nothing about volume. An
authorization given before ADR-0032 only reads, and Spotify refuses each
command until it is given again (`make -C deploy/relay spotify AGAIN=1`).

## The Spotify driver

Spotify has nothing local to pair with. The driver asks Spotify's servers what
the account is playing, every ten seconds
([ADR-0012](../../docs/content/docs/architecture/decisions/0012-spotify-by-web-api.md)).

```bash
NP_SPOTIFY_CLIENT_ID=… bin/npagent spotify-auth   # a browser opens; click Agree, once
bin/npagent spotify-devices                       # the account's devices, by name
NP_DRIVERS=spotify NP_SPOTIFY_DEVICES="Study PC" bin/npagent watch
```

- **It asks to read playback and to change it** - play, pause, skip, volume
  - **and nothing else**, by PKCE, so the app's secret
  is never used. The app's redirect URI must be exactly
  `http://127.0.0.1:8898/callback`.
- **Only named devices count.** Playback anywhere else on the account is
  reported as `stopped`. So are advertisements, podcast episodes, and
  anything in a private session.
- **The cover is the album's picture as Spotify names it**, in the answer
  the driver already asks for, fetched once for each album and with no
  token
  ([ADR-0031](../../docs/content/docs/architecture/decisions/0031-spotify-is-shown-like-itunes.md)).
  The stand does not follow it: Spotify is in `NP_DARK_SOURCES`.
- **The authorization has one holder.** Spotify may replace the refresh token
  whenever it is used, and the old one stops working. Authorize on a desk,
  then move the file to where the driver runs; `deploy/relay` does this.
- **It ends after six months** whatever is done with it. The driver warns
  from two weeks before, and when it ends reports itself offline and stops.
- **The app's owner needs Premium.** Without it Spotify answers `403` and
  says why; the driver reports itself offline and asks again every five
  minutes.

## The turntable driver

A turntable cannot be asked what it is playing. The driver listens instead:
to the amplifier's tape output, through a sound card on the agent Pi, and it
has a recognizer name what it hears
([ADR-0021](../../docs/content/docs/architecture/decisions/0021-the-turntable-is-identified-by-listening.md)).

```bash
NP_DRIVERS=turntable NP_TURNTABLE_DEVICE=plughw:UA4FX bin/npagent watch
```

Linux only, as it stands: it listens with `arecord`. `arecord -L` lists the
inputs; a `plughw:` one lets ALSA do the converting to what the driver wants,
mono at 16 kHz.

- **It knows silence for certain, and nothing else.** Twenty seconds of
  silence is `stopped`. Music is `playing`, with the track when the recognizer
  names it and without one when it cannot. It is never made up.
- **Its clock is the audio.** Everything is counted in tenths of a second
  heard, not in time passed, so a sound card that stalls is not mistaken for
  silence, and a test plays it a side in milliseconds.
- **A clip is twelve seconds, sent every twenty** while music plays: three
  requests a minute, and none in silence.
- **A gap starts the listening over** (ADR-0026). A second of quiet with
  music after it is taken for the gap between two tracks: what was heard
  before it is dropped, and a clip is sent as soon as there are twelve
  seconds of what came after. Gaps closer together than twenty seconds are
  let pass, so music that keeps stopping is still asked about.
- **The first answer after silence or a gap is believed. No other single
  answer is.** Those clips are all one track; one sent by the clock may
  straddle two, and a new track is then taken when two clips running agree
  on it. Four running that nothing is known about let the track go.
- **It reports the album as the recognizer named it**, an empty one
  included. Which record a track is on is nowplayd's to say, by what is on it
  ([ADR-0038](../../docs/content/docs/architecture/decisions/0038-the-turntable-names-the-original-album-by-what-is-on-it.md)):
  the owner's collection first; then the record already playing, if its
  track list has the track, which keeps a side's sleeve across a flip and a
  track the recognizer files under a box set; and otherwise the original
  studio album the track is on. Deezer finds the albums a track is on and
  lists their tracks; MusicBrainz says which is the studio original. No rule
  depends on how long the deck was quiet: another record by the same band
  is another record, however soon it is put on.
- **It names no cover and takes no commands.**

### The recognizer

The driver knows its recognizer as a program: a WAV file on its standard
input, one JSON object on its output - `artist`, `title`, `album` - or `{}`
when it does not know. A failure is a non-zero exit, and is not mistaken for
not knowing.

On the Pi that program is `eds-recognize`
([`images/agent/files`](../../images/agent/files)), and it is Shazam's
recognizer through ShazamIO. **Nobody published the endpoint it speaks to**,
and it can stop working on any day without anything here changing. It is
behind the program so that mending or replacing it is that one file and its
pinned requirements. Do not move knowledge of Shazam into the driver.

## More than one driver

`NP_DRIVERS=itunes,spotify` runs both in one agent. Each has its own
connection to the broker, because a connection has one last will and each
driver's presence is its own. A driver that is missing what it needs - a
pairing, an authorization - is reported offline and the other runs. With
nothing able to run the agent exits 78, which its unit does not restart.

## What the agent reports about itself

With `NP_LOG_DEVICE` set to the name the Pi is registered under, the agent
publishes a few events to `<tenant>/log/<device>`, which Deevnet IoTaaS carries
into the tenant's log store, the same way the stand's go
([ADR-0017](../../docs/content/docs/architecture/decisions/0017-an-agent-pi-is-a-registered-device.md)):
`agent.started`, `driver.online` and `driver.offline`, `track.playing`,
`track.paused` and `track.stopped` with the artist, album and title,
Spotify's refusals, and the turntable losing its input or its recognizer. They are on the tenant's dashboard under *What played*
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

### A source that is switched on

One thing comes before all of that
([ADR-0023](../../docs/content/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand.md)).
A source can have a beacon: a device on the same switch as its equipment,
connected to the broker while that equipment has power. `NP_SOURCE_POWER`
says which beacon speaks for which source, and nowplayd reads
`<prefix>/beacon/<id>/status`.

- **On**, the source is current, whatever else is playing and whatever it is
  doing itself. The turntable between its sides is the turntable, stopped,
  and a track left paused in iTunes does not take its place.
- **Off**, the source is not heard, whatever its driver reports. What the
  turntable's driver hears with the turntable off is not a record.
- **Not heard from**, the beacon changes nothing. The setting can be in
  place before the beacon is, and a subscription the broker refuses is a
  warning in the log.
- The switch does not stand in for the driver: a source that is on, and
  offline or never heard from, holds nothing.

While a source that takes no commands is held, a controller's commands go
nowhere. That is what "current" means, and it is the turntable's case. So
`current` says `no_controls` for it, and the cover screen draws no buttons
([ADR-0025](../../docs/content/docs/architecture/decisions/0025-current-says-when-there-are-no-controls.md)).

### Paused long enough is not playing

A track left paused stays current for `NP_IDLE_AFTER`, fifteen seconds, and
then nothing is said to be playing
([ADR-0024](../../docs/content/docs/architecture/decisions/0024-paused-long-enough-is-not-playing.md)):
`current` is `stopped` with no track, and the cover topics are emptied,
which is what tells `lightd` there is nothing to light the stand from. The
source is still named, so a command still reaches it and play can be
pressed. Nobody reporting at all goes the same way after the same wait.

The stand is for records. A source in `NP_DARK_SOURCES`, `itunes` on the
workload, has its cover published as usual and given the look `dark`, and
`lightd` lights nothing from it.

An idle room is dark unless something is switched on
([ADR-0028](../../docs/content/docs/architecture/decisions/0028-the-stand-is-for-records.md)).
Before the cover topics are emptied, `current/look` is given the look of no
cover, `{"v":1,"look":"standby","art":""}`, when the beacon of any source in
`NP_SOURCE_POWER` says it is on, and `{"v":1,"art":""}` when none does.
`lightd` draws the first dim and the second dark. A beacon going on or off
in an idle room publishes the look again, and nothing else.

- The wait is counted from when the source says it paused, so a track paused
  an hour ago is idle at once when nowplayd starts, and not lit again first.
- A source that is stopped and still current, which is one being held, has
  a wait of its own, `NP_IDLE_AFTER_STOPPED`, one second. The turntable's
  driver has heard twenty seconds of silence before it says `stopped`, so
  the room is dim about twenty seconds after the music ends, and a record
  being turned over dims it too (ADR-0026). Dim and not dark: the turntable
  is on.
- Playing again publishes everything again, the same cover included.

## What is asked of other people's services

None of the services this module calls is EdS's, and each has a limit,
published or not. Every call is counted (`internal/calls`, ADR-0027), and
once a minute each service that was asked anything gets one line, the event
`api.calls`:

```json
{"event":"api.calls","api":"discogs","calls":3,"limited":0,"failed":0,"slowest_ms":412,"mean_ms":280,"remaining":57}
```

- `limited` is answers of 429: the service saying it was asked too often.
  `failed` is no answer, or an error or a refusal. "Not found" is neither.
- `remaining` is the least the service said was left of its allowance in
  that minute, for a service that says. Discogs does.
- HTTP calls are counted by the client they go through, redirects included,
  and named by host: `discogs`, `deezer`, `itunes`, `musicbrainz`,
  `coverartarchive`, `spotify`. A service's pictures come from other hosts
  and are counted apart (`discogs-images`), because the allowance is the
  API's.
- The turntable's recognizer is a program and not an HTTP client, so each
  clip it is asked about is one call. It says whose service it asked in its
  answer; the driver does not know.
- **It counts, and limits nothing.** Each client keeps to its service's
  rules itself.

`npagent` reports the line as an event, like any other, and it arrives in
the tenant's device partition. `nowplayd` is not a device: it posts the line
to the tenant's log store itself (`internal/logstore`), to the workload
partition, when it has `LOG_ENDPOINT` and `LOG_INGEST_TOKEN`. That is the
only thing it sends there. The dashboard is **Outside Services**
(`infra/deevnet-tenant-eds/dashboards/outside-services.json`).

## Albums with a look of their own

A few albums are lit by something other than their cover's colors
([ADR-0019](../../docs/content/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own.md)).
The list is in `internal/looks`, matched on the artist and album the track
already carries, and it is short on purpose: the one entry is *The Dark Side
of the Moon*, whose look is `spectrum`.

One piece of music has a look too, whatever record it is on and whoever
plays it
([ADR-0030](../../docs/content/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source.md)):
the theme from *Knight Rider*, `scanner`. It is matched on the track's title
alone, which must be `knight rider` once the words that only say "this is a
theme" are set aside. It comes before the album's look, and it is the one
look published for a source in `NP_DARK_SOURCES`: `looks.AnySource` says so.

nowplayd publishes the look's **name** on `…/current/look`, retained, with
the hash of the cover it belongs to, just before that cover. It says what an
album is called and nothing about colors or effects: what `spectrum` looks
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

- **The player's own cover always wins.** It is the right cover; a catalog's
  is a good guess. It counts only when its hash matches the one the state
  names, so a track change never goes out with the previous track's image.
- Otherwise the cover is looked up, in order: **Deezer**, the **iTunes Search
  API**, then **MusicBrainz** with the Cover Art Archive. None needs a key.
- A missing **album** is looked up by artist and title. A known album is never
  renamed to a catalog's "…(Legacy Edition)".
- An answer about a different artist, album or track is **refused**. A
  confident wrong cover lights the room for a record that is not playing.
- **One lookup per album**, remembered, misses included. That is what keeps it
  far inside the catalogs' limits; the tightest is about twenty calls a
  minute.
- When nobody has the cover, the track is published **without one** rather
  than withheld.
- The cover is published **before** the state that names it, and never twice
  running: the next track on the same album does not repaint the room.
- A source's state and its cover are two messages, and a broker handing over
  retained messages promises no order. A state that names a cover nowplayd
  does not yet hold is **held for up to two seconds** for it, rather than
  completed from a catalog and then corrected.

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
| `NP_LOOKUP_TIMEOUT` | `10s` | for one catalog request; 1s to 1m |
| `NP_CONTACT` | this repository's URL | goes in the User-Agent; catalogs require a way to reach the caller |
| `NP_COLLECTION_SOURCES` | none | sources that play the owner's own records, looked up in the owner's Discogs collection before any catalog ([ADR-0022](../../docs/content/docs/architecture/decisions/0022-the-owners-collection-names-the-record.md)). `turntable` on the workload |
| `NP_DISCOGS_USER` | none | whose collection. Required when a source is named above |
| `NP_DISCOGS_TOKEN` | none | a personal access token; needed for a private collection, and raises Discogs' limit from 25 requests a minute to 60. **Secret** |
| `NP_COVERLESS_SOURCES` | none | sources reported without a cover: none is published for them and none is looked up ([ADR-0016](../../docs/content/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover.md)). No source on the workload: Spotify was one until [ADR-0031](../../docs/content/docs/architecture/decisions/0031-spotify-is-shown-like-itunes.md) |
| `NP_DARK_SOURCES` | none | sources no room is lit for: their covers are published as usual, with the look `dark`, which `lightd` answers by sending the stand dark ([ADR-0028](../../docs/content/docs/architecture/decisions/0028-the-stand-is-for-records.md)). `itunes,spotify` on the workload: the stand is for records |
| `NP_IDLE_AFTER` | `15s` | how long a track may sit paused, or nobody report, before nothing is said to be playing and the cover topics are emptied ([ADR-0024](../../docs/content/docs/architecture/decisions/0024-paused-long-enough-is-not-playing.md)). `0s` is never; at most an hour |
| `NP_IDLE_AFTER_STOPPED` | `1s` | the same wait for a source that is stopped and still current: a turntable switched on with nothing playing. Short, because its driver has already waited out twenty seconds of silence. `0s` is never; at most a day |
| `NP_NO_CONTROLS_SOURCES` | none | sources a controller is told to offer nothing for, as `no_controls` in `current` ([ADR-0025](../../docs/content/docs/architecture/decisions/0025-current-says-when-there-are-no-controls.md)). `turntable` on the workload |
| `NP_LOG_ENDPOINT`, `NP_LOG_INGEST_TOKEN` | `LOG_ENDPOINT` and `LOG_INGEST_TOKEN`, as `kit.env` gives them | the tenant's log store and the token that writes to its workload partition: where the counts of calls to outside services go once a minute (ADR-0027). Without both, the counts are in the journal only. The token is secret |
| `NP_SOURCE_POWER` | none | `source=beacon` pairs: a source whose equipment has a beacon on its switch, and so can say it is on ([ADR-0023](../../docs/content/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand.md)). `turntable=turntable-power-01` on the workload |

## The owner's collection

A source that hears a track cannot know which record it is on. The
recognizer behind the turntable names an album with each track, and names a
compilation or a reissue as often as not. But the record on the platter is
one its owner has, and the owner's collection at Discogs says which
([ADR-0022](../../docs/content/docs/architecture/decisions/0022-the-owners-collection-names-the-record.md)).

For the sources `NP_COLLECTION_SOURCES` names, `nowplayd` asks the collection
first, by artist and title:

- **The record's title becomes the album**, replacing what the source called
  it, and **its sleeve becomes the cover**: the pressing that is owned, not a
  catalog's guess at an edition. `art.from` is `discogs`.
- **A track on several records** goes to an album before a compilation,
  always; then the one named as the source named the album, the one last
  answered with (a side is played through), vinyl before a disc, the
  earliest. So a greatest hits played through shows each song's own album
  where that album is owned, and its own sleeve for the rest. That is the
  owner's choice.
- **A track the collection does not have** is completed the ordinary way,
  from the catalogs.
- **Only those sources.** iTunes says which album it is playing, and knows.

The list of records is read when nowplayd starts and again every six hours. A
record's tracklist is read the first time its artist is heard, and kept. One
cover is held, the last. Nothing is written to disk.

Two things it gets wrong, both by not knowing what it has not been told:

- **A record that is not in the collection, whose track is on one that is.**
  The other record is named. Adding the record at Discogs is the cure.
- **Compilations of several artists.** A record is found by the artist it is
  credited to, and those are credited to "Various".

`internal/discogs` has a test that asks the real Discogs, skipped unless
`NP_TEST_DISCOGS="Artist|Title|Album"` and `NP_DISCOGS_USER` are set.

## Things worth knowing

- **Deezer's field search does not work for tracks.** `artist:"…" album:"…"`
  works on its album search; `artist:"…" track:"…"` finds nothing or something
  unrelated. Track lookups use plain text and check every result. Found by
  running against the live service, after tests against a stand-in had passed.
- **Catalogs do not always agree on the album.** Asked which album a track
  is on, Deezer may name the single and iTunes the album.
- **A catalog's cover is not byte-stable.** The same image fetched twice can
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

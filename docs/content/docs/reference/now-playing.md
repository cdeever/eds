---
title: "Now Playing"
weight: 4
---

# Now Playing

The contract between the programs that know what is playing and the programs
that react to it. It is version **1**, stamped on every message as `"v": 1`,
and a message with any other version is refused rather than guessed at.

Why it is shaped this way:
[ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/) and
[ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/).

## Topics

All under the tenant's prefix, `eds/`.

| Topic | From | Retained | Payload |
|---|---|---|---|
| `nowplaying/source/<id>/state` | a driver | yes | what that player is doing |
| `nowplaying/source/<id>/status` | a driver | yes (last will) | `online` / `offline` |
| `nowplaying/source/<id>/art` | a driver | yes | the cover, as image bytes |
| `nowplaying/current` | nowplayd | yes | the one current track |
| `nowplaying/current/art` | nowplayd | yes | its cover, as image bytes |
| `nowplaying/current/art/small` | nowplayd | yes | the same cover, small: a baseline JPEG, 240 pixels on its longer side |
| `nowplaying/current/look` | nowplayd | yes | whether that cover's album has a look of its own; with no cover, whether the room is waiting for one |
| `nowplaying/command` | a controller | **never** | a button press, for whatever is current |
| `nowplaying/source/<id>/command` | nowplayd | **never** | that press, passed to the current source |
| `nowplaying/status` | nowplayd | yes (last will) | `online` / `offline` |

`<id>` names one driver deployment, such as `itunes-den`. It is lowercase
letters, digits and hyphens, at most 63 characters, starting with a letter or
digit.

**Subscribers read `nowplaying/current` and nothing else.** The source topics
are the drivers' side of the contract.

**Everything that describes the world is retained**, for the reason the scene
is: a program that starts later must find the state of the world waiting for
it. A command is the exception, and is never retained: see
[Commands](#commands).

## The state message

The same shape on a source topic and on `current`.

```json
{ "v": 1,
  "source": "itunes-den", "player": "itunes",
  "state": "playing",
  "track": { "title": "So What", "artist": "Miles Davis",
             "album": "Kind of Blue", "duration_ms": 562000 },
  "position_ms": 12000,
  "changed_at": "2026-10-10T16:00:00Z",
  "art": { "sha256": "76cc…1661", "mime": "image/jpeg", "from": "player" } }
```

| Field | | |
|---|---|---|
| `v` | required | `1` |
| `source` | on a source topic, required and equal to the `<id>` in the topic | on `current`, the source that won; absent when nothing is reporting |
| `player` | optional | `itunes`, `music`, `spotify`, `tidal`, `musicbee`, `turntable` |
| `state` | required | `playing`, `paused` or `stopped` |
| `track` | optional | absent when stopped with nothing loaded |
| `track.title`, `.artist`, `.album`, `.album_artist` | optional | a source reports what its player says and nothing more |
| `track.duration_ms` | optional | |
| `position_ms` | optional | where playback was when the message was sent; a hint, not kept current |
| `changed_at` | required | when the track or the play state last changed, by the sender's clock, RFC 3339 |
| `art` | optional | describes the image on the matching `art` topic |
| `art.sha256` | required within `art` | the SHA-256 of that image, in hex |
| `art.mime` | optional | |
| `art.from` | on `current` | `player` when the source supplied it; `discogs` when it is the sleeve of a record in the owner's collection ([ADR-0022](/docs/architecture/decisions/0022-the-owners-collection-names-the-record/)); else the catalog it was found in |

Unknown fields are ignored, so one can be added without breaking a subscriber
that has not been rebuilt.

## What `current` guarantees

While `state` is `playing`, `current` carries `track.title`, `track.album` and
`art` — whichever driver reported it, and whether or not that driver knew them.
nowplayd fills in what the source lacked.

The exception is honest rather than hidden: when no catalog has the album or
the cover, `current` is published **without** it. A track with no cover is
still the track that is playing.

**A source can be reported without its cover on purpose**
([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).
No source is, at present: Spotify was until
[ADR-0031](/docs/architecture/decisions/0031-spotify-is-shown-like-itunes/). While such a source is current, `current` names the track with no `art`,
nothing is published to `current/art`, and that topic keeps the last cover it
held, which belongs to an earlier track. A subscriber that wants the cover of
the track that is playing must find `art` in `current` and match its
`sha256`; one that only follows `current/art`, as the stand does, is not
moved by it at all.

## Looks

A few albums are lit by something other than their cover's colors
([ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/)).
`nowplayd` says which, beside every cover it publishes, and before it:

```json
{"v":1,"look":"spectrum","art":"<sha256 of the cover>"}
```

| Field | |
|---|---|
| `v` | `1`. Another version is refused |
| `look` | the look's name; left out when the album has none |
| `art` | the SHA-256 of the cover this is about, as in `current`. Empty, the look is for there being no cover: see [Idle](#idle) |

A look belongs to the cover it names and to no other, so the two messages
can arrive in either order. What a name looks like is `lightd`'s to say; a
name it does not know is the same as none.

Two albums have a look: *The Dark Side of the Moon*, `spectrum`, and
*American Idiot*, `grenade`. Each is matched on the track's own tags: an
album that starts with that title, the leading "The" or not, by an artist
that includes the band. A remaster with its year in parentheses matches; a
copy tagged some other way does not, and is lit from its cover.

One look is a single track's: `scanner`, for the theme from *Knight Rider*
([ADR-0030](/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source/)).
It is matched on the title alone - `knight rider`, with or without `theme`,
`from`, `main title` and the like, and whatever is in parentheses - and comes
before the album's look. The look is published again, for the same cover,
when the next track's is different.

A second look, `dark`, is not an album's. It is given to every cover from a
source in `NP_DARK_SOURCES`, before any look the album has - but not before
`scanner`, which is shown from any source - and means that
nothing is to be lit from this cover
([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/)).
The cover itself is published as usual. `itunes` is such a source.

Nothing is said about a look when no cover is published, so a source reported
without its cover never has one.

When no source is reporting at all, `current` is
`{"v":1,"state":"stopped","changed_at":"…"}`, with no `source` and no `track`.

## Covers

A cover travels as its own retained message of raw image bytes, never inside
the JSON. That keeps the state readable in `mosquitto_sub`, and lets a
subscriber that wants only the picture subscribe to only the picture, which is
what `lightd` does.

- `art.sha256` is what ties a state to its image. A subscriber holding an
  image whose hash is not the one the state names is holding the previous
  track's.
- **The image is published before the state that names it**, by drivers and
  by nowplayd alike.
- **A subscriber must not rely on that order**, all the same. Both messages
  are retained, and a broker handing a new subscriber its retained messages
  may deliver them either way around. A subscriber that holds a state whose
  `art.sha256` it has no image for should wait briefly for the image, as
  nowplayd does for a source's.
- The same image is not published twice running.
- When playback pauses or stops, the cover topic is left as it is, until
  nothing has played for a while: see [Idle](#idle).

## Idle

A track left paused is not something playing for ever
([ADR-0024](/docs/architecture/decisions/0024-paused-long-enough-is-not-playing/)).
After `NP_IDLE_AFTER`, fifteen seconds on the workload, nowplayd says so:

- `current` becomes `{"v":1,"source":…,"player":…,"state":"stopped"}` with no
  `track` and no `art`. **The source is still named**: it is who a command
  is for, so play can be pressed from where pause was.
- `current/art` and `current/art/small` are **emptied**. An emptied cover
  topic means there is no cover, which is different from a cover not being
  sent: Spotify's track sends none and leaves the last one standing
  ([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).
  `lightd` answers an emptied topic by resting the stand: see below.
- Before they are emptied, `current/look` is given **the look of no cover**:
  `{"v":1,"look":"standby","art":""}` when the equipment of any source in
  `NP_SOURCE_POWER` is switched on, and `{"v":1,"art":""}` when none is
  ([ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/)).
  `lightd` shows `standby` as its idle scene, dim and warm, and anything
  else as dark. A switch going on or off while the room is idle publishes
  this look again, alone.
- The same goes for nobody reporting at all, after the same wait, with no
  source named.
- When something plays again, everything is published again in full, the
  same cover included.

A source that is `stopped` and still current has a wait of its own,
`NP_IDLE_AFTER_STOPPED`, one second. That is a source being held
([ADR-0023](/docs/architecture/decisions/0023-a-source-that-is-switched-on-holds-the-stand/)):
the turntable, whose driver says `stopped` after twenty seconds of silence.
The lights follow the music, and a record being turned over goes idle too
([ADR-0026](/docs/architecture/decisions/0026-what-silence-means-on-the-turntable/)).

An emptied retained topic is not delivered to a subscriber that arrives
later. One that lights something from the cover should take hearing nothing,
shortly after subscribing, as there being no cover.

## Controls

`current` may carry `"no_controls": true`
([ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/)).
It means a command to whatever is current would do nothing, and a controller
should offer none: the cover screen draws no buttons. It is said for the
sources `NP_NO_CONTROLS_SOURCES` names, the turntable on the workload, and
left out otherwise. A reader that does not know the field offers its
controls as before, and they do nothing.

## The small cover

Not every subscriber can use a cover as published: it may be 600 pixels, a
PNG or a progressive JPEG, and several hundred kilobytes. Beside each cover
nowplayd publishes a small one on `nowplaying/current/art/small`
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)).

- A **baseline JPEG**, three components, scaled so its longer side is exactly
  240 pixels. A cover that is not square stays not square.
- At most 32 KB. Quality is lowered until it fits.
- Published after the cover and before the state that names it.
- **An empty message means this cover has no small form**: it could not be
  read, or could not be made to fit. A subscriber holding the album before's
  drops it.
- When no cover is published at all, this topic is left as it is, like the
  cover topic. A subscriber must find `art` in `current` before showing it.
- It has no hash of its own in `current`. `art.sha256` is the full cover's.

## Commands

A controller can ask for something of whatever is playing
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)).
It publishes to `nowplaying/command`:

```json
{"v":1,"command":"next","from":"cover-screen-01"}
```

| Field | |
|---|---|
| `v` | `1`. Another version is refused |
| `command` | `play_pause`, `next`, `previous`, `volume_up` or `volume_down`. Anything else is refused |
| `from` | optional: who sent it, for the logs. Not trusted for anything |

nowplayd passes the message, unchanged, to `nowplaying/source/<id>/command`
for the source of the current track. A controller knows no source and no
player.

- **A command is never retained, and a retained one is ignored.** A command
  the broker kept would be carried out again by every program that connected
  afterward.
- **Send it only while connected, and do not queue it.** A command is for
  now. The cover screen sends at QoS 0 for this reason.
- With no current source the command is dropped. A paused track keeps its
  source, so `play_pause` resumes it.
- **Not every source takes commands.** iTunes does. Spotify does not: its
  driver reads nothing. A command sent while it is current goes nowhere, and
  nothing says so.
- `volume_up` and `volume_down` move the player's volume five points in a
  hundred. There is no ceiling.

## Rules for a driver

1. Publish `online` to your status topic on connect, and make `offline` your
   last will. A driver that dies leaves "playing" retained; the will is what
   takes it out.
2. Publish the cover, if the player has one, **then** the state that names it.
3. Publish state when something changes. Repeating yourself is harmless and
   does not make you current.
4. Report what the player says. Do not look anything up: that is nowplayd's
   job, and it does it once for everyone.
5. A driver that can carry out commands subscribes to its own command topic
   and to nothing else. It ignores a retained one, and it keeps reporting if
   the subscription is refused.

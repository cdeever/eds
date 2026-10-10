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
| `nowplaying/status` | nowplayd | yes (last will) | `online` / `offline` |

`<id>` names one driver deployment, such as `itunes-den`. It is lowercase
letters, digits and hyphens, at most 63 characters, starting with a letter or
digit.

**Subscribers read `nowplaying/current` and nothing else.** The source topics
are the drivers' side of the contract.

**Everything is retained**, for the reason the scene is: a program that starts
later must find the state of the world waiting for it.

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
| `player` | optional | `itunes`, `music`, `spotify`, `tidal`, `musicbee` |
| `state` | required | `playing`, `paused` or `stopped` |
| `track` | optional | absent when stopped with nothing loaded |
| `track.title`, `.artist`, `.album`, `.album_artist` | optional | a source reports what its player says and nothing more |
| `track.duration_ms` | optional | |
| `position_ms` | optional | where playback was when the message was sent; a hint, not kept current |
| `changed_at` | required | when the track or the play state last changed, by the sender's clock, RFC 3339 |
| `art` | optional | describes the image on the matching `art` topic |
| `art.sha256` | required within `art` | the SHA-256 of that image, in hex |
| `art.mime` | optional | |
| `art.from` | on `current` | `player` when the source supplied it, else the catalogue it was found in |

Unknown fields are ignored, so one can be added without breaking a subscriber
that has not been rebuilt.

## What `current` guarantees

While `state` is `playing`, `current` carries `track.title`, `track.album` and
`art` — whichever driver reported it, and whether or not that driver knew them.
nowplayd fills in what the source lacked.

The exception is honest rather than hidden: when no catalogue has the album or
the cover, `current` is published **without** it. A track with no cover is
still the track that is playing.

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
- The same image is not published twice running.
- When playback pauses or stops, the cover topic is left as it is.

## Rules for a driver

1. Publish `online` to your status topic on connect, and make `offline` your
   last will. A driver that dies leaves "playing" retained; the will is what
   takes it out.
2. Publish the cover, if the player has one, **then** the state that names it.
3. Publish state when something changes. Repeating yourself is harmless and
   does not make you current.
4. Report what the player says. Do not look anything up: that is nowplayd's
   job, and it does it once for everyone.

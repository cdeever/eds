---
title: "ADR-0006: Now Playing Is a Bus of Small Drivers"
weight: 6
---

# ADR-0006: Now Playing Is a Bus of Small Drivers Behind One Contract

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | How EdS learns what is playing across several players, and how that knowledge reaches whatever reacts to it. |
| **Related** | [ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/), [ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/), [ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/), and one record per player: [0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/), [0012](/docs/architecture/decisions/0012-spotify-by-web-api/), [0013](/docs/architecture/decisions/0013-musicbee-by-remote-protocol/), [0014](/docs/architecture/decisions/0014-tidal-by-lastfm/) |

---

## Context

The stand lights from a cover image, but nothing tells EdS what is playing.
`lightd` has carried `POST /v1/cover` since the first vertical as "the
interface the future album-cover resolver will call"; this is what sits
upstream of it.

The players are Spotify, TIDAL, MusicBee and iTunes, all desktop clients on
Windows 11. Subscribers need at least the track title, the album title and
the cover art. More than one thing will want to react to a track change, and
some deployments will be small: something whose only job is to follow iTunes.

Nothing can dial the EdS workload
([ADR-0003](/docs/architecture/decisions/0003-broker-is-the-substrates/)), and
the workload cannot reach a home LAN.

## Options considered

**One Windows agent on the system media controls.** Windows exposes what
players report through one API (`GlobalSystemMediaTransportControlsSessionManager`),
and it looked like a universal driver. It is not, for these four:

| Player | Reports to it | Gap |
|---|---|---|
| Spotify | natively | thumbnail carries Spotify branding |
| TIDAL | natively | no album title |
| MusicBee | only with a third-party plugin | plugin last updated 2024, open bugs |
| iTunes (classic) | no | needs a separate helper program |

It also cannot be read from a background service on Windows 11, only from
the logged-in session, and its first-class bindings are .NET.
Sources: [Music Presence player notes](https://docs.musicpresence.app/setup/media-player/),
[dotnet/runtime#84293](https://github.com/dotnet/runtime/issues/84293),
[mb_MediaControl issues](https://github.com/HenryPDT/mb_MediaControl/issues),
[iTunes-SMTC](https://github.com/thewizrd/iTunes-SMTC).

**Cloud only.** Ask each service what the account is playing. Only Spotify
offers this to a third party. MusicBee and iTunes play local files and have
no cloud at all.

**Scrobbling as the bus.** Have every player scrobble to Last.fm or
ListenBrainz and read "now playing" from there. It reaches TIDAL, but it is
15–30 seconds late, has no pause state, carries no usable art, and iTunes
needs a helper whose state on Windows 11 is doubtful. The best-known
aggregator, multi-scrobbler, has no MQTT output and no source for TIDAL,
MusicBee, iTunes or the Windows media controls.
Source: [multi-scrobbler sources](https://github.com/FoxxMD/multi-scrobbler/tree/master/docsite/docs/configuration/sources).

**A plugin inside each player.** Exact, but a different toolchain per player,
.NET for MusicBee, and nothing at all for Spotify or TIDAL.

**Small drivers behind one contract.** Each driver knows one player and
reaches it however that player is best reached. All of them publish the same
message to the bus.

## Decision

Now playing is a **bus of small drivers behind one versioned contract**.

- A **driver** knows one player. It publishes that player's state, retained,
  to `eds/nowplaying/source/<id>/state`, with presence on `…/status` and the
  cover, when the player supplies one, as image bytes on `…/art`.
- A **deployment** runs whichever drivers suit where it sits: one, or several.
- **`nowplayd`** reads every source and publishes the one current track
  ([ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/)).
- **Subscribers** read `eds/nowplaying/current` and do their own thing. The
  first lights the stand
  ([ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/)).

The contract is stamped `"v": 1`, and a mismatched version is refused, as
with the scene descriptor.

## Consequences

- A player is added, replaced or dropped without touching anything else. A
  mechanism that stops working — an API withdrawn, a pairing refused — costs
  one driver and one superseded record.
- Each player gets the mechanism that suits it, at the price of several
  mechanisms to maintain instead of one.
- Drivers differ in what they know: one supplies art, another has no album.
  The contract allows a source to be incomplete, and `nowplayd` makes the
  guarantee to subscribers.
- Art travels as its own retained message, so the JSON stays readable in
  `mosquitto_sub`. A hash in the state ties the two together.
- Every driver deployment needs a broker account granted its own source
  topics, and an ungranted publish fails silently.
- This is a new source of truth for "what is playing" beside the vinyl the
  product was first described around. It is an additional source, not a
  replacement: a turntable that identifies its record would be one more
  driver on the same bus.

---
title: "ADR-0008: nowplayd Picks the Track and Guarantees the Fields"
weight: 8
---

# ADR-0008: nowplayd Picks the Current Track and Guarantees Title, Album and Art

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | What turns several sources' states into one current track, and who is responsible for the fields subscribers rely on. |
| **Related** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0009](/docs/architecture/decisions/0009-lightd-takes-covers-from-a-topic/) |

---

## Context

Several drivers may report at once: a paused iTunes, a Spotify playing on a
phone. Subscribers want one answer. They also need a title, an album and a
cover, and the drivers cannot all supply those: TIDAL through Last.fm arrives
with no art, and a locally read TIDAL has no album.

## Options considered

**Which source wins**

- *Most recent wins.* The source whose track most recently started or
  changed is current. Needs no configuration and matches what a person means
  by "what's playing".
- *Fixed priority.* A configured ranking. Predictable, but wrong whenever the
  listener's habit differs from the ranking.
- *One source per stand.* No arbitration; each stand is assigned a source.
  Suits several rooms, and is more to configure than one stand needs.

**Who completes the fields**

- *Each driver.* Every driver would need its own catalogue lookups.
- *Each subscriber.* Every subscriber would resolve the same cover.
- *One place between them.*

## Decision

`nowplayd` subscribes to every source and publishes
`eds/nowplaying/current`, retained, with the cover on
`eds/nowplaying/current/art`.

- **Arbitration: most recent wins.** When the winner pauses, stops or goes
  offline (its last will), fall back to another source that is still
  playing. `current` is republished only when it actually changes.
- **Enrichment.** When a track is playing, `current` always carries a title,
  an album and art. If the winning source lacks an album or usable art,
  `nowplayd` looks it up, in this order, and caches the answer per
  `(artist, album)`:
  1. art supplied by the source
  2. Deezer album search (no key, 1000 px)
  3. iTunes Search API (no key, about 20 calls a minute)
  4. MusicBrainz, then the Cover Art Archive (1 request a second)
- `nowplayd` knows nothing about lights or stands.

## Consequences

- Drivers stay small: report what the player says and nothing more.
- Subscribers can rely on the three fields and never call a catalogue.
- Art supplied by the player always wins, because a lookup can return the
  wrong edition's cover and the player cannot.
- One lookup per album change, not per track or per poll, keeps well inside
  every catalogue's limits. Misses are cached too. Requests carry a
  descriptive User-Agent.
- `nowplayd` depends on third-party catalogues reachable from the workload.
  When all of them miss, `current` is published without art and says so,
  rather than being withheld.
- Last.fm's images are not used: small, and its terms exclude artwork.
- Paused or stopped playback leaves `current` describing the last track with
  its state. What a subscriber does with "stopped" is the subscriber's
  decision.
- Per-stand assignment is not ruled out. It would extend this record when a
  second stand in a second room exists.

Sources: [Deezer API](https://api.deezer.com/search/album),
[iTunes Search API](https://performance-partners.apple.com/search-api),
[MusicBrainz rate limiting](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting),
[Cover Art Archive API](https://musicbrainz.org/doc/Cover_Art_Archive/API),
[Last.fm API terms](https://www.last.fm/api/tos).

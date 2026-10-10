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

## Evidence

**2026-10-10 — built, and run against the live catalogues.** This record was
accepted on reasoning; these are the things building it showed.

- All three catalogues returned the right cover for a known album (Miles
  Davis, *Kind of Blue*): Deezer 111 KB, iTunes 59 KB, the Cover Art Archive
  49 KB. A track with no album was resolved to its album by Deezer and by
  iTunes, and a track that does not exist was correctly not found.
- **Deezer's field syntax does not work for tracks.** `artist:"…" album:"…"`
  works on its album search; `artist:"…" track:"…"` returns nothing, or
  something unrelated, on both its track search and its general search. Track
  lookups use plain text, which returns the right track first, and check every
  result's artist and title before taking it. The tests against a stand-in
  had passed with the field syntax; only the live service showed it.
- **Catalogues disagree about the album.** For one track, Deezer named the
  single and iTunes the album. The order above stands for covers, which is
  what it was chosen for; for naming a missing album, iTunes may be the better
  first choice. Left as it is until a source without albums is actually in
  use ([ADR-0014](/docs/architecture/decisions/0014-tidal-by-lastfm/)).
- **A catalogue's cover is not byte-stable.** The same Deezer cover fetched
  in two runs had two different hashes. Within a run the cache hides it; a
  restarted nowplayd may publish the "same" cover once as a new one.
- End to end on a development broker: a source state with no album and no
  cover became `current` with both about a second later, and `lightd` lit a
  scene from it. A second source starting another album took over, as the
  rule says, in about four seconds including the lookup.

Sources: [Deezer API](https://api.deezer.com/search/album),
[iTunes Search API](https://performance-partners.apple.com/search-api),
[MusicBrainz rate limiting](https://musicbrainz.org/doc/MusicBrainz_API/Rate_Limiting),
[Cover Art Archive API](https://musicbrainz.org/doc/Cover_Art_Archive/API),
[Last.fm API terms](https://www.last.fm/api/tos).

---
title: "ADR-0014: TIDAL by Last.fm"
weight: 14
---

# ADR-0014: TIDAL Is Read Through Last.fm, Because Nothing Better Is Offered

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until it has been tried against the account; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-10 |
| **Scope** | How the TIDAL driver learns what is playing. |
| **Priority** | The lowest of the four players, by the owner's choice. Built last. |
| **Related** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/), [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/) |

---

## Context

The wish was for TIDAL to work like Spotify: ask the account, from the cloud,
with nothing on the PC, so that it also works away from home.

TIDAL does not offer that. Its API has playback-state resources with a change
event stream, but every one is marked for TIDAL's own clients, and its staff
have said that currently-playing is not planned for third parties.

## Options considered

**TIDAL's official API.** The playback-state endpoints exist
(`/userPlaybackStates`, `/playQueues`) and are refused to third-party apps.
No listening history is offered at any tier.

**TIDAL's API with a first-party client's identity.** The only direct cloud
route, and it means impersonating TIDAL's own client.

**Last.fm as a relay.** TIDAL can be linked to a Last.fm account in its
settings. It then tells Last.fm what is playing, and Last.fm's
`user.getRecentTracks` returns the current track flagged `nowplaying`, with
artist, title and album. It needs only an API key. It is late by the client's
delay plus the poll interval, has no pause or progress, and no usable art.
Users report the desktop link losing scrobbles in 2025–2026.

**The Windows media controls.** The TIDAL desktop client reports to them
natively and immediately, with a thumbnail, but with no album title: an
upstream fix landed in Chromium in May 2026 and whether TIDAL has shipped it
is unknown. It needs Windows-specific code on the PC, and it does not work
away from home.

## Decision

The TIDAL driver polls Last.fm's `user.getRecentTracks` for the `nowplaying`
entry, from the tenant workload. It treats the entry disappearing as
"stopped". The cover is resolved by `nowplayd`
([ADR-0008](/docs/architecture/decisions/0008-nowplayd-arbitrates-and-enriches/)).

**Fallback:** the Windows media controls, in a driver on the PC, if this
proves too slow or too unreliable.

## Consequences

- Nothing is installed on any PC, and it works wherever the rack has
  internet, like Spotify.
- **It is late by design**, by fifteen to thirty seconds. For a light that
  follows the album cover that matters less than it sounds: the cover changes
  only when the album does.
- It depends on two third parties, and on a TIDAL-to-Last.fm link with a
  record of bugs.
- There is no pause state. A paused TIDAL looks like a playing one until
  Last.fm drops the entry, on a schedule Last.fm does not document.
- It needs a Last.fm account linked in TIDAL, and an API key.
- The driver is not really a TIDAL driver: it reports whatever that Last.fm
  account is playing. If another player scrobbles to the same account, the
  two cannot be told apart, so only TIDAL should be linked to it.

## What would settle it

1. The TIDAL desktop client, linked to Last.fm, produces a `nowplaying`
   entry, and how long after the track starts.
2. How long the entry lingers after playback stops or pauses.
3. How often the link drops a track over an evening's listening.
4. Whether polling every fifteen seconds draws any rate limiting.

## Sources

- [TIDAL API reference (OpenAPI)](https://tidal-music.github.io/tidal-api-reference/tidal-api-oas.json) — playback-state operations marked `INTERNAL`
- [TIDAL staff: currently playing "not planned"](https://github.com/orgs/tidal-music/discussions/10)
- [Last.fm: user.getRecentTracks](https://www.last.fm/api/show/user.getRecentTracks)
- [Last.fm: TIDAL scrobbling](https://support.last.fm/t/tidal-scrobbling/181) and [reports of lost scrobbles](https://support.last.fm/t/songs-played-from-tidal-desktop-app-randomly-not-scrobbling/116105)
- [multi-scrobbler: TIDAL as a source is not possible](https://github.com/FoxxMD/multi-scrobbler/issues/357)
- [Chromium: report album name to the Windows media controls](https://github.com/chromium/chromium/commit/8d74ab10c5b8f55c93bf269d25625d72b6c03008) (May 2026)

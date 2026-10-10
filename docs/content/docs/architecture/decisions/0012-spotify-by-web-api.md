---
title: "ADR-0012: Spotify by the Web API"
weight: 12
---

# ADR-0012: Spotify Is Read From the Account, Through the Web API

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until the driver has run against the account; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-10 |
| **Scope** | How the Spotify driver learns what is playing. |
| **Related** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/) |

---

## Context

Spotify knows what an account is playing wherever it plays: start a track on
the desktop and the phone shows it. Spotify must also work when the rack is
away from the house, which rules out anything that depends on the home LAN.

## Options considered

**The Web API.** `GET /v1/me/player` returns the track, album, artist,
progress, play state, the device, and the album's images. It is official and
still available to a personal app. It covers every device on the account. It
is polling only: no webhook or websocket is offered.

**The Windows media controls.** Spotify reports to them natively and needs no
account authorization, but only playback on that PC is seen, the thumbnail
carries Spotify branding, and it needs Windows-specific code running in the
logged-in session.

**The unofficial "dealer" websocket** that Spotify's own clients use. Push,
but it means impersonating a first-party client.

## Decision

The Spotify driver polls `GET /v1/me/player` about every ten seconds, from
the tenant workload, and honours `Retry-After`.

It is authorized once, by hand: a one-off command on the development machine
runs the authorization-code flow against a loopback redirect
(`http://127.0.0.1:PORT/callback`) and writes the refresh token to a
gitignored file that deployment copies to the workload.

## Consequences

- Nothing is installed on any PC, and playback on a phone or a speaker is
  seen too.
- It works wherever the rack has internet.
- Album art arrives with the track, from Spotify.
- **The owner needs Spotify Premium.** A development-mode app stops working
  if it lapses.
- **Authorization expires six months after it was granted**, and refreshing
  does not extend it. On `invalid_grant` the driver reports that on its
  status topic and stops, rather than retrying; re-authorizing is a recorded
  change.
- The refresh token is a secret the workload holds in a file. The workload is
  rebuilt empty, so it is redeployed from the development machine's copy.
- Track changes are seen up to one poll interval late.
- The development-mode quota is unpublished and shared across the owner's
  apps.
- Spotify's terms allow caching cover art only as needed, not indefinitely.
- Spotify's API pages carry the line *"You may not synchronize any sound
  recordings with any visual media."* Whether a private light that follows a
  cover falls under it is the owner's judgement. It is recorded here so the
  question is not rediscovered.

## What would settle it

1. A development-mode app owned by the account gets `/v1/me/player` with
   the two playback scopes.
2. Playback on the desktop client, and on the phone, appears within a poll.
3. What the API returns during a private session.
4. How it behaves at ten-second polling over a day: any `429`.

## Sources

- [Get playback state](https://developer.spotify.com/documentation/web-api/reference/get-information-about-the-users-current-playback)
- [February 2026 changes](https://developer.spotify.com/documentation/web-api/references/changes/february-2026) — player endpoints listed as still available; Premium required of the app owner
- [Refresh tokens expire after six months](https://developer.spotify.com/blog/2026-06-18-refresh-token-expiration) (June 2026)
- [Redirect URI rules](https://developer.spotify.com/documentation/web-api/concepts/redirect_uri) — loopback IP literal, not `localhost`
- [Rate limits](https://developer.spotify.com/documentation/web-api/concepts/rate-limits)
- [Quota modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes) — a personal app stays in development mode
- [Developer terms](https://developer.spotify.com/terms)

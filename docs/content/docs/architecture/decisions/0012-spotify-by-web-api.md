---
title: "ADR-0012: Spotify by the Web API"
weight: 12
---

# ADR-0012: Spotify Is Read From the Account, Through the Web API

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} 2026-10-11, once the driver had run against the account; two observations are still wanted, under [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-10 |
| **Scope** | How the Spotify driver learns what is playing. |
| **Amended** | 2026-10-11, before it was accepted: where the driver runs, how it is authorized, which devices count, and that it takes no artwork. The text below is the amended decision; [What changed](#what-changed-on-2026-10-11) lists the differences |
| **Extended by** | [ADR-0031](/docs/architecture/decisions/0031-spotify-is-shown-like-itunes/): the driver takes the album's picture after all, from the answer it already asks for. [ADR-0032](/docs/architecture/decisions/0032-spotify-takes-commands/): the authorization also allows changing playback, and the driver carries out commands |
| **Related** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/), [ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/) |

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

The Spotify driver polls `GET /v1/me/player` every ten seconds and honors
`Retry-After`. It runs **in the agent, on the agent Pi**, beside the iTunes
driver.

**It is authorized once, by hand, with PKCE.** A command on a machine with a
browser runs the authorization-code flow against a loopback redirect
(`http://127.0.0.1:8898/callback`). PKCE needs no client secret, so the app's
secret is never used or stored. The result is a refresh token in a file,
which is *moved* to the Pi.

**It asks to read playback and nothing else**: `user-read-playback-state` and
`user-read-currently-playing`. Controlling playback is a further permission
and is not asked for until something uses it.

**Only named devices count.** The account plays wherever its owner is. The
driver is told which devices are in the room (`NP_SPOTIFY_DEVICES`), and
playback anywhere else is reported as `stopped`.

**It reports a track, or nothing.** An advertisement, a podcast episode, and
anything played in a private session are reported as `stopped`.

**It takes no artwork.** The driver never requests an image
([ADR-0016](/docs/architecture/decisions/0016-a-source-can-be-reported-without-its-cover/)).

## Consequences

- Nothing is installed on any PC.
- It works wherever the Pi has internet.
- **The owner of the app needs Spotify Premium.** Without it every request is
  refused with `403`, and the refusal says so. The driver reports itself
  offline and asks again every five minutes; Spotify says a change of
  subscription takes hours to be noticed.
- **Authorization expires six months after it was granted**, and refreshing
  does not extend it. The driver warns in its log from two weeks before. On
  `invalid_grant` it reports itself offline and stops, rather than retrying;
  re-authorizing is `make spotify` again.
- **The refresh token has one holder.** Spotify may replace it on any refresh,
  and the one it replaced stops working. That is why the driver is on the Pi,
  whose disk persists, and not on the workload, which is rebuilt empty; and
  why the file is moved from the desk, not copied.
- The app's Client ID is not a secret under PKCE. It still names one person's
  app, and this repository is public, so it is kept out of it.
- Track changes are seen up to one poll interval late.
- A device is known by its name. Renaming a device in Spotify silently stops
  it counting.
- The development-mode quota is unpublished and shared across the owner's
  apps.
- Spotify's API pages carry the line *"You may not synchronize any sound
  recordings with any visual media."* With the lights not following Spotify
  and its artwork never fetched, this design steps around that question rather
  than answering it.

## What changed on 2026-10-11

| | As first proposed | As amended |
|---|---|---|
| Where it runs | the tenant workload | the agent Pi, which keeps a file across rebuilds |
| Authorization | authorization code, token copied to the workload | authorization code with PKCE, read-only, token moved to the Pi |
| Which playback | every device on the account | named devices only |
| Artwork | arrives with the track, from Spotify | never requested |

## What would settle it

1. A development-mode app owned by the account gets `/v1/me/player` with
   the two playback scopes. **Settled 2026-10-11.** An app owned by an account
   without Premium was authorized and then refused every request with `403
   Active premium subscription required for the owner of the app`. One owned
   by the Premium account was answered at once.
2. Playback on the desktop client appears within a poll, and playback on the
   phone is told apart from it. **Settled 2026-10-11.** Four tracks on the
   named computer were each reported; moving playback to the phone was
   reported as `stopped`
   ([CHG-0004](/docs/changes/2026/0004-spotify-on-the-agent-pi/)).
3. What the API returns during a private session. *Not yet observed.* The
   driver treats the device's private-session flag as `stopped`; whether
   Spotify sends the track at all in that case is unknown.
4. How it behaves at ten-second polling over a day: any `429`. *Not yet
   observed.*

## Sources

- [Get playback state](https://developer.spotify.com/documentation/web-api/reference/get-information-about-the-users-current-playback)
- [February 2026 changes](https://developer.spotify.com/documentation/web-api/references/changes/february-2026) — player endpoints listed as still available; Premium required of the app owner
- [Refresh tokens expire after six months](https://developer.spotify.com/blog/2026-06-18-refresh-token-expiration) (June 2026)
- [Redirect URI rules](https://developer.spotify.com/documentation/web-api/concepts/redirect_uri) — loopback IP literal, not `localhost`
- [Rate limits](https://developer.spotify.com/documentation/web-api/concepts/rate-limits)
- [Quota modes](https://developer.spotify.com/documentation/web-api/concepts/quota-modes) — a personal app stays in development mode
- [Developer terms](https://developer.spotify.com/terms)

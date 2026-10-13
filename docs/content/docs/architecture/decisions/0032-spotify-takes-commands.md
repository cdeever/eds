---
title: "ADR-0032: Spotify Takes Commands"
weight: 32
---

# ADR-0032: The Spotify Driver Carries Out Commands, With a Wider Authorization, for What Is Playing Here

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a press on the cover screen has paused Spotify; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-13 |
| **Scope** | Whether a controller's command reaches Spotify, what the agent must be authorized for to carry it out, and which playback a command may touch. |
| **Extends** | [ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/), whose commands only iTunes carried out. [ADR-0012](/docs/architecture/decisions/0012-spotify-by-web-api/), whose authorization only read. |
| **Related** | [ADR-0031](/docs/architecture/decisions/0031-spotify-is-shown-like-itunes/), which put Spotify's sleeve on the cover screen above buttons that did nothing; [ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/), the other answer to that; [ADR-0027](/docs/architecture/decisions/0027-what-is-asked-of-outside-services-is-counted/) |

---

## Context

Since [ADR-0031](/docs/architecture/decisions/0031-spotify-is-shown-like-itunes/)
the cover screen shows what Spotify is playing, with the same five buttons
it shows for iTunes. Pressed, they do nothing: `nowplayd` passes the command
to the current source
([ADR-0020](/docs/architecture/decisions/0020-a-touch-screen-that-can-change-what-plays/)),
and Spotify's driver reads no command topic. It was built to read, with an
authorization that only reads, and both the code and the broker account say
that changing it is a decision. This is that decision.

## Options considered

**Hide the buttons for Spotify**, as for the turntable
([ADR-0025](/docs/architecture/decisions/0025-current-says-when-there-are-no-controls/)).
One word in a setting. Honest, and it leaves a screen beside the music that
cannot pause it, for a player that can be paused.

**Control the playing device directly** - Spotify Connect, as the phone's
own remote does it. No published protocol, and a different one per device.

**Carry the commands out through the Web API.** The same service the driver
already asks, with one more permission: `user-modify-playback-state`.

## Decision

**The Spotify driver carries out the five commands through Spotify's Web
API. The agent asks for one more scope, and a command is carried out only
for playback on a device that counts.**

- **The scope added is `user-modify-playback-state`**, and no other. It
  allows play, pause, skip, seek, volume and choosing a device. It allows
  nothing about the account, its library or its playlists. The authorization
  has to be given again for it; one given before only reads, and goes on
  reading.
- **The commands are the five there are**: pause or play
  (`PUT /v1/me/player/pause`, `…/play`), `POST …/next`, `POST …/previous`,
  and `PUT …/volume` five points up or down from where the last answer said
  it was. No command is added.
- **A command is for what is playing here.** Spotify's "the player" is
  whichever device the account last used, wherever it is. The driver sends
  nothing unless its last answer was a track on a device named in
  `NP_SPOTIFY_DEVICES`. A press on the screen does not pause a phone that
  does not count.
- **A device that does not let its volume be set is told nothing about
  volume**, and the press is logged as failed. Spotify says which devices
  those are, and a phone is often one.
- **What a command did is believed until Spotify is next asked**, which is
  seven tenths of a second later and not at the next ten seconds: two
  presses of pause are a pause and a play.
- **The agent's broker account may read
  `nowplaying/source/spotify/command`.** `nowplayd` already may publish to
  any source's.
- **The console still only looks**, and a controller still names no source.

## Consequences

- **The cover screen's buttons work for Spotify**, on the computer and on
  the phone.
- **EdS can now change something on an account at a company**, where it
  could only look. The token that allows it is on the agent Pi, in a file
  only the agent's user reads, as before; what it would let a thief do is
  now pause the music, not just see it.
- **Commands need Spotify Premium**, as the reading already did.
- **A press is a request to Spotify's servers**, counted with the driver's
  others under `spotify`. A person pressing buttons is far inside any limit.
- **A press arrives up to a few hundred milliseconds later than for
  iTunes**, which is asked on the home network.
- **A track changed from the phone itself is still seen at the next poll**,
  up to ten seconds on. Only a command from here hurries it.
- **The authorization is given again by a person**, in a browser, once now
  and every six months as before.
- **With the phone counting, a press at home acts on the phone wherever it
  is.** That is what counting means, and the list is a setting.

## What would settle it

Tested against a stand-in for Spotify: each command as the request Spotify
documents; nothing sent when nothing counts here; volume refused where it
cannot be set; a refusal passed on as Spotify worded it; a stale token
refreshed once.

Not yet shown:

- [ ] Each of the five buttons, with Spotify playing on the computer.
- [ ] The same on the phone, and what its volume does.
- [ ] How long from a press to the screen showing what it did.

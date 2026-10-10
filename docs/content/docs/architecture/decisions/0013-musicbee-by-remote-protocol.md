---
title: "ADR-0013: MusicBee by the Remote Plugin's Protocol"
weight: 13
---

# ADR-0013: MusicBee Is Read Through the MusicBee Remote Plugin's Protocol

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — the protocol has been read and does what is needed; it has not been tried against a real MusicBee. See [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-10 |
| **Scope** | How the MusicBee driver learns of track changes and gets artwork. |
| **Related** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/), [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/), [ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/) |

---

## Context

MusicBee plays local files and has no cloud. Anything that reads it runs on
or near the PC. Its extension point is a plugin: a .NET DLL loaded into
MusicBee, with a notification when the track changes.

The drivers are to be portable Go with no .NET
([ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/)).

## Options considered

**Our own MusicBee plugin** that publishes to MQTT. Exact and immediate, and
a small one exists to learn from. But it is .NET, loaded into MusicBee's own
process, with an MQTT client and TLS inside it.

**The Windows media controls.** MusicBee reports to them only through a
third-party plugin last updated in 2024, with open bugs: missing art for most
tracks, hotkey conflicts, a crash on portable installs.

**The MusicBee Remote plugin's protocol.** An existing, maintained plugin
(v1.5.0, August 2026) that exists so a phone can see and control MusicBee
over the network, with its protocol documented in its repository. This is the
same shape as the iTunes decision: someone else's maintained remote-control
surface, read by a small driver of ours.

**Scrobbling.** MusicBee can submit "now playing" to ListenBrainz through a
plugin. Late, and with no art or pause state.

## Decision

The MusicBee driver connects to the **MusicBee Remote plugin** and speaks its
protocol. The plugin is installed in MusicBee; nothing of ours is.

**Fallback:** a plugin of our own, which would mean a .NET component and a
new record extending
[ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/).

## Consequences

- No .NET, and the driver runs on the relay rather than the PC.
- The plugin has to be installed in MusicBee, and its port admitted by the
  PC's firewall.
- The driver depends on a third party's protocol staying compatible. The
  plugin is actively maintained, and the protocol is versioned.
- The driver speaks the plugin's **legacy protocol (V4)**, not its newer one;
  see the evidence below.

## Evidence so far

**2026-10-10 — the protocol documents, read at the repository's `main`.** Not
yet tried against a running MusicBee.

The plugin serves two protocols on one TCP port (default 3000) and tells them
apart by the first frame.

| | V4 (legacy) | V6 |
|---|---|---|
| In a release | every shipping version, including v1.5.0 | **no** — on `main` only, 181 commits past v1.5.0 |
| Stability | frozen, "preserved byte-for-byte, not extended" | "active development" |
| Framing | JSON objects ended by CRLF | JSON objects ended by newline, with an envelope and ids |
| Track change | pushed: `nowplayingtrack` with artist, title, album, year, path | pushed: `now_playing_changed`, then re-query `now_playing_state` |
| Play state | pushed: `playerstate` — `Playing`, `Paused`, `Stopped` | pushed: `play_state_changed` |
| Cover | pushed: `nowplayingcover`, the image as base64 | a content hash on the track; fetched from `GET /api/cover/{hash}` |
| Keepalive | the server pings | the client pings every 15 s |
| Access | an address filter | optional pairing, off by default |

So, against the questions below: (1) yes, track changes are pushed in both;
(2) title, artist and album are supplied, and the cover as base64 in V4;
(3) yes, three states; (4) no authentication by default.

**What this decides for the driver:** it speaks **V4**. V4 is what the
installed plugin has, it is frozen, and it pushes everything needed. V6 is
the better protocol, with a typed track and a cover that can be cached by
hash, but it is not released. Moving to it later would extend this record.

## What would settle it

1. ~~The protocol pushes a message when the track changes, without polling.~~
   Yes, on paper.
2. ~~It supplies title, artist and album, and the cover, and in what form.~~
   Yes, on paper; the cover is base64 in V4.
3. ~~Play, pause and stop are distinguishable.~~ Yes, on paper.
4. ~~Whether a client must authenticate, and how.~~ Not by default.
5. All of the above against a running MusicBee with the plugin installed.
6. Whether the plugin's address filter admits the relay by default.
7. Behaviour when MusicBee quits and restarts.

## Sources

- [MusicBee Remote plugin](https://github.com/musicbeeremote/mbrc-plugin) — v1.5.0, 2026-08-31
- [Its protocol overview](https://github.com/musicbeeremote/mbrc-plugin/blob/main/docs/protocol.md), [V4](https://github.com/musicbeeremote/mbrc-plugin/blob/main/docs/protocol-v4.md) and [V6](https://github.com/musicbeeremote/mbrc-plugin/blob/main/docs/protocol-v6.md)
- [Musicbee-MQTT](https://github.com/TroyFernandes/Musicbee-MQTT) — a small plugin that publishes now-playing to MQTT; the model for the fallback
- [mb_MediaControl issues](https://github.com/HenryPDT/mb_MediaControl/issues) — the media-controls plugin's open bugs
- [ScrobblerBrainz](https://github.com/karaluh/ScrobblerBrainz) — "now playing" to ListenBrainz

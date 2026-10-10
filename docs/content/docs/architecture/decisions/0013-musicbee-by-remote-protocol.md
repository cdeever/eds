---
title: "ADR-0013: MusicBee by the Remote Plugin's Protocol"
weight: 13
---

# ADR-0013: MusicBee Is Read Through the MusicBee Remote Plugin's Protocol

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — the protocol has not yet been read, let alone tried; see [What would settle it](#what-would-settle-it) |
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
- This record rests on less evidence than the other three: the protocol's
  documents have been located but not read.

## What would settle it

1. The protocol pushes a message when the track changes, without polling.
2. It supplies title, artist and album, and the cover, and in what form.
3. Play, pause and stop are distinguishable.
4. Whether a client must authenticate, and how.
5. Behaviour when MusicBee quits and restarts.

## Sources

- [MusicBee Remote plugin](https://github.com/musicbeeremote/mbrc-plugin) — v1.5.0, 2026-08-31; protocol under `docs/`
- [Musicbee-MQTT](https://github.com/TroyFernandes/Musicbee-MQTT) — a small plugin that publishes now-playing to MQTT; the model for the fallback
- [mb_MediaControl issues](https://github.com/HenryPDT/mb_MediaControl/issues) — the media-controls plugin's open bugs
- [ScrobblerBrainz](https://github.com/karaluh/ScrobblerBrainz) — "now playing" to ListenBrainz

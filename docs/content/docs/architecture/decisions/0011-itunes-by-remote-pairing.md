---
title: "ADR-0011: iTunes by Remote Pairing"
weight: 11
---

# ADR-0011: iTunes Is Read by Remote Pairing, the Way the Phone Remote Does It

| | |
|---|---|
| **Status** | {{< adr-status "Proposed" >}} — until a driver has paired with the real iTunes; see [What would settle it](#what-would-settle-it) |
| **Date** | 2026-10-10 |
| **Scope** | How the iTunes driver learns of track changes and gets artwork. |
| **Related** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/), [ADR-0007](/docs/architecture/decisions/0007-drivers-are-portable-go/), [ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/) |

---

## Context

The PCs run **classic iTunes for Windows**, not the Microsoft Store build and
not the newer Apple Music app. A phone remote app already sees and controls
it, after a one-time pairing and with no Apple ID sign-in. That pairing has to
keep working for as long as Apple supports it.

## Options considered

**The remote protocol (DACP).** What phone remotes speak: HTTP on port 3689,
found by Bonjour. A remote advertises itself, the user types its four-digit
code into iTunes once, and from then on it logs in with a stored pairing
GUID. A request to `playstatusupdate` carrying the last revision number
**hangs until something changes**, which is push notification without
polling. Artwork is one more request. It works from any machine on the same
LAN, in any language.

**The COM automation interface.** `iTunes.Application` and its events. No
pairing, and it still appears to work: two tools that depend on it shipped
releases in 2025. But it is Windows-only and must run on the iTunes PC, its
play event is reported not to fire reliably so those tools poll as well, and
creating the object launches iTunes unless the process is checked for first.

**The Windows media controls.** Classic iTunes does not report to them; it
would need a separate bridge program that itself uses COM.

**Home Sharing.** The protocol's other login, tied to an Apple ID. Not
wanted, and probably closed to third-party code.

## Decision

The iTunes driver speaks the remote protocol and **pairs the way the phone
remote does**: a four-digit code typed into iTunes once, never an Apple ID or
Home Sharing. It registers as an additional remote and does not disturb one
already paired.

Its loop: log in with the stored pairing GUID; long-poll
`playstatusupdate`; on each return publish the state; fetch
`nowplayingartwork` when the album changes; on any error report offline, back
off and start again.

**Fallback:** the COM interface, in a Windows-only driver behind the same
interface, if pairing proves closed.

## Consequences

- The driver need not run on the iTunes PC. It runs on the relay
  ([ADR-0010](/docs/architecture/decisions/0010-home-lan-relay/)).
- It can be developed on a Mac against the Music app, if that speaks the
  same protocol — unverified.
- Track changes arrive pushed, with title, artist, album, play state and
  times, and the artwork comes from the player itself.
- Setup costs one code typed into iTunes, and the firewall on the PC must
  admit iTunes and Bonjour.
- The pairing GUID is state the driver must keep; lose it and pairing is
  repeated.
- The driver must detect a dead connection itself. What the long-poll does
  when iTunes quits or the PC sleeps is not documented.
- It works only while the PC stays on classic iTunes. The Apple Music app
  for Windows supports neither the Remote app nor Home Sharing, and
  installing it reduces iTunes to podcasts and audiobooks.
- There is no Go implementation to reuse; the client is written here, with
  [pyatv](https://github.com/postlund/pyatv)'s `dmap` module as the reference.

## What would settle it

Recorded here when tried. Each is a check against the real player:

1. Classic iTunes 12.13 shows the Remote button and accepts a pairing from a
   third-party remote. One user report says the button never appeared on
   12.13.3.2.
2. The phone remote still works afterwards.
3. The long-poll returns on a track change, and on pause.
4. Artwork is returned, and in what format and size.
5. The Music app on macOS accepts the same pairing.
6. What happens to the connection when iTunes quits or the PC sleeps.

## Sources

- [Apple: use the iTunes Remote app (iTunes 12.13 for Windows)](https://support.apple.com/en-au/guide/itunes/itnsa1c27e74/12.13/windows/10) — both pairing modes still documented
- [DACP, reverse-engineered](http://dacp.jsharkey.org/) (2008) — pairing, login, the long-poll, the response tags
- [pyatv protocol notes](https://pyatv.dev/documentation/protocols/) and [its pairing code](https://github.com/postlund/pyatv/blob/master/pyatv/protocols/dmap/pairing.py)
- [OwnTone's server side](https://github.com/owntone/owntone-server/blob/master/src/httpd_dacp.c) — confirms the long-poll's behaviour
- [Report: Remote button missing on 12.13.3.2](https://discussions.apple.com/thread/255771984)
- [Apple Music for Windows does not support the Remote app](https://discussions.apple.com/thread/255807548)
- [iTunes-SMTC](https://github.com/thewizrd/iTunes-SMTC/blob/master/iTunes.SMTC/iTunes/iTunesController.cs) — COM in use in 2025, and "the iTunes OnPlayerPlayEvent does not always fire"

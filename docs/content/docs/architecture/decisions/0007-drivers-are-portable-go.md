---
title: "ADR-0007: Drivers Are Portable Go"
weight: 7
---

# ADR-0007: Drivers Are Portable Go, in One Agent Binary

| | |
|---|---|
| **Status** | {{< adr-status "Accepted" >}} |
| **Date** | 2026-10-10 |
| **Scope** | The language and packaging of the now-playing drivers and backend. |
| **Related** | [ADR-0006](/docs/architecture/decisions/0006-now-playing-bus-of-drivers/) |

---

## Context

The players run on Windows 11, which pulls toward Windows tooling. But
development happens on a Mac against the Deevnet Mobile site, some drivers
will run on a Raspberry Pi and some on the tenant workload, and `lightd` is
already Go.

## Options considered

**C#/.NET.** First-class access to the Windows media controls and to
iTunes's COM interface, and MusicBee plugins must be .NET anyway. But it ties
the drivers to Windows, cannot be developed against the Mac's Music app, and
adds a second service toolchain.

**Rust.** Good Windows bindings and portable, but a third language in a
small repository.

**Go.** One language with `lightd`; cross-compiles from the Mac to Windows,
Linux and ARM with no extra toolchain. Reading the Windows media controls
from Go is possible but needs hand-written bindings
([winrt-go](https://github.com/saltosystems/winrt-go),
[an example](https://github.com/soarqin/smtc-now-playing)).

The choice of mechanisms removed most of the pull toward Windows: the iTunes
and MusicBee drivers speak network protocols, and the Spotify and TIDAL
drivers speak web APIs
([0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/)–[0014](/docs/architecture/decisions/0014-tidal-by-lastfm/)).
None of them needs to run on the PC that plays the music.

## Decision

- The drivers and the backend are **Go, in one module**,
  `services/nowplaying`.
- **One agent binary, `npagent`**, contains every driver and runs the ones
  named in `NP_DRIVERS`. A small deployment is that binary with one driver
  switched on.
- **No .NET, and nothing Windows-specific**, in the initial build.

## Consequences

- The same binary is the development setup on the Mac, the relay on a Pi and
  the cloud drivers on the workload.
- A driver can be developed against the Music app on macOS, which speaks the
  same remote protocol as iTunes — to be confirmed
  ([ADR-0011](/docs/architecture/decisions/0011-itunes-by-remote-pairing/)).
- The module follows `lightd`'s conventions: configuration from the
  environment only, the platform's `kit.env` names as fallbacks, `slog`, the
  paho client, fakes behind small interfaces. `lightd`'s helpers are
  `internal` to its module, so they are copied rather than imported.
- If a mechanism fails and its fallback is Windows-only — iTunes's COM
  interface, or the Windows media controls for TIDAL — that driver would be
  Go with hand-written Windows bindings, behind the same interface. This
  record would be extended, not superseded.
- MusicBee is read through an existing plugin's protocol rather than a plugin
  of our own
  ([ADR-0013](/docs/architecture/decisions/0013-musicbee-by-remote-protocol/)),
  which is what keeps .NET out.

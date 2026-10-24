---
title: "CHG-0033: An Album Asked For on the Bus"
weight: 33
---

# CHG-0033: An Album Asked For on the Bus

| | |
|---|---|
| **Date** | 2026-10-23 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | The tenant's services workload (`services.eds.mobile.deevnet.net`): the four services rebuilt and restarted. `dv02rpi002p01`, the relay: `npagent` replaced |
| **How** | `make -C deploy ship WHERE=builder` and `make -C deploy/relay push`, from the branch `voice-play-album` |
| **Risk** | Low. Most likely to go wrong: a service not coming back, which leaves the mobile site's stand and screen without a current track until shipped again. The home back-end Pi is not touched |
| **Decisions** | [ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/), whose command this puts on the bus |
| **Related** | [CHG-0032](/docs/changes/2026/0032-american-idiot-has-a-look/), the last change, made on the back-end Pi |

---

## Summary

`play_album` is the first command that chooses what plays
([ADR-0040](/docs/architecture/decisions/0040-a-request-can-name-an-album-to-play/)).
`npagent play-album` has already played Van Halen's first album from the
relay by hand; this puts the same request behind the bus. nowplayd passes
`play_album` to `itunes` whatever is current, and the iTunes driver asks
for the album and then asks what plays.

Nothing in the settings changes: `NP_ALBUM_SOURCE` defaults to `itunes`.
The workload is shipped from `main` with this branch's two commits on it,
so it also takes whatever has merged since it was last shipped.

## Goal

- The four services are active after the ship, and `npagent` after the push.
- `{"v":1,"command":"play_album","artist":"Van Halen","album":"Van Halen"}`
  on `eds/nowplaying/source/itunes/command` plays the album, and `current`
  says *Van Halen*.
- An album the library does not have is logged as `driver.command_failed`,
  "the library played no album by those names".

## Procedure

1. `make -C deploy ship WHERE=builder`.
2. `make -C deploy status`.
3. `make -C deploy/relay push`, then `make -C deploy/relay status`.
4. Publish the command above with nowplayd's account, which may publish to
   a source's command topic. No account yet may publish `play_album` to
   `nowplaying/command` but the cover screen's; nowplayd's routing of it is
   tested in `internal/command` and is proved when its sender has an account.
5. Publish one for an album not in the library.

## Undo

Ship and push `main` again: `git switch main`, then the two commands above.

## Outcome

| When (UTC) | What happened |
|---|---|
| 00:00:45–00:02:25 | `ship WHERE=builder` from `22889ba`: the four images built on the Builder and loaded; all four services active at 00:02:24. nowplayd at once had iTunes playing *Van Halen*, "Ain't Talkin' 'Bout Love", and listed the Discogs collection (227 records). |
| 00:02:49 | `make -C deploy/relay push`: installed `22889ba`; `npagent` active, iTunes followed again. |
| 00:02:57 | `play_album` for *Aerosmith* by Aerosmith, asked for by the owner, published with nowplayd's account on `eds/nowplaying/source/itunes/command`. Within a second the driver reported *Aerosmith*, "Make It", the album's first track, and nowplayd published it as `current`. At 00:02:59, after the check, the driver logged `command: play_album` as done. |

### Departures from the plan

- Step 5, an album the library does not have, was not run: it would have
  emptied the queue under the album the owner had just asked for.

## Follow-ups

- Step 5, when nothing is being listened to: what iTunes does with an
  album it does not have, and that the driver logs it.
- An account that may publish `nowplaying/command` for whatever sends
  `play_album` by voice, in the tenant's Terraform.

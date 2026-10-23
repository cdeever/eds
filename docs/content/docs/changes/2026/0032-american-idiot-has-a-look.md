---
title: "CHG-0032: American Idiot Has a Look"
weight: 32
---

# CHG-0032: American Idiot Has a Look

| | |
|---|---|
| **Date** | 2026-10-23 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-23 |
| **Systems** | `dv02rpi003p01`, the back-end Pi: nowplayd and lightd replaced, and the four services restarted |
| **How** | `make -C deploy/backend push PI=192.168.2.40`, from #60's branch with this change on it |
| **Risk** | Low. Most likely to go wrong: a service not coming back after the push, which leaves the stand dark until it is pushed again |
| **Decisions** | [ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/), a second album on its list |
| **Related** | [CHG-0030](/docs/changes/2026/0030-the-console-switches-the-stand/), whose branch is what runs on the Pi and is pushed again here |

---

## Summary

Green Day's *American Idiot* was lit from the owner's own pressing once the
record was added to the Discogs collection. That sleeve's picture is a
photograph, and lightd read it as one red in four shades
(`#df0040 56%, #b90237 16%, #92122f 14%, #671c2b 14%`), which a strip of LEDs
shows as one red going nowhere. Deezer's clean scan of the same sleeve, a few
minutes earlier, had read as two reds with the pale pink of the hand and a
salmon among them (`#d90018 45%, #e22f28 31%, #efb6b0 13%, #e35f55 12%`), and
its owner liked that better.

The album gets a look of its own, `grenade`: those four colors, kept, and
shown with the stand's usual effect, speed and brightness. The cover shown
everywhere else stays the owner's pressing.

## Goal

- `make status` is clean after the push, and the console's version is the
  pushed tree's.
- A track from *American Idiot* on the turntable: nowplayd publishes the look
  `grenade`, and lightd publishes a `chase` with `#d90018` first and
  `#efb6b0` among the accents.
- The cover is the owner's (`art_from=discogs`).

## Procedure

1. `make -C deploy/backend push PI=192.168.2.40`, from a worktree of #60's
   branch with this change cherry-picked onto it.
2. `make -C deploy/backend status PI=192.168.2.40`.
3. A track from *American Idiot*, by the owner.

## Undo

Push #60's branch again, as it was:
`make -C deploy/backend push PI=192.168.2.40` from `9fdd99a`.

## Outcome

| When (UTC) | What happened |
|---|---|
| 03:05–03:06 | `push`, from a worktree of #60's branch (`94700c9`, a record ahead of `9fdd99a`) with this change cherry-picked: installed `94700c9-dirty`; palette, lightd, nowplayd, the console and `npagent` active. |
| 03:06:24 | *Jesus of Suburbia*, *American Idiot*, from the owner's collection (`art_from=discogs`). lightd: `look=grenade effect=chase colors=4 palette="#d90018 45%, #e22f28 31%, #efb6b0 13%, #e35f55 11%"`. |
| 03:06:25 | The turntable stopped: `npagent` had been restarted by the push, and its driver starts out saying so. The stand went to standby. The side had ended. |
| 03:12:54 | Another record, by the owner: *Turn to Stone*, *Out Of The Blue*, from the collection, lit from its own cover, `chase` in four colors. Nothing but *American Idiot* has the look. |

### Departures from the plan

- `make status` was not run; `push` reported every service active, and the console's version was `94700c9-dirty`.

## Follow-ups

- None.

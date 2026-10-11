---
title: "CHG-0010: A Look for One Album, and a Strip of Eighteen"
weight: 10
---

# CHG-0010: A Look for One Album, and a Strip of Eighteen

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | the `eds` tenant: two broker accounts gain a topic; `eds-nowplayd`, `eds-lightd` and `eds-console` on the `services` workload; `lp-stand-01`, over USB |
| **How** | `terraform apply`; `make -C firmware/lp-stand build flash`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: a grant that does not match the topic, which the broker refuses without saying so and which leaves every album lit from its cover as before |
| **Decisions** | [ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/) |
| **Related** | [CHG-0009](/docs/changes/2026/0009-two-effects-for-the-accents/) |

---

## Summary

Two things, deployed together because they touch the same three programs.

*The Dark Side of the Moon* is a black sleeve with a thin spectrum across it,
and its cover's colours are not what that album should light a room with.
After this change `nowplayd` says that album has a look, `spectrum`, and
`lightd` lights its cover as six colours from red to violet instead of the
cover's own.

The stand has room for 18 LEDs across its front, not the 60 the firmware
assumed. The firmware and the console's drawing both become 18.

## Goal

- `nowplayd` publishes `eds/nowplaying/current/look` beside every cover.
- `lightd` logs that it is listening for looks, and lights *The Dark Side of
  the Moon* as `spectrum`: six colours, the effect in
  `LIGHTD_SPECTRUM_EFFECT`.
- Every other album is lit from its cover as before, including the one played
  straight after.
- The stand reports 18 LEDs at start, and the console draws 18.

## Scope

**In scope:** one topic granted to two accounts; the three services rebuilt;
the stand reflashed for the strip's length and wider `chase` patches.

**Out of scope:** the scene contract, which is unchanged: a look reaches the
stand as an ordinary scene with an effect it already has. Spotify, which has
no cover published and so no look. The LEDs themselves, still not attached.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| A grant does not match the topic | the broker | both are written as the exact topic; a miss is silent, and shows as `lightd` never logging a look. Nothing is worse than before |
| An account is replaced, not updated | the tenant | the plan is read first. A replacement would reissue a password; stop if it shows one |
| The look lands on the wrong album | the room | a look names its cover by hash and is applied to no other; tested in both arrival orders |
| The tags on this copy do not match | `nowplayd` | then it is lit from its cover, as now. The match is on the album starting with the title and the artist including the band |
| The console draws a strip the stand does not have | the page | both are 18 by default, and the console's is a setting |

## Procedure

```bash
make -C infra/deevnet-tenant-eds plan      # two accounts updated in place
make -C firmware/lp-stand test
make -C firmware/lp-stand build flash PORT=/dev/cu.usbserial-0001
make -C deploy deploy WHERE=builder
```

## Verification

- `eds/nowplaying/current/look` holds a retained message naming the current
  cover's hash.
- `lightd` logs `listening for looks`.
- The stand's boot log names 18 LEDs.
- The console's strip has 18 segments.
- *The Dark Side of the Moon* played in iTunes: `lightd` publishes a scene of
  six colours with `look=spectrum`, and the stand applies it. The next album
  is lit from its cover.

## Undo

Remove `LIGHTD_LOOK_TOPIC` from `deploy/lightd.env` and run `make -C deploy
units`: every album is lit from its cover. The grants can stay. The strip's
length is `LP_LED_COUNT` and `CONSOLE_STRIP_LEDS`.

## Outcome

| When (UTC) | What happened |
|---|---|
| 19:43 | Host tests passed, 5421 checks. Built with `LP_LED_COUNT=18` and flashed. The stand restarted as `9d39658`, rejoined and applied the retained scene. |
| 19:45 | Deployed. `lightd` logged `listening for looks topic=eds/nowplaying/current/look spectrum=sweep`. The console's page carried `strip_leds: 18`. The album paused in iTunes was *Dark Side Of The Moon*, and it was lit from its cover: the grants were not in yet. |
| 19:47 | The plan showed the `lightd` and `nowplayd` accounts updated in place, one topic each, nothing replaced. Applied by the tenant's owner. `lightd` and `nowplayd` restarted. |
| 19:49 | A track from *Dark Side Of The Moon* played in iTunes. `lightd` published `look=spectrum effect=sweep colors=6`, and the stand applied it 0.3 s after the agent reported the track: `Showing sweep in 6 colours`, red to violet, 17% each. |
| 20:37 | *Abacab* played next: lit from its cover, `chase` in three colours. Back to *Dark Side Of The Moon* at 20:38: the spectrum again. Back to *Abacab*: its cover again. A look did not outlive its album in either direction. |

Every check under Verification holds, from the logs and the stand's own
reports. No LEDs are attached, so nobody has seen the spectrum on a strip.

### Departures from the plan

- **The apply came after the deployment, not before.** For two minutes
  `nowplayd` published looks the broker refused without saying so. Nothing
  was lit wrongly; the album was lit as it would have been yesterday.
- **The tag on this copy has no leading "The".** The rule allows for it, and
  it matched.
- **The console's page went stale after sitting idle, and this was fixed
  here.** With iTunes paused for three quarters of an hour, the page did not
  show the next track for half a minute, and a reload took as long. The
  services were not late: each track was on the bus and on the stand within
  a second of the agent reporting it. The page's stream had died without the
  browser knowing, and the page had no way to notice, because the console's
  keep-alive was a comment a page cannot see. The keep-alive is now an event
  every 15 seconds, and a page that hears nothing for 40 reconnects, as it
  does on coming back to the tab or the network. That the stream was dead is
  inferred from the timings, not observed.

## Follow-ups

- [ ] Look at `spectrum` on real LEDs, as `sweep` and as `morph`
  (`LIGHTD_SPECTRUM_EFFECT`), and say which it should be. That settles
  [ADR-0019](/docs/architecture/decisions/0019-an-album-can-have-a-look-of-its-own/).
- [ ] Leave the console open and idle for an hour, then change the track:
  the page should follow within a few seconds, with no reload.
- [ ] The console is told the strip's length. The stand could report it.

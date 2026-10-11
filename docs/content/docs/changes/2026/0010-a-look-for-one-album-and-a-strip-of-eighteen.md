---
title: "CHG-0010: A Look for One Album, and a Strip of Eighteen"
weight: 10
---

# CHG-0010: A Look for One Album, and a Strip of Eighteen

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
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

_To be filled in as it runs._

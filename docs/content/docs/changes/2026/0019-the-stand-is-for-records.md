---
title: "CHG-0019: The Stand Is for Records"
weight: 19
---

# CHG-0019: The Stand Is for Records

| | |
|---|---|
| **Date** | 2026-10-13 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "in-progress" "In progress" >}} |
| **Systems** | `nowplayd` and `lightd` on the workload |
| **How** | `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: a stand that is dark when someone expects it lit, and is taken for dead |
| **Decisions** | [ADR-0028](/docs/architecture/decisions/0028-the-stand-is-for-records/), which this is the evidence for |
| **Related** | [CHG-0016](/docs/changes/2026/0016-idle-the-screen-on-the-console-and-no-buttons-for-a-record/), which gave the stand its idle scene; [CHG-0015](/docs/changes/2026/0015-the-turntables-power-beacon/), whose beacon now decides between dim and dark |

---

## Summary

The stand was lit all day: dim whenever nothing played, and from iTunes'
cover whenever iTunes did. It is an LP stand, and from this change it is lit
by records only.

- iTunes' cover is published with the look `dark`, and the stand is sent
  dark for it. The cover screen and the console still show the cover.
- With nothing playing the stand is dark, and dim only while the turntable's
  beacon says the turntable is switched on.

## Goal

- iTunes playing, the turntable off: the stand dark, the sleeve on both
  screens.
- The turntable switched on: the stand dim within seconds of the beacon
  connecting.
- A record: lit from its sleeve, as before. Between its sides: dim.
- The turntable switched off: dark within about half a minute.

## Scope

**In scope:** `nowplayd` (one new setting, `NP_DARK_SOURCES=itunes`, and a
look published when the room goes idle) and `lightd` (two looks it now
draws).

**Out of scope:** every firmware, the agent Pi, the console, `palette`, the
tenant. No topic is added and no broker account changes: the looks travel
on `nowplaying/current/look`, which both services already have. The stand
already renders brightness 0 as off.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| A dark stand is taken for a dead one | the room | the console shows the stand's presence and the scene it was sent, "solid · brightness 0%" |
| The two services disagree for a moment while restarting | the workload | they restart together; an old `lightd` lights a `dark` cover from its colors and refuses the look of no cover, which is how it was before |
| The beacon is stuck `online` and the stand stays dim | the room | switch the turntable on and off (ADR-0023) |

## Procedure

1. `make -C deploy deploy WHERE=builder`.

## Verification

- `nowplayd` logs that it publishes `itunes` with the look that lights
  nothing.
- `eds/nowplaying/current/look` holds either a look that names no cover, or
  iTunes' cover with `"look":"dark"`.
- The stand's `scene` is solid at brightness 0 with the turntable off, and
  solid at 0.15 with it on and silent.

## Undo

- **iTunes on the stand again:** remove `NP_DARK_SOURCES` from
  `deploy/nowplayd.env`, and `make -C deploy units`.
- **Dim when idle again:** deploy the commit before this one.

## Outcome

| When (UTC) | What happened |
|---|---|

## Departures

## Follow-ups

- [ ] The five things ADR-0028 lists as not yet shown.

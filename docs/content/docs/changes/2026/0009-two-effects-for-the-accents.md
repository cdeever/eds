---
title: "CHG-0009: Two Effects for the Accents"
weight: 9
---

# CHG-0009: Two Effects for the Accents

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "active" "In progress" >}} |
| **Systems** | `lp-stand-01`, over USB; `eds-lightd` and `eds-console` on the `services` workload |
| **How** | `make -C firmware/lp-stand build flash`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: `lightd` asking for an effect the stand does not know, which the stand shows as a steady colour |
| **Decisions** | [ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/) |
| **Related** | [CHG-0008](/docs/changes/2026/0008-the-stand-reports-its-colours/) |

---

## Summary

The stand's default effect, `breathe`, lights a cover's dominant colour and
nothing else. The other colours `lightd` picks, the accents, reach the stand
in every scene and are used only by `sweep`.

After this change the stand knows two more effects, both built on the
accents: `morph`, which turns the whole strip from the dominant colour to
each accent and back, and `chase`, which sends the accents round over the
dominant colour. `lightd` asks for `chase` by default, and the console draws
both.

## Goal

- The stand's firmware knows `morph` and `chase`, and reports them by name in
  `scene.applied`.
- A new album produces a scene with `effect=chase`, applied by the stand.
- The console draws it: the dominant colour with the accents moving over it.
- `morph` can be asked for by hand and is applied and drawn.

## Scope

**In scope:** the stand's firmware; `LIGHTD_EFFECT` on the workload; the
console's drawing of the two effects.

**Out of scope:** how colours are picked, which is `lightd`'s and unchanged;
the scene contract's version, which adding an effect name does not move; the
LEDs, still not attached.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| `lightd` asks for an effect the stand does not know | the stand | the stand is flashed first. An unknown name is shown as `solid`, by design: the right colour, not moving |
| The console draws something the stand does not do | the page | the console's effects are a port of the firmware's, and a test holds them to pixels printed from the firmware's own code |
| An effect is unpleasant on real LEDs | the room | no LEDs are attached. `LIGHTD_EFFECT` is one line to change back |
| The timing fix changes the effects already there | the stand | the existing effect tests pass unchanged; the difference is below a millisecond until uptime is in hours |

## Procedure

```bash
make -C firmware/lp-stand test
make -C firmware/lp-stand build flash PORT=/dev/cu.usbserial-0001
make -C deploy deploy WHERE=builder
```

## Verification

- `make -C firmware/lp-stand logs` shows the stand restart and apply the
  retained scene.
- A new cover: `lightd` logs `effect=chase`, the stand reports
  `scene.applied effect=chase`.
- By hand: a scene with `"effect":"morph"` posted to `lightd` is applied and
  reported.
- The console's strip shows patches moving for `chase`, and one colour
  turning into another for `morph`.

## Undo

Remove `LIGHTD_EFFECT` from `deploy/lightd.env` and run `make -C deploy
units`: `breathe` again. The firmware can stay; it still knows the old three.

## Outcome

*To be filled in as it happens.*

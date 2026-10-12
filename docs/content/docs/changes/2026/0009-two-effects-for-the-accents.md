---
title: "CHG-0009: Two Effects for the Accents"
weight: 9
---

# CHG-0009: Two Effects for the Accents

| | |
|---|---|
| **Date** | 2026-10-11 |
| **Change type** | Upgrade |
| **Status** | {{< status-badge "complete" "Complete" >}} 2026-10-11 |
| **Systems** | `lp-stand-01`, over USB; `eds-lightd` and `eds-console` on the `services` workload |
| **How** | `make -C firmware/lp-stand build flash`; `make -C deploy deploy WHERE=builder` |
| **Risk** | Low. Most likely to go wrong: `lightd` asking for an effect the stand does not know, which the stand shows as a steady color |
| **Decisions** | [ADR-0001](/docs/architecture/decisions/0001-device-renders-effects/) |
| **Related** | [CHG-0008](/docs/changes/2026/0008-the-stand-reports-its-colours/) |

---

## Summary

The stand's default effect, `breathe`, lights a cover's dominant color and
nothing else. The other colors `lightd` picks, the accents, reach the stand
in every scene and are used only by `sweep`.

After this change the stand knows two more effects, both built on the
accents: `morph`, which turns the whole strip from the dominant color to
each accent and back, and `chase`, which sends the accents around over the
dominant color. `lightd` asks for `chase` by default, and the console draws
both.

## Goal

- The stand's firmware knows `morph` and `chase`, and reports them by name in
  `scene.applied`.
- A new album produces a scene with `effect=chase`, applied by the stand.
- The console draws it: the dominant color with the accents moving over it.
- `morph` can be asked for by hand and is applied and drawn.

## Scope

**In scope:** the stand's firmware; `LIGHTD_EFFECT` on the workload; the
console's drawing of the two effects.

**Out of scope:** how colors are picked, which is `lightd`'s and unchanged;
the scene contract's version, which adding an effect name does not move; the
LEDs, still not attached.

## Risk and impact

| Risk | Where | Guard |
|---|---|---|
| `lightd` asks for an effect the stand does not know | the stand | the stand is flashed first. An unknown name is shown as `solid`, by design: the right color, not moving |
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
- The console's strip shows patches moving for `chase`, and one color
  turning into another for `morph`.

## Undo

Remove `LIGHTD_EFFECT` from `deploy/lightd.env` and run `make -C deploy
units`: `breathe` again. The firmware can stay; it still knows the old three.

## Outcome

| When (UTC) | What happened |
|---|---|
| 19:16 | Host tests passed, 5421 checks. Built and flashed. The stand restarted as `c35de3a`, rejoined, and applied the retained scene, still `breathe`: `lightd` had not been changed yet. |
| 19:18 | Deployed. `lightd` lit the retained cover again with the new default: `effect=chase colors=4 palette="#fddb00 91%, #b87113 4%, #cda712 3%, #7e5f1e 2%"`. The stand applied it 0.2 s later and reported `Showing chase in 4 colours`. |
| 19:18 | The console's picture agreed: `chase`, the same four colors, the stand online and reporting `chase` as applied. |
| 19:18 | A scene with `"effect":"morph"` and the same palette, posted to `lightd` by hand, was applied and reported by the stand, and drawn by the console. `chase` was then posted back. |

All of the goal's checks hold. What neither effect has had is a look on real
LEDs, or a person's opinion of it on the console.

### Departures from the plan

- **A fault in the effects already there was found and fixed.** Animation
  time was taken as `(float)t_ms`. A float holds a millisecond count exactly
  for about four and a half hours; after that the remainder moves in steps,
  and after a week `breathe` and `sweep` would have advanced in jumps of tens
  of milliseconds. Time is now divided as a whole number first. The stand had
  never been up long enough, with LEDs, for anyone to see it.
- The cover lit at the time is 91% one yellow, so its accents are small and
  close to it in color. `chase` on it is a quiet thing. A cover with real
  contrast has not been shown yet.

## Follow-ups

- [ ] Look at both on real LEDs, and on the console with a few covers, and
  say which should be the default. `LIGHTD_EFFECT` is the one line.
- [ ] An accent very close to the dominant color barely shows in `chase`.
  `lightd` could prefer accents that differ from it; that is a color
  decision, and its to make.
- [ ] `morph` blends in RGB. Between two colors far apart in hue it passes
  through a duller one; blending in OKLab would not.

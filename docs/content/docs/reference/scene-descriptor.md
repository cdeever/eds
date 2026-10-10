---
title: "Scene Descriptor"
weight: 1
---

# Scene Descriptor

The payload `lightd` publishes and the LP stand renders. It is a description of
*what to render*, never a frame.

```json
{
  "v": 1,
  "effect": "breathe",
  "speed": 0.4,
  "brightness": 0.8,
  "palette": [
    { "rgb": [232, 77, 0], "weight": 0.5594 },
    { "rgb": [182, 159, 0], "weight": 0.3661 }
  ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `v` | int | Contract version. Currently `1`. |
| `effect` | string | `solid`, `breathe` or `sweep`. |
| `speed` | float | `0..1` dial, not a frequency. |
| `brightness` | float | `0..1`, applied before gamma on the device. |
| `palette` | array | Colours already corrected for the strip, ranked by weight. |
| `palette[].rgb` | `[int, int, int]` | sRGB, 0–255. Gamma is **not** applied. |
| `palette[].weight` | float | Share of the scene. Renormalised after filtering. |

## Version handling

A scene carrying a different `v` is **refused, not guessed at**. A scene that is
malformed or unparseable is **ignored**, and the previously rendering scene
keeps running — a stand that goes dark on a bad payload is worse than one that
ignores it.

An **unknown effect name falls back to `solid`** rather than being refused. A
publisher that learns a new effect before the stands do is expected; a lit
stand showing the right colours with the wrong animation beats a dark one.

## Effects

Deliberately few, because adding one means reflashing every stand.

`solid`
: The dominant colour, scaled by `brightness`.

`breathe`
: The dominant colour on a sine, with a floor above zero. A breathing stand
  that reaches full darkness reads as broken rather than slow.

`sweep`
: Lays the palette out in proportion to the weights, so a colour covering 60%
  of the sleeve covers 60% of the strip, and cross-fades at the seams. Hard
  edges on a diffused strip look like a fault in the diffuser.

## Speed

`speed` is a `0..1` dial rather than a frequency so the publisher never has to
know what a sensible rate looks like on this hardware. It maps linearly onto a
period between 6000 ms (slow) and 1000 ms (fast).

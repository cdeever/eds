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
| `effect` | string | `solid`, `breathe`, `sweep`, `morph`, `chase` or `scanner`. |
| `speed` | float | `0..1` dial, not a frequency. |
| `brightness` | float | `0..1`, applied before gamma on the device. |
| `palette` | array | Colors already corrected for the strip, ranked by weight. |
| `palette[].rgb` | `[int, int, int]` | sRGB, 0–255. Gamma is **not** applied. |
| `palette[].weight` | float | Share of the scene. Renormalized after filtering. |

## Version handling

A scene carrying a different `v` is **refused, not guessed at**. A scene that is
malformed or unparseable is **ignored**, and the previously rendering scene
keeps running — a stand that goes dark on a bad payload is worse than one that
ignores it.

An **unknown effect name falls back to `solid`** rather than being refused. A
publisher that learns a new effect before the stands do is expected; a lit
stand showing the right colors with the wrong animation beats a dark one.

## Effects

Deliberately few, because adding one means reflashing every stand.

`solid`
: The dominant color, scaled by `brightness`.

`breathe`
: The dominant color on a sine, with a floor above zero. A breathing stand
  that reaches full darkness reads as broken rather than slow.

`sweep`
: Lays the palette out in proportion to the weights, so a color covering 60%
  of the sleeve covers 60% of the strip, and cross-fades at the seams. Hard
  edges on a diffused strip look like a fault in the diffuser.

The last two use the **accents**: the colors of a cover that are not its
dominant one.

`morph`
: The whole strip is one color, turning from the dominant color to each
  accent in turn and back. A color's turn is half by its weight and half
  equal, so the dominant color is on longest and a 4% accent is still seen.
  The end of each turn is a glide into the next. One pass through every
  color takes two cycles for each color in the palette.

`chase`
: The dominant color fills the strip, and each accent is a soft-edged patch
  traveling around over it, wider for a larger weight. Alternate accents go
  opposite ways, so they pass through one another. One lap takes two cycles.

With a palette of one color, both show that color.

`scanner`
: One point of the dominant color going from one end of the strip to the
  other and back, with what it has just passed fading behind it, and the
  rest of the strip dark. There and back takes one cycle. Only the first
  color of the palette is used
  ([ADR-0030](/docs/architecture/decisions/0030-a-piece-of-music-can-have-a-look-from-any-source/)).

## Speed

`speed` is a `0..1` dial rather than a frequency so the publisher never has to
know what a sensible rate looks like on this hardware. It maps linearly onto a
period between 6000 ms (slow) and 1000 ms (fast).

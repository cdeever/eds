---
title: "Layer Boundaries"
weight: 2
---

# Layer Boundaries

The most common way to break EdS is to put a decision in the wrong layer. Each
layer below is ignorant of the next on purpose.

## palette decides nothing about hardware

`palette` takes image bytes and returns perceptual colour data. It knows
nothing about music, MQTT or LEDs.

It deliberately does **not** do role assignment (primary/secondary/accent) and
**not** LED correction. A palette that looks right on a monitor reads washed
out on a strip, and deciding which swatch drives which part of a strip requires
knowing what hardware is on the other end. Both belong to the consumer.

That boundary is what makes the extractor developable and tunable against a
folder of JPEGs with no infrastructure at all.

## lightd owns the hardware-facing colour decisions

`lightd` knows there is a strip on the other end, so the decisions that need
that knowledge live here:

- **Rejecting neutrals.** A swatch below the minimum chroma will not read as a
  colour on a strip regardless of how much of the sleeve it covers. Weight
  alone is not enough: asking for few swatches from busy art forces clusters to
  merge, and a merged centroid lands near the average of what it merged — a
  high-weight grey. Filtering never returns nothing, though; genuinely
  monochrome art keeps its most colourful swatch, because the stand still has
  to light up.
- **Saturation.** Chroma is scaled in OKLab, so a boosted colour stays
  recognisably the same colour.
- **Gamut mapping.** Boosting chroma pushes colours outside sRGB. Rather than
  clipping channels — which shifts hue, so a boosted orange arrives as a
  different colour than the one on the sleeve — chroma is walked back down
  until the colour is representable, holding hue and lightness fixed.

## Firmware owns gamma

Gamma is **not** applied upstream. WS2812 PWM is linear in duty cycle while
perception is not, so an uncorrected fade rushes through the dark end and then
stalls — but the right curve is a property of *this* driver and diffuser.

Correcting at the output stage keeps gamma next to the hardware it corrects,
and keeps the wire format showing the colours actually intended. That matters
the first time something looks wrong: `mosquitto_sub` shows what was meant, not
what a particular strip needed.

## A practical consequence

OKLab conversion is implemented independently in all three places —
`palette/oklab.py`, `lightd/internal/oklab`, and the firmware's effects. If you
change conversion behaviour in one, check the other two.

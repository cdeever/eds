---
title: "Layer Boundaries"
weight: 2
---

# Layer Boundaries

Each component of EdS is built to do one well-defined job, and to do it
without knowing how its neighbors do theirs. It takes a plain input, returns
a plain output, and keeps everything in between to itself: much as
abstraction and encapsulation work in object-oriented programming, applied to
services and devices in place of classes.

That is what lets each part be built, tested and tuned alone, and replaced
without the others noticing. `palette` can be tuned against a folder of
JPEGs with nothing else running. A different diffuser wants a different
gamma curve, and that changes the firmware and nothing upstream of it.

The sections below say what each layer owns, and what it leaves to the next.

## palette decides nothing about hardware

`palette` takes image bytes and returns perceptual color data. It knows
nothing about music, MQTT or LEDs.

It deliberately does **not** do role assignment (primary/secondary/accent) and
**not** LED correction. A palette that looks right on a monitor reads washed
out on a strip, and deciding which swatch drives which part of a strip requires
knowing what hardware is on the other end. Both belong to the consumer.

That boundary is what makes the extractor developable and tunable against a
folder of JPEGs with no infrastructure at all.

## lightd owns the hardware-facing color decisions

`lightd` knows there is a strip on the other end, so the decisions that need
that knowledge live here:

- **Rejecting neutrals.** A swatch below the minimum chroma will not read as a
  color on a strip regardless of how much of the sleeve it covers. Weight
  alone is not enough: asking for few swatches from busy art forces clusters to
  merge, and a merged centroid lands near the average of what it merged — a
  high-weight gray. Filtering never returns nothing, though; genuinely
  monochrome art keeps its most colorful swatch, because the stand still has
  to light up.
- **Saturation.** Chroma is scaled in OKLab, so a boosted color stays
  recognizably the same color.
- **Gamut mapping.** Boosting chroma pushes colors outside sRGB. Rather than
  clipping channels — which shifts hue, so a boosted orange arrives as a
  different color than the one on the sleeve — chroma is walked back down
  until the color is representable, holding hue and lightness fixed.

## Firmware owns gamma

Gamma is **not** applied upstream. WS2812 PWM is linear in duty cycle while
perception is not, so an uncorrected fade rushes through the dark end and then
stalls — but the right curve is a property of *this* driver and diffuser.

Correcting at the output stage keeps gamma next to the hardware it corrects,
and keeps the wire format showing the colors actually intended. That matters
the first time something looks wrong: `mosquitto_sub` shows what was meant, not
what a particular strip needed.

## The color space they agree on

The one thing two layers have in common is a definition, not code: OKLab.

A picture stores color as red, green and blue, which describes what a
screen emits and not what a person sees. Arithmetic on those numbers goes
wrong in ways the eye notices at once. The average of a strong red and a
strong blue is a gray-purple that is nowhere on the sleeve. Push a color
to be more vivid and a channel runs out of room, the hue shifts, and a
boosted orange arrives as some other color.

[OKLab](https://bottosson.github.io/posts/oklab/) (Björn Ottosson, 2020)
describes a color the way it is perceived: one number for lightness and two
that place it between green and red and between blue and yellow. From those
come a color's hue and its chroma, how far it is from gray. Equal steps look
like equal changes, and the three can be moved independently, so the
questions EdS asks have honest answers:

| Question | Asked by | In OKLab |
|---|---|---|
| Which colors is this sleeve made of? | `palette` | the center of a cluster is a color a person would agree is in the image |
| Is this swatch a color or a gray? | `lightd` | its chroma says so directly |
| Make it more vivid for a strip | `lightd` | raise chroma; hue and lightness stay put |
| It is now too vivid to display | `lightd` | walk chroma back down; it is still the same color |

`palette` and `lightd` each carry their own copy of the conversion
(`palette/oklab.py` in Python, `lightd/internal/oklab` in Go), so that neither
needs the other to build or run. A test in `lightd` holds its copy to values
printed from `palette`'s. The math is a published standard and should not
need to change; if it is ever corrected in one, correct the other and the
test's reference values with it.

The firmware does none of this. It receives colors already chosen, as red,
green and blue, and mixes between them as they are.

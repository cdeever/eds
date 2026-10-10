---
title: "Palette API"
weight: 2
---

# Palette API

`POST /v1/palette` — image bytes in, ranked colour swatches out.

Accepts either a multipart upload or a raw body. Both return the same result:
multipart is what a person uses, the raw body is what `lightd` uses.

```bash
curl -F image=@jacket.jpg 'http://localhost:8731/v1/palette?n=5'

curl --data-binary @jacket.jpg -H 'Content-Type: image/png' \
     'http://localhost:8731/v1/palette'
```

`GET /openapi.json` publishes the schema, which is how the contract stays
honest across the Python/Go boundary.

## Parameters

| Name | Type | Default | |
|---|---|---|---|
| `n` | int, 1–16 | 6 | Swatches to return. |

## Response

```json
{
  "v": 1,
  "swatches": [
    { "rgb": [225, 82, 26], "hex": "#e1521a",
      "oklab": [0.6264, 0.147, 0.1184],
      "weight": 0.3705, "chroma": 0.1888, "lightness": 0.6264 }
  ],
  "background_dropped": 0.8855,
  "fallback": false,
  "source": { "width": 600, "height": 600, "format": "jpeg" }
}
```

`weight`
: Share of clustered pixels, so swatches are ranked by how much of the sleeve
  they actually cover.

`background_dropped`
: Share of pixels discarded before clustering.

`fallback`
: True when filtering removed so much that the unfiltered image was used
  instead. Genuinely monochrome art sets this rather than returning nothing.

`chroma`
: How far the colour is from neutral. **Consumers should treat a low-chroma
  swatch as unfit to drive a strip regardless of its weight** — see below.

## Known characteristics

**Low `n` on busy art returns low-chroma centroids.** Asking for 2 swatches
from a four-colour sleeve forces one cluster to merge three of them, and the
merged centroid is close to their average. This is arithmetic, not a bug — but
it is why `chroma` is in the contract. The default `n=6` avoids the situation
for most art.

## The algorithm

Five steps, each answering a specific pathology of album sleeves rather than
being generic image processing:

1. **Decode and downscale** to 160px on the long edge.
2. **Convert to OKLab.** RGB averaging crosses hue: the mean of a strong red and
   a strong blue is a grey-purple that appears nowhere on the sleeve.
3. **Drop background pixels** — near-black, near-white, near-neutral — and
   record the fraction. A large share of sleeves are mostly flat field; cluster
   those in and the dominant colour is "dark", so the strip just goes dim.
4. **Cluster** what remains (k-means with k-means++ seeding, fixed seed).
5. **Rank by population share** and convert back to sRGB.

Transparency is composited onto **white**, not black — compositing onto black
would manufacture exactly the dark background step 3 then removes.

The fixed seed matters: a stable input must not repaint the room.

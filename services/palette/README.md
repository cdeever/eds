# palette

Album cover art in, ranked colour swatches out.

This service knows nothing about music, MQTT or LEDs. It takes image bytes and
returns perceptual colour data. That boundary is deliberate: it makes the
extractor developable and tunable against a folder of JPEGs with no
infrastructure at all, and it is the interface the future album cover resolver
will reach through `lightd`.

## Running it

```bash
python3 -m venv .venv
.venv/bin/pip install -e ".[dev]"

.venv/bin/pytest                                    # 37 tests, no network
.venv/bin/uvicorn eds_palette.api:app --port 8731   # serve
```

```bash
curl -F image=@jacket.jpg 'http://localhost:8731/v1/palette?n=5'   # multipart
curl --data-binary @jacket.jpg -H 'Content-Type: image/png' \
     'http://localhost:8731/v1/palette'                            # raw body
```

Both shapes return the same result. Multipart is what a person uses; the raw
body is what `lightd` uses. `GET /openapi.json` publishes the schema, which is
how the contract stays honest across the Python/Go boundary.

## Tuning by eye

The thresholds are judgement calls that no test can settle — where a colour
stops being a colour, how much black field to discard. The CLI exists for that
loop:

```bash
.venv/bin/eds-palette ~/covers -n 5 --contact-sheet /tmp/sheet.png
.venv/bin/eds-palette ~/covers --min-chroma 0.05 --min-lightness 0.08
```

It prints truecolor swatches in the terminal and can render a PNG sheet of
thumbnails beside their weighted palettes, so many sleeves can be compared at
once.

## The contract

`POST /v1/palette` — image bytes, optional `?n=` (1–16, default 6).

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

`weight` is the share of clustered pixels, so swatches are ranked by how much
of the sleeve they actually cover. `background_dropped` is the share of pixels
discarded before clustering. `fallback` is true when filtering removed so much
that the unfiltered image was used instead.

## The algorithm, and why each step is there

Five steps, each answering a specific pathology of album sleeves rather than
being generic image processing:

1. **Decode and downscale** to 160px on the long edge. Full resolution buys
   nothing for finding six colours and costs milliseconds.
2. **Convert to OKLab.** RGB averaging crosses hue: the mean of a strong red
   and a strong blue is a grey-purple that appears nowhere on the sleeve. In
   OKLab a centroid is a colour a person would agree was in the image.
3. **Drop background pixels** — near-black, near-white, near-neutral — and
   record the fraction. A large share of sleeves are mostly flat field; cluster
   those in and the dominant colour is "dark", so the strip just goes dim.
4. **Cluster** what remains (k-means with k-means++ seeding, fixed seed).
5. **Rank by population share** and convert back to sRGB.

Transparency is composited onto **white**, not black — compositing onto black
would manufacture exactly the dark background step 3 then removes, quietly
discarding real colour on transparent art.

k-means is hand-rolled in numpy rather than pulled from scikit-learn: it is
thirty lines, it keeps the dependency set small enough to matter on a small VM,
and a fixed seed makes output reproducible. A stable input must not repaint the
room.

## What this deliberately does not do

No role assignment (primary/secondary/accent), and no LED correction. A palette
that looks right on a monitor reads washed out on a strip, and deciding which
swatch drives which part of the strip requires knowing what hardware is on the
other end. Both belong in `lightd`.

## Known characteristics

**Low `n` on busy art returns low-chroma centroids.** Asking for 2 swatches
from a four-colour sleeve forces one cluster to merge three of them, and the
merged centroid is close to their average. This is arithmetic, not a bug — but
it is why `chroma` is in the contract. `lightd` should treat a low-chroma
swatch as unfit to drive a strip regardless of its weight. The default `n=6`
avoids the situation for most art.

**Genuinely monochrome art sets `fallback`.** A sepia or greyscale sleeve has
no colour to find. The filter stands down, clusters the unfiltered pixels and
says so, rather than returning nothing.

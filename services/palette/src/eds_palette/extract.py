"""Palette extraction from cover art.

The pipeline is five steps, each answering a specific pathology of album
sleeves rather than being generic image processing:

1. Decode and downscale. Full resolution buys nothing for clustering.
2. Convert to OKLab, because RGB clustering returns mud (see oklab module).
3. Drop background pixels. A large share of sleeves are mostly black or white
   field; cluster those in and the dominant colour is "dark", so the strip
   just goes dim.
4. Cluster what is left.
5. Rank by population share and convert back to sRGB.

The result is deliberately raw: ranked swatches with weights and no role
assignment. Deciding which swatch drives which part of an LED strip, and
correcting for the fact that a palette which looks right on a monitor reads
washed out on a strip, belongs to the consumer that knows the hardware.
"""

from __future__ import annotations

import io
from dataclasses import dataclass, field

import numpy as np
from PIL import Image, UnidentifiedImageError

from .oklab import chroma, oklab_to_srgb, srgb_to_oklab

# Clustering resolution. 160px on the long edge is ~25k pixels, which is far
# more than enough to find six colours and keeps a request in the low
# milliseconds.
DEFAULT_MAX_DIM = 160
DEFAULT_SWATCHES = 6

# Background thresholds in OKLab. L runs roughly 0..1; chroma of 0.035 is about
# where a colour stops reading as a colour and starts reading as a grey.
DEFAULT_MIN_LIGHTNESS = 0.12
DEFAULT_MAX_LIGHTNESS = 0.95
DEFAULT_MIN_CHROMA = 0.035

# If filtering leaves less than this share of the image, the sleeve is
# genuinely monochrome (or near enough) and the filter is telling us nothing.
# Fall back to the unfiltered pixels rather than clustering noise.
MIN_KEEP_FRACTION = 0.02

_KMEANS_SEED = 20260907
_KMEANS_ITERATIONS = 40
_KMEANS_TOLERANCE = 1e-6


class InvalidImageError(ValueError):
    """Raised when the supplied bytes are not a decodable image."""


@dataclass(frozen=True)
class Swatch:
    rgb: tuple[int, int, int]
    hex: str
    oklab: tuple[float, float, float]
    weight: float
    chroma: float
    lightness: float


@dataclass(frozen=True)
class SourceInfo:
    width: int
    height: int
    format: str


@dataclass(frozen=True)
class PaletteResult:
    swatches: list[Swatch] = field(default_factory=list)
    background_dropped: float = 0.0
    fallback: bool = False
    source: SourceInfo | None = None


def _load_pixels(data: bytes, max_dim: int) -> tuple[np.ndarray, SourceInfo]:
    """Decode to a downscaled (N, 3) array of 0..1 sRGB floats."""
    try:
        image = Image.open(io.BytesIO(data))
        image.load()
    except (UnidentifiedImageError, OSError) as exc:
        raise InvalidImageError("could not decode image") from exc

    source = SourceInfo(
        width=image.width,
        height=image.height,
        format=(image.format or "unknown").lower(),
    )

    # Flatten alpha onto white before dropping the channel: compositing onto
    # black would manufacture exactly the dark background the filter then
    # removes, which would silently discard real colour on transparent art.
    if image.mode in ("RGBA", "LA", "PA") or "transparency" in image.info:
        rgba = image.convert("RGBA")
        backdrop = Image.new("RGBA", rgba.size, (255, 255, 255, 255))
        image = Image.alpha_composite(backdrop, rgba)

    image = image.convert("RGB")
    image.thumbnail((max_dim, max_dim), Image.Resampling.BILINEAR)

    pixels = np.asarray(image, dtype=np.float64).reshape(-1, 3) / 255.0
    return pixels, source


def _kmeans(points: np.ndarray, k: int, *, seed: int) -> tuple[np.ndarray, np.ndarray]:
    """Minimal k-means with k-means++ seeding.

    Hand-rolled rather than pulled from scikit-learn: it is thirty lines, it
    keeps the service's dependency set small enough to matter on a small VM,
    and a fixed seed makes the output reproducible, which the tests rely on.

    Returns (centres, counts) with empty clusters already removed.
    """
    rng = np.random.default_rng(seed)
    n = len(points)
    k = min(k, n)

    centres = np.empty((k, points.shape[1]), dtype=np.float64)
    centres[0] = points[rng.integers(n)]
    nearest_sq = np.sum((points - centres[0]) ** 2, axis=1)

    for i in range(1, k):
        total = nearest_sq.sum()
        if total <= 0:
            # Every remaining point is already a centre; pad arbitrarily.
            centres[i] = points[rng.integers(n)]
        else:
            centres[i] = points[rng.choice(n, p=nearest_sq / total)]
        nearest_sq = np.minimum(nearest_sq, np.sum((points - centres[i]) ** 2, axis=1))

    labels = np.zeros(n, dtype=np.int64)
    for _ in range(_KMEANS_ITERATIONS):
        distances = ((points[:, None, :] - centres[None, :, :]) ** 2).sum(axis=2)
        labels = distances.argmin(axis=1)

        shift = 0.0
        for i in range(k):
            member = points[labels == i]
            if len(member) == 0:
                # Reseed an empty cluster onto the worst-served point rather
                # than dropping k silently.
                worst = distances[np.arange(n), labels].argmax()
                new_centre = points[worst]
            else:
                new_centre = member.mean(axis=0)
            shift = max(shift, float(np.sum((new_centre - centres[i]) ** 2)))
            centres[i] = new_centre

        if shift < _KMEANS_TOLERANCE:
            break

    counts = np.bincount(labels, minlength=k)
    keep = counts > 0
    return centres[keep], counts[keep]


def extract_palette(
    data: bytes,
    *,
    swatches: int = DEFAULT_SWATCHES,
    max_dim: int = DEFAULT_MAX_DIM,
    min_lightness: float = DEFAULT_MIN_LIGHTNESS,
    max_lightness: float = DEFAULT_MAX_LIGHTNESS,
    min_chroma: float = DEFAULT_MIN_CHROMA,
    seed: int = _KMEANS_SEED,
) -> PaletteResult:
    """Extract a ranked, weighted palette from encoded image bytes."""
    if swatches < 1:
        raise ValueError("swatches must be at least 1")

    pixels, source = _load_pixels(data, max_dim)
    if len(pixels) == 0:
        raise InvalidImageError("image contains no pixels")

    lab = srgb_to_oklab(pixels)
    lightness = lab[:, 0]
    pixel_chroma = chroma(lab)

    keep = (
        (lightness >= min_lightness)
        & (lightness <= max_lightness)
        & (pixel_chroma >= min_chroma)
    )
    dropped = float(1.0 - keep.mean())

    fallback = bool(keep.sum() < max(swatches, len(pixels) * MIN_KEEP_FRACTION))
    subject = lab if fallback else lab[keep]

    centres, counts = _kmeans(subject, swatches, seed=seed)

    order = np.argsort(counts)[::-1]
    centres, counts = centres[order], counts[order]
    weights = counts / counts.sum()

    rgb = np.rint(oklab_to_srgb(centres) * 255.0).astype(int)
    centre_chroma = chroma(centres)

    result = [
        Swatch(
            rgb=(int(r), int(g), int(b)),
            hex=f"#{int(r):02x}{int(g):02x}{int(b):02x}",
            oklab=(round(float(c[0]), 4), round(float(c[1]), 4), round(float(c[2]), 4)),
            weight=round(float(w), 4),
            chroma=round(float(ch), 4),
            lightness=round(float(c[0]), 4),
        )
        for (r, g, b), c, w, ch in zip(rgb, centres, weights, centre_chroma)
    ]

    return PaletteResult(
        swatches=result,
        background_dropped=round(dropped, 4),
        fallback=fallback,
        source=source,
    )

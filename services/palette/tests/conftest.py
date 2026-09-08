"""Synthetic sleeves.

Real cover art cannot be committed, so the corpus here is built to exhibit the
specific pathologies the algorithm exists to handle: two-tone sleeves, heavy
black field, blown-out white field, and genuinely monochrome art.
"""

from __future__ import annotations

import io

import pytest
from PIL import Image

RED = (220, 40, 30)
BLUE = (30, 60, 200)
GREEN = (40, 190, 70)
BLACK = (0, 0, 0)
WHITE = (255, 255, 255)


def encode(image: Image.Image, fmt: str = "PNG") -> bytes:
    buffer = io.BytesIO()
    image.save(buffer, format=fmt)
    return buffer.getvalue()


def solid(color, size=(200, 200), mode="RGB") -> Image.Image:
    return Image.new(mode, size, color)


def halves(left, right, size=(200, 200)) -> Image.Image:
    """Two equal vertical bands - the simplest weighted palette to reason about."""
    image = Image.new("RGB", size, left)
    image.paste(Image.new("RGB", (size[0] // 2, size[1]), right), (size[0] // 2, 0))
    return image


def patch_on_field(patch, field, coverage=0.1, size=(200, 200)) -> Image.Image:
    """A small coloured patch on a large flat field, like a minimalist sleeve."""
    image = Image.new("RGB", size, field)
    side = max(1, int((size[0] * size[1] * coverage) ** 0.5))
    image.paste(Image.new("RGB", (side, side), patch), (10, 10))
    return image


@pytest.fixture
def two_tone() -> bytes:
    return encode(halves(RED, BLUE))


@pytest.fixture
def black_field() -> bytes:
    return encode(patch_on_field(RED, BLACK, coverage=0.1))


@pytest.fixture
def white_field() -> bytes:
    return encode(patch_on_field(BLUE, WHITE, coverage=0.1))


@pytest.fixture
def all_black() -> bytes:
    return encode(solid(BLACK))


@pytest.fixture
def transparent_red() -> bytes:
    """Red square on a fully transparent background."""
    image = Image.new("RGBA", (200, 200), (0, 0, 0, 0))
    image.paste(Image.new("RGBA", (60, 60), (*RED, 255)), (70, 70))
    return encode(image)


@pytest.fixture
def three_tone() -> bytes:
    image = Image.new("RGB", (300, 200), RED)
    image.paste(Image.new("RGB", (100, 200), BLUE), (100, 0))
    image.paste(Image.new("RGB", (100, 200), GREEN), (200, 0))
    return encode(image)

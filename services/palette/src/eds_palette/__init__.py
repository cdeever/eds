"""Palette extraction for EdS: album cover art in, ranked colour swatches out."""

__version__ = "0.1.0"

from .extract import (
    InvalidImageError,
    PaletteResult,
    SourceInfo,
    Swatch,
    extract_palette,
)

__all__ = [
    "InvalidImageError",
    "PaletteResult",
    "SourceInfo",
    "Swatch",
    "extract_palette",
    "__version__",
]

"""Command-line wrapper for tuning the extractor by eye.

The algorithm has thresholds that no test can really settle - where a colour
stops being a colour, how much black field to discard - so the fast loop
matters: run it over a folder of sleeves, look at the result, move a number.
"""

from __future__ import annotations

import argparse
import json
import sys
from dataclasses import asdict
from pathlib import Path

from .extract import (
    DEFAULT_MAX_DIM,
    DEFAULT_MIN_CHROMA,
    DEFAULT_MIN_LIGHTNESS,
    DEFAULT_SWATCHES,
    InvalidImageError,
    PaletteResult,
    extract_palette,
)

IMAGE_SUFFIXES = {".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff"}

_SHEET_THUMB = 120
_SHEET_BAR_WIDTH = 420


def _gather(paths: list[Path]) -> list[Path]:
    found: list[Path] = []
    for path in paths:
        if path.is_dir():
            found.extend(
                sorted(p for p in path.rglob("*") if p.suffix.lower() in IMAGE_SUFFIXES)
            )
        else:
            found.append(path)
    return found


def _swatch_block(rgb: tuple[int, int, int], width: int = 4) -> str:
    """A truecolor terminal block, so the palette is visible where it is run."""
    r, g, b = rgb
    return f"\x1b[48;2;{r};{g};{b}m{' ' * width}\x1b[0m"


def _print_report(path: Path, result: PaletteResult, *, color: bool) -> None:
    src = result.source
    flag = "  [fallback: filter removed too much]" if result.fallback else ""
    print(f"\n{path}")
    print(
        f"  {src.width}x{src.height} {src.format}"
        f"   background dropped: {result.background_dropped:.0%}{flag}"
    )
    for i, s in enumerate(result.swatches, 1):
        block = _swatch_block(s.rgb) + " " if color else ""
        print(
            f"  {i}. {block}{s.hex}  weight {s.weight:>6.1%}"
            f"  L {s.lightness:.3f}  C {s.chroma:.3f}"
        )


def _contact_sheet(rows: list[tuple[Path, PaletteResult]], out: Path) -> None:
    """Render thumbnails beside their palettes, for comparing many at once."""
    from PIL import Image

    width = _SHEET_THUMB + _SHEET_BAR_WIDTH
    sheet = Image.new("RGB", (width, _SHEET_THUMB * len(rows)), (18, 18, 18))

    for index, (path, result) in enumerate(rows):
        top = index * _SHEET_THUMB
        with Image.open(path) as source:
            thumb = source.convert("RGB")
            thumb.thumbnail((_SHEET_THUMB, _SHEET_THUMB), Image.Resampling.BILINEAR)
            sheet.paste(thumb, (0, top))

        # Bars are proportional to weight, so the ranking is visible as area.
        x = _SHEET_THUMB
        for swatch in result.swatches:
            bar = max(1, round(swatch.weight * _SHEET_BAR_WIDTH))
            bar = min(bar, width - x)
            if bar <= 0:
                break
            sheet.paste(Image.new("RGB", (bar, _SHEET_THUMB), swatch.rgb), (x, top))
            x += bar

    sheet.save(out)
    print(f"\ncontact sheet: {out}", file=sys.stderr)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="eds-palette",
        description="Extract colour palettes from cover art.",
    )
    parser.add_argument("paths", nargs="+", type=Path, help="Image files or directories.")
    parser.add_argument("-n", "--swatches", type=int, default=DEFAULT_SWATCHES)
    parser.add_argument("--max-dim", type=int, default=DEFAULT_MAX_DIM)
    parser.add_argument("--min-lightness", type=float, default=DEFAULT_MIN_LIGHTNESS)
    parser.add_argument("--min-chroma", type=float, default=DEFAULT_MIN_CHROMA)
    parser.add_argument("--json", action="store_true", help="Emit JSON instead of a report.")
    parser.add_argument("--contact-sheet", type=Path, help="Write a PNG comparison sheet.")
    parser.add_argument("--no-color", action="store_true", help="Suppress terminal swatches.")
    args = parser.parse_args(argv)

    files = _gather(args.paths)
    if not files:
        print("no images found", file=sys.stderr)
        return 1

    color = not args.no_color and sys.stdout.isatty()
    rows: list[tuple[Path, PaletteResult]] = []
    payload: list[dict] = []
    failures = 0

    for path in files:
        try:
            result = extract_palette(
                path.read_bytes(),
                swatches=args.swatches,
                max_dim=args.max_dim,
                min_lightness=args.min_lightness,
                min_chroma=args.min_chroma,
            )
        except (InvalidImageError, OSError) as exc:
            print(f"{path}: {exc}", file=sys.stderr)
            failures += 1
            continue

        rows.append((path, result))
        if args.json:
            payload.append({"path": str(path), **asdict(result)})
        else:
            _print_report(path, result, color=color)

    if args.json:
        print(json.dumps(payload, indent=2))

    if args.contact_sheet and rows:
        _contact_sheet(rows, args.contact_sheet)

    return 1 if failures and not rows else 0


if __name__ == "__main__":
    raise SystemExit(main())

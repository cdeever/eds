from __future__ import annotations

import pytest

from eds_palette.extract import InvalidImageError, extract_palette

from conftest import RED, encode, solid


def is_reddish(rgb) -> bool:
    r, g, b = rgb
    return r > 120 and r > g + 50 and r > b + 50


def is_bluish(rgb) -> bool:
    r, g, b = rgb
    return b > 120 and b > r + 50 and b > g + 50


def test_two_tone_recovers_both_hues_at_equal_weight(two_tone):
    result = extract_palette(two_tone, swatches=2)

    assert len(result.swatches) == 2
    assert any(is_reddish(s.rgb) for s in result.swatches)
    assert any(is_bluish(s.rgb) for s in result.swatches)
    for swatch in result.swatches:
        assert swatch.weight == pytest.approx(0.5, abs=0.05)


def test_two_tone_never_returns_the_average(two_tone):
    """The whole reason for clustering in OKLab: a red-and-blue sleeve must not
    yield the grey-purple that sits between them and appears nowhere on it."""
    result = extract_palette(two_tone, swatches=2)
    for swatch in result.swatches:
        r, g, b = swatch.rgb
        assert not (abs(r - b) < 60 and g < 100), f"muddy centroid {swatch.hex}"


def test_black_field_is_dropped_and_the_patch_wins(black_field):
    """A sleeve that is 90% black field must light the strip with its 10% of
    actual colour, not report 'dark' and go dim."""
    result = extract_palette(black_field, swatches=3)

    assert result.background_dropped > 0.8
    assert result.fallback is False
    assert is_reddish(result.swatches[0].rgb)


def test_white_field_is_dropped_too(white_field):
    result = extract_palette(white_field, swatches=3)

    assert result.background_dropped > 0.8
    assert result.fallback is False
    assert is_bluish(result.swatches[0].rgb)


def test_monochrome_art_falls_back_instead_of_failing(all_black):
    """Filtering removes everything here. Clustering nothing is worse than
    clustering the unfiltered image, so the filter stands down and says so."""
    result = extract_palette(all_black, swatches=4)

    assert result.fallback is True
    assert result.background_dropped == pytest.approx(1.0, abs=1e-6)
    assert result.swatches
    assert all(sum(s.rgb) < 30 for s in result.swatches)


def test_transparency_composites_onto_white_not_black(transparent_red):
    """Compositing onto black would manufacture the dark background the filter
    then removes, quietly discarding the real colour."""
    result = extract_palette(transparent_red, swatches=3)
    assert any(is_reddish(s.rgb) for s in result.swatches)


def test_weights_are_normalised_and_ranked(three_tone):
    result = extract_palette(three_tone, swatches=3)

    weights = [s.weight for s in result.swatches]
    assert sum(weights) == pytest.approx(1.0, abs=1e-3)
    assert weights == sorted(weights, reverse=True)


def test_three_tone_finds_three_distinct_hues(three_tone):
    result = extract_palette(three_tone, swatches=3)
    hexes = {s.hex for s in result.swatches}
    assert len(hexes) == 3


def test_is_deterministic(three_tone):
    """lightd will publish these; a stable input must not repaint the room."""
    first = extract_palette(three_tone, swatches=5)
    second = extract_palette(three_tone, swatches=5)
    assert [s.hex for s in first.swatches] == [s.hex for s in second.swatches]
    assert [s.weight for s in first.swatches] == [s.weight for s in second.swatches]


def test_swatch_count_is_capped_by_available_colour(two_tone):
    """Asking for more clusters than the image contains must not invent any."""
    result = extract_palette(two_tone, swatches=12)
    assert 2 <= len(result.swatches) <= 12
    assert sum(s.weight for s in result.swatches) == pytest.approx(1.0, abs=1e-3)


def test_source_metadata_reports_the_original_size(two_tone):
    result = extract_palette(two_tone, swatches=2)
    assert result.source.width == 200
    assert result.source.height == 200
    assert result.source.format == "png"


def test_jpeg_is_accepted():
    data = encode(solid(RED), fmt="JPEG")
    result = extract_palette(data, swatches=2)
    assert result.source.format == "jpeg"
    assert is_reddish(result.swatches[0].rgb)


def test_hex_matches_rgb(three_tone):
    for swatch in extract_palette(three_tone, swatches=3).swatches:
        assert swatch.hex == "#%02x%02x%02x" % swatch.rgb


def test_rejects_non_image_bytes():
    with pytest.raises(InvalidImageError):
        extract_palette(b"this is not an image")


def test_rejects_empty_input():
    with pytest.raises(InvalidImageError):
        extract_palette(b"")


def test_rejects_zero_swatches(two_tone):
    with pytest.raises(ValueError):
        extract_palette(two_tone, swatches=0)


def test_single_swatch_is_allowed(two_tone):
    result = extract_palette(two_tone, swatches=1)
    assert len(result.swatches) == 1
    assert result.swatches[0].weight == pytest.approx(1.0, abs=1e-6)


def test_lightness_field_matches_oklab_l(three_tone):
    for swatch in extract_palette(three_tone, swatches=3).swatches:
        assert swatch.lightness == pytest.approx(swatch.oklab[0], abs=1e-4)

from __future__ import annotations

import numpy as np
import pytest

from eds_palette.oklab import chroma, oklab_to_srgb, srgb_to_oklab


def test_round_trips_within_tolerance():
    """Ottosson's forward and inverse matrices are not exact inverses of one
    another, so a round trip drifts by ~1.6e-6. That is roughly 2500x smaller
    than one 8-bit step (1/255), so it cannot change a rendered swatch."""
    rng = np.random.default_rng(0)
    rgb = rng.random((4096, 3))
    round_tripped = oklab_to_srgb(srgb_to_oklab(rgb))
    assert np.max(np.abs(round_tripped - rgb)) < 1e-5
    assert np.max(np.abs(round_tripped - rgb)) < (1 / 255) / 100


@pytest.mark.parametrize(
    ("rgb", "expected_l"),
    [((1.0, 1.0, 1.0), 1.0), ((0.0, 0.0, 0.0), 0.0)],
)
def test_lightness_anchors(rgb, expected_l):
    lab = srgb_to_oklab(np.array([rgb]))
    assert lab[0, 0] == pytest.approx(expected_l, abs=1e-4)


def test_neutrals_have_no_chroma():
    greys = np.linspace(0, 1, 16)[:, None].repeat(3, axis=1)
    assert np.all(chroma(srgb_to_oklab(greys)) < 1e-6)


def test_saturated_colours_have_chroma():
    lab = srgb_to_oklab(np.array([[1.0, 0.0, 0.0], [0.0, 0.0, 1.0]]))
    assert np.all(chroma(lab) > 0.1)


def test_red_and_blue_are_far_apart_in_oklab():
    """The premise of clustering in OKLab: opposing hues must not collapse."""
    lab = srgb_to_oklab(np.array([[0.86, 0.16, 0.12], [0.12, 0.24, 0.78]]))
    assert np.linalg.norm(lab[0] - lab[1]) > 0.2

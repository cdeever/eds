"""sRGB <-> OKLab conversion, vectorised over numpy arrays.

Clustering happens in OKLab rather than RGB because RGB averaging crosses hue.
The mean of a strong red and a strong blue is a grey-purple that appears
nowhere on the sleeve; in OKLab a cluster centroid is a colour a person would
agree was actually in the image.

Coefficients are Bjorn Ottosson's, from the original OKLab derivation.
"""

from __future__ import annotations

import numpy as np

# Linear sRGB -> LMS, then the cube root of LMS -> Lab.
_LMS_FROM_LINEAR = np.array(
    [
        [0.4122214708, 0.5363325363, 0.0514459929],
        [0.2119034982, 0.6806995451, 0.1073969566],
        [0.0883024619, 0.2817188376, 0.6299787005],
    ]
)

_LAB_FROM_LMS = np.array(
    [
        [0.2104542553, 0.7936177850, -0.0040720468],
        [1.9779984951, -2.4285922050, 0.4505937099],
        [0.0259040371, 0.7827717662, -0.8086757660],
    ]
)

_LMS_FROM_LAB = np.array(
    [
        [1.0, 0.3963377774, 0.2158037573],
        [1.0, -0.1055613458, -0.0638541728],
        [1.0, -0.0894841775, -1.2914855480],
    ]
)

_LINEAR_FROM_LMS = np.array(
    [
        [4.0767416621, -3.3077115913, 0.2309699292],
        [-1.2684380046, 2.6097574011, -0.3413193965],
        [-0.0041960863, -0.7034186147, 1.7076147010],
    ]
)


def srgb_to_linear(srgb: np.ndarray) -> np.ndarray:
    """Undo the sRGB transfer function. Input and output are 0..1 floats."""
    srgb = np.asarray(srgb, dtype=np.float64)
    return np.where(srgb <= 0.04045, srgb / 12.92, ((srgb + 0.055) / 1.055) ** 2.4)


def linear_to_srgb(linear: np.ndarray) -> np.ndarray:
    """Apply the sRGB transfer function. Input and output are 0..1 floats."""
    linear = np.asarray(linear, dtype=np.float64)
    return np.where(
        linear <= 0.0031308, linear * 12.92, 1.055 * np.clip(linear, 0, None) ** (1 / 2.4) - 0.055
    )


def srgb_to_oklab(rgb: np.ndarray) -> np.ndarray:
    """Convert gamma-encoded sRGB in 0..1 to OKLab. Shape (..., 3) throughout."""
    linear = srgb_to_linear(rgb)
    lms = linear @ _LMS_FROM_LINEAR.T
    # Cube root is the perceptual compression; guard the sign so out-of-gamut
    # negatives round-trip instead of producing NaN.
    lms_cbrt = np.cbrt(lms)
    return lms_cbrt @ _LAB_FROM_LMS.T


def oklab_to_srgb(lab: np.ndarray) -> np.ndarray:
    """Convert OKLab back to gamma-encoded sRGB, clipped to 0..1."""
    lms_cbrt = np.asarray(lab, dtype=np.float64) @ _LMS_FROM_LAB.T
    lms = lms_cbrt**3
    linear = lms @ _LINEAR_FROM_LMS.T
    return np.clip(linear_to_srgb(linear), 0.0, 1.0)


def chroma(lab: np.ndarray) -> np.ndarray:
    """Distance from the neutral axis: how colourful, independent of lightness."""
    lab = np.asarray(lab, dtype=np.float64)
    return np.hypot(lab[..., 1], lab[..., 2])

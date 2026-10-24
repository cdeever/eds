"""Recordings made here, so none has to be committed: a tone stands in for
speech, since what is tested is the reading, not the words."""

from __future__ import annotations

import io
import wave

import numpy as np
import pytest


def recording(seconds: float = 2.0, rate: int = 16_000, channels: int = 1, width: int = 2) -> bytes:
    t = np.arange(int(seconds * rate)) / rate
    tone = (0.3 * np.sin(2 * np.pi * 440 * t) * 32767).astype("<i2")
    if channels > 1:
        tone = np.repeat(tone, channels)
    buf = io.BytesIO()
    with wave.open(buf, "wb") as w:
        w.setnchannels(channels)
        w.setsampwidth(width)
        w.setframerate(rate)
        w.writeframes(tone.tobytes() if width == 2 else tone.astype("<i4").tobytes())
    return buf.getvalue()


@pytest.fixture
def make_recording():
    return recording

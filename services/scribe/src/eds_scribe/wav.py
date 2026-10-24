"""Reading the one kind of recording scribe takes.

A request arrives as the device recorded it: a WAV of 16 kHz, 16-bit mono
PCM (ADR-0042). Anything else is refused rather than converted, because
there is only one sender and a different recording means something is wrong
with it, not that it needs help.
"""

from __future__ import annotations

import io
import wave

import numpy as np

RATE = 16_000
MIN_SECONDS = 0.3
MAX_SECONDS = 15.0


class InvalidRecordingError(ValueError):
    """The bytes are not a recording scribe takes."""


def read(data: bytes) -> np.ndarray:
    """A WAV's samples as float32 in [-1, 1], as Whisper takes them."""
    try:
        with wave.open(io.BytesIO(data), "rb") as w:
            channels, width, rate, frames = (
                w.getnchannels(),
                w.getsampwidth(),
                w.getframerate(),
                w.getnframes(),
            )
            if (channels, width, rate) != (1, 2, RATE):
                raise InvalidRecordingError(
                    f"{rate} Hz, {8 * width}-bit, {channels} channel(s); "
                    f"expected {RATE} Hz, 16-bit mono"
                )
            seconds = frames / rate
            if not MIN_SECONDS <= seconds <= MAX_SECONDS:
                raise InvalidRecordingError(
                    f"{seconds:.2f} s; expected {MIN_SECONDS} to {MAX_SECONDS} s"
                )
            pcm = w.readframes(frames)
    except (wave.Error, EOFError) as e:
        raise InvalidRecordingError(f"not a WAV: {e}") from e
    samples = np.frombuffer(pcm, dtype="<i2").astype(np.float32) / 32768.0
    if samples.size == 0:
        raise InvalidRecordingError("no samples")
    return samples


def seconds(samples: np.ndarray) -> float:
    return samples.size / RATE

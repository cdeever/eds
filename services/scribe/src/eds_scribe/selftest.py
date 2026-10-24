"""The image's own check, run when it is built: the model loads offline and
writes down a second of tone as nothing it is sure of.

It is what says the image works without the packages taken out of it -
PyAV, onnxruntime - before the image is ever sent anywhere.
"""

from __future__ import annotations

import sys

import numpy as np

from .transcribe import Whisper


def main() -> int:
    t = np.arange(16_000) / 16_000
    tone = (0.3 * np.sin(2 * np.pi * 440 * t)).astype(np.float32)
    heard = Whisper()(tone)
    print(f"selftest: {heard.text!r}, confident={heard.confident}")
    return 1 if heard.confident else 0


if __name__ == "__main__":
    sys.exit(main())

"""Speech to words, with faster-whisper.

The model is loaded once, on first use, and kept: loading takes longer than
writing down a request. Its settings are environment variables, read then.
"""

from __future__ import annotations

import os
import threading
from typing import Protocol

import numpy as np

# Requests are short and said to a music system. A prompt in the same voice
# makes the model likelier to write "Van Halen's" than "van Halen's", and
# "play" than "pray".
PROMPT = "Play Van Halen's first album. Play Abbey Road by the Beatles."


class Transcriber(Protocol):
    def __call__(self, samples: np.ndarray) -> str: ...


class Whisper:
    """faster-whisper, on the CPU, quantized to int8."""

    def __init__(self) -> None:
        self.model_name = os.environ.get("SCRIBE_MODEL", "base.en")
        self.threads = int(os.environ.get("SCRIBE_THREADS", "2"))
        self._model = None
        self._lock = threading.Lock()

    def _load(self):
        from faster_whisper import WhisperModel  # heavy; loaded when needed

        return WhisperModel(
            self.model_name, device="cpu", compute_type="int8", cpu_threads=self.threads
        )

    def __call__(self, samples: np.ndarray) -> str:
        # One at a time: two at once on a two-core workload are each slower
        # than both in turn.
        with self._lock:
            if self._model is None:
                self._model = self._load()
            segments, _ = self._model.transcribe(
                samples,
                language="en",
                beam_size=1,
                initial_prompt=PROMPT,
                condition_on_previous_text=False,
                vad_filter=False,  # the device has already cut the silence
            )
            return " ".join(s.text.strip() for s in segments).strip()

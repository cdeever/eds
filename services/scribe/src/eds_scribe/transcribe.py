"""Speech to words, with faster-whisper.

The model is loaded once and kept: loading takes longer than writing down a
request. The service loads it as it starts (`load`), so that the first
request after a restart is not the one that waits; whatever has not been
loaded by then is loaded on first use. Its settings are environment
variables, read when it is made.
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

    def load(self) -> None:
        """Load the model now, if it is not yet. A request that arrives
        meanwhile waits for it, rather than loading a second."""
        with self._lock:
            self._loaded()

    def _loaded(self):
        if self._model is None:
            from faster_whisper import WhisperModel  # heavy; imported when needed

            self._model = WhisperModel(
                self.model_name, device="cpu", compute_type="int8", cpu_threads=self.threads
            )
        return self._model

    def __call__(self, samples: np.ndarray) -> str:
        # One at a time: two at once on a two-core workload are each slower
        # than both in turn.
        with self._lock:
            segments, _ = self._loaded().transcribe(
                samples,
                language="en",
                beam_size=1,
                initial_prompt=PROMPT,
                condition_on_previous_text=False,
                vad_filter=False,  # the device has already cut the silence
            )
            return " ".join(s.text.strip() for s in segments).strip()

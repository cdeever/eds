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
from dataclasses import dataclass
from typing import Protocol

import numpy as np

# Names, as they are spelled, so that the model is likelier to write
# "Van Halen's" than "van Halen's" and "AC/DC" than "ACDC".
#
# Never a sentence a request could be. Given what it cannot make out, the
# model can write its prompt back instead; when the prompt was "Play Abbey
# Road by the Beatles", that was a request, and on 2026-10-24 the Beatles
# played for a request nobody made (INC-0001). A list of names is not one.
PROMPT = "Aerosmith, Van Halen, AC/DC, Led Zeppelin, Guns N' Roses, the Beatles, Pink Floyd."

# Whisper's own thresholds, as its reference implementation uses them to
# decide that a stretch of audio held no speech: a segment the model thinks
# more likely silent than not, or whose words it is not sure of, is not
# taken for words.
NO_SPEECH_ABOVE = 0.6
UNSURE_BELOW = -1.0


@dataclass
class Heard:
    text: str
    # The likelihood the model gives that there was no speech at all, the
    # least of any segment's, and its mean log probability for the words.
    no_speech_prob: float
    avg_logprob: float

    @property
    def confident(self) -> bool:
        """Whether the words are worth acting on: something was said, and the
        model is sure enough of what."""
        return bool(self.text) and self.no_speech_prob < NO_SPEECH_ABOVE and self.avg_logprob > UNSURE_BELOW


class Transcriber(Protocol):
    def __call__(self, samples: np.ndarray) -> Heard: ...


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

    def __call__(self, samples: np.ndarray) -> Heard:
        # One at a time: two at once on a two-core workload are each slower
        # than both in turn.
        with self._lock:
            segments, _ = self._loaded().transcribe(
                samples,
                language="en",
                beam_size=1,
                # Once. By default an unsure decode is tried again, warmer, up
                # to five more times: speech scribe cannot make out took ten
                # seconds to come back not confident (CHG-0036), and words
                # it is not sure of are not acted on anyway.
                temperature=0.0,
                initial_prompt=PROMPT,
                condition_on_previous_text=False,
                vad_filter=False,  # the device sends what it heard, silence and all
            )
            segments = list(segments)  # transcribed as it is read
        if not segments:
            return Heard(text="", no_speech_prob=1.0, avg_logprob=-10.0)
        return Heard(
            text=" ".join(s.text.strip() for s in segments).strip(),
            no_speech_prob=min(s.no_speech_prob for s in segments),
            avg_logprob=sum(s.avg_logprob for s in segments) / len(segments),
        )

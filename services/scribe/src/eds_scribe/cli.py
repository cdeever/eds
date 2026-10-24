"""Write down WAVs from the command line: the tuning loop, no service."""

from __future__ import annotations

import sys
import time
from pathlib import Path

from . import wav
from .transcribe import Whisper


def main() -> int:
    if len(sys.argv) < 2:
        print("usage: eds-scribe FILE.wav ...", file=sys.stderr)
        return 2
    whisper = Whisper()
    for name in sys.argv[1:]:
        try:
            samples = wav.read(Path(name).read_bytes())
        except (OSError, wav.InvalidRecordingError) as e:
            print(f"{name}: {e}", file=sys.stderr)
            continue
        started = time.monotonic()
        text = whisper(samples)
        print(f"{name}\t{time.monotonic() - started:.2f}s\t{text}")
    return 0


if __name__ == "__main__":
    sys.exit(main())

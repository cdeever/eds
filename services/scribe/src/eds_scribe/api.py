"""HTTP surface for scribe.

The service knows nothing about music, MQTT or albums (ADR-0042). It takes a
recording and returns its words. That boundary is what lets it be developed
against a folder of WAVs with no infrastructure at all, as palette is
against a folder of covers.
"""

from __future__ import annotations

import time

from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel, Field

from . import __version__, wav
from .transcribe import Transcriber, Whisper

CONTRACT_VERSION = 1

app = FastAPI(
    title="EdS scribe",
    version=__version__,
    summary="A short spoken request in, its words out.",
)

# Replaced in tests; the real one loads its model on first use.
transcriber: Transcriber = Whisper()


class Transcript(BaseModel):
    v: int = CONTRACT_VERSION
    text: str = Field(description="What was said, as the model wrote it. Empty when nothing was.")
    seconds: float = Field(description="How long the recording is.")
    took_ms: int = Field(description="How long writing it down took.")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok", "version": __version__}


@app.post("/v1/transcribe", response_model=Transcript)
async def transcribe(request: Request) -> Transcript:
    # Read whole, but no more than a recording can be: fifteen seconds at
    # 16 kHz, 16-bit, is under half a megabyte.
    body = bytearray()
    async for chunk in request.stream():
        body.extend(chunk)
        if len(body) > 1_000_000:
            raise HTTPException(status_code=413, detail="larger than any recording scribe takes")
    try:
        samples = wav.read(bytes(body))
    except wav.InvalidRecordingError as e:
        raise HTTPException(status_code=422, detail=str(e)) from e

    started = time.monotonic()
    text = transcriber(samples)
    took = int((time.monotonic() - started) * 1000)
    return Transcript(text=text, seconds=round(wav.seconds(samples), 2), took_ms=took)

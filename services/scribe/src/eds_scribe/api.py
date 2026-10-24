"""HTTP surface for scribe.

The service knows nothing about music, MQTT or albums (ADR-0042). It takes a
recording and returns its words. That boundary is what lets it be developed
against a folder of WAVs with no infrastructure at all, as palette is
against a folder of covers.
"""

from __future__ import annotations

import logging
import os
import threading
import time
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException, Request
from fastapi.concurrency import run_in_threadpool
from pydantic import BaseModel, Field

from . import __version__, wav
from .transcribe import Transcriber, Whisper

CONTRACT_VERSION = 1

log = logging.getLogger("uvicorn.error")

# Replaced in tests; the real one loads its model as the service starts.
transcriber: Transcriber = Whisper()


def _preload() -> None:
    started = time.monotonic()
    try:
        transcriber.load()
    except Exception:  # loaded again on the first request, and said there
        log.exception("the model could not be loaded")
        return
    log.info("model loaded in %.1f s", time.monotonic() - started)


@asynccontextmanager
async def lifespan(_: FastAPI):
    # In the background, so that the service answers /healthz while the model
    # loads, and a request that arrives meanwhile waits for it.
    # SCRIBE_PRELOAD=0 leaves it to the first request.
    if hasattr(transcriber, "load") and os.environ.get("SCRIBE_PRELOAD", "1") != "0":
        threading.Thread(target=_preload, name="preload", daemon=True).start()
    yield


app = FastAPI(
    title="EdS scribe",
    version=__version__,
    summary="A short spoken request in, its words out.",
    lifespan=lifespan,
)


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
    # On a thread of its own: writing down takes seconds, and the event loop
    # has /healthz to answer meanwhile.
    text = await run_in_threadpool(transcriber, samples)
    took = int((time.monotonic() - started) * 1000)
    return Transcript(text=text, seconds=round(wav.seconds(samples), 2), took_ms=took)

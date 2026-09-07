"""HTTP surface for the palette extractor.

The service knows nothing about music, MQTT or LEDs. It takes image bytes and
returns perceptual colour data. That boundary is what lets it be developed and
tuned against a folder of JPEGs with no infrastructure at all, and it is the
interface the album cover resolver will eventually call through lightd.
"""

from __future__ import annotations

from fastapi import FastAPI, File, HTTPException, Query, Request, UploadFile
from pydantic import BaseModel, Field

from . import __version__
from .extract import (
    DEFAULT_SWATCHES,
    InvalidImageError,
    extract_palette,
)

CONTRACT_VERSION = 1

app = FastAPI(
    title="EdS palette extractor",
    version=__version__,
    summary="Ranked, weighted colour swatches from album cover art.",
)


class SwatchModel(BaseModel):
    rgb: tuple[int, int, int]
    hex: str
    oklab: tuple[float, float, float]
    weight: float = Field(description="Share of clustered pixels in this cluster.")
    chroma: float
    lightness: float


class SourceModel(BaseModel):
    width: int
    height: int
    format: str


class PaletteResponse(BaseModel):
    v: int = CONTRACT_VERSION
    swatches: list[SwatchModel]
    background_dropped: float = Field(
        description="Share of pixels removed as background before clustering."
    )
    fallback: bool = Field(
        description=(
            "True when background filtering left too little of the image to "
            "cluster and the unfiltered pixels were used instead."
        )
    )
    source: SourceModel


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.post("/v1/palette", response_model=PaletteResponse)
async def palette(
    request: Request,
    n: int = Query(DEFAULT_SWATCHES, ge=1, le=16, description="Swatches to return."),
    image: UploadFile | None = File(None),
) -> PaletteResponse:
    """Extract a palette from a multipart `image` field or a raw request body."""
    data = await image.read() if image is not None else await request.body()
    if not data:
        raise HTTPException(status_code=400, detail="no image supplied")

    try:
        result = extract_palette(data, swatches=n)
    except InvalidImageError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc

    return PaletteResponse(
        swatches=[SwatchModel(**vars(s)) for s in result.swatches],
        background_dropped=result.background_dropped,
        fallback=result.fallback,
        source=SourceModel(**vars(result.source)),
    )

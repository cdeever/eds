from __future__ import annotations

import pytest
from fastapi.testclient import TestClient

from eds_palette.api import app

client = TestClient(app)


def test_healthz():
    response = client.get("/healthz")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_multipart_upload(two_tone):
    """The shape the verification plan uses: curl -F image=@jacket.jpg"""
    response = client.post("/v1/palette", files={"image": ("cover.png", two_tone, "image/png")})

    assert response.status_code == 200
    body = response.json()
    assert body["v"] == 1
    assert len(body["swatches"]) >= 2
    assert body["source"] == {"width": 200, "height": 200, "format": "png"}


def test_raw_body_upload(two_tone):
    """The shape lightd will use: POST the bytes, no multipart envelope."""
    response = client.post(
        "/v1/palette", content=two_tone, headers={"Content-Type": "image/png"}
    )

    assert response.status_code == 200
    assert response.json()["swatches"]


def test_multipart_and_raw_agree(two_tone):
    multipart = client.post("/v1/palette", files={"image": ("c.png", two_tone, "image/png")})
    raw = client.post("/v1/palette", content=two_tone, headers={"Content-Type": "image/png"})
    assert multipart.json()["swatches"] == raw.json()["swatches"]


def test_swatch_count_parameter(three_tone):
    response = client.post(
        "/v1/palette", params={"n": 3}, content=three_tone, headers={"Content-Type": "image/png"}
    )
    assert response.status_code == 200
    assert len(response.json()["swatches"]) == 3


@pytest.mark.parametrize("n", [0, 17, -1])
def test_rejects_out_of_range_swatch_counts(three_tone, n):
    response = client.post(
        "/v1/palette", params={"n": n}, content=three_tone, headers={"Content-Type": "image/png"}
    )
    assert response.status_code == 422


def test_rejects_empty_body():
    response = client.post("/v1/palette", content=b"", headers={"Content-Type": "image/png"})
    assert response.status_code == 400
    assert response.json()["detail"] == "no image supplied"


def test_rejects_garbage():
    response = client.post(
        "/v1/palette", content=b"not an image at all", headers={"Content-Type": "image/png"}
    )
    assert response.status_code == 400


def test_reports_fallback_for_monochrome_art(all_black):
    response = client.post(
        "/v1/palette", content=all_black, headers={"Content-Type": "image/png"}
    )
    body = response.json()
    assert body["fallback"] is True
    assert body["background_dropped"] == pytest.approx(1.0, abs=1e-6)


def test_response_carries_the_full_contract(black_field):
    response = client.post(
        "/v1/palette", content=black_field, headers={"Content-Type": "image/png"}
    )
    body = response.json()

    assert set(body) == {"v", "swatches", "background_dropped", "fallback", "source"}
    assert set(body["swatches"][0]) == {
        "rgb",
        "hex",
        "oklab",
        "weight",
        "chroma",
        "lightness",
    }


def test_openapi_schema_is_generated():
    """lightd is a separate language; a published schema is how the contract
    stays honest across that boundary."""
    schema = client.get("/openapi.json").json()
    assert "/v1/palette" in schema["paths"]
    assert "PaletteResponse" in schema["components"]["schemas"]

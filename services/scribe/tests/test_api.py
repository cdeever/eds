import pytest
from fastapi.testclient import TestClient

from eds_scribe import api


@pytest.fixture
def client(monkeypatch):
    heard = []

    def fake(samples):
        heard.append(samples.size)
        return "Play Van Halen's first album."

    monkeypatch.setattr(api, "transcriber", fake)
    c = TestClient(api.app)
    c.heard = heard
    return c


def test_a_recording_is_written_down(client, make_recording):
    r = client.post("/v1/transcribe", content=make_recording(2.5), headers={"Content-Type": "audio/wav"})
    assert r.status_code == 200
    body = r.json()
    assert body["v"] == 1
    assert body["text"] == "Play Van Halen's first album."
    assert body["seconds"] == 2.5
    assert body["took_ms"] >= 0
    assert client.heard == [40_000]


def test_a_recording_of_the_wrong_kind_is_refused_and_not_heard(client, make_recording):
    r = client.post("/v1/transcribe", content=make_recording(2.0, rate=44_100))
    assert r.status_code == 422
    assert "expected 16000 Hz, 16-bit mono" in r.json()["detail"]
    assert client.heard == []


def test_more_than_any_recording_is_refused(client):
    r = client.post("/v1/transcribe", content=b"\x00" * 1_000_001)
    assert r.status_code == 413
    assert client.heard == []


def test_health(client):
    assert client.get("/healthz").json()["status"] == "ok"

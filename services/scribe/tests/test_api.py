import threading

import pytest
from fastapi.testclient import TestClient

from eds_scribe import api
from eds_scribe.transcribe import Heard


@pytest.fixture
def client(monkeypatch):
    heard = []

    def fake(samples):
        heard.append(samples.size)
        return Heard("Play Van Halen's first album.", no_speech_prob=0.02, avg_logprob=-0.2)

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
    assert body["confident"] is True
    assert body["no_speech_prob"] == 0.02
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


class Loading:
    """A transcriber that says when it was loaded."""

    def __init__(self):
        self.loaded = threading.Event()

    def load(self):
        self.loaded.set()

    def __call__(self, samples):
        return Heard("Play Rocks by Aerosmith.", no_speech_prob=0.02, avg_logprob=-0.2)


def test_the_model_is_loaded_as_the_service_starts(monkeypatch):
    fake = Loading()
    monkeypatch.setattr(api, "transcriber", fake)
    with TestClient(api.app):
        assert fake.loaded.wait(2), "the model was not loaded at startup"


def test_preloading_can_be_left_to_the_first_request(monkeypatch):
    fake = Loading()
    monkeypatch.setattr(api, "transcriber", fake)
    monkeypatch.setenv("SCRIBE_PRELOAD", "0")
    with TestClient(api.app):
        assert not fake.loaded.wait(0.2)


@pytest.mark.parametrize(
    "heard",
    [
        Heard("Play Abbey Road by the Beatles.", no_speech_prob=0.81, avg_logprob=-0.4),  # likely silence
        Heard("Play Abbey Road by the Beatles.", no_speech_prob=0.1, avg_logprob=-1.6),  # unsure of the words
        Heard("", no_speech_prob=0.1, avg_logprob=-0.2),  # nothing written
    ],
    ids=["no speech", "unsure", "empty"],
)
def test_words_the_model_is_not_sure_of_are_said_to_be(monkeypatch, make_recording, heard):
    monkeypatch.setattr(api, "transcriber", lambda samples: heard)
    r = TestClient(api.app).post("/v1/transcribe", content=make_recording(2.0))
    assert r.status_code == 200
    assert r.json()["confident"] is False


def test_the_prompt_is_not_a_request():
    from eds_scribe.transcribe import PROMPT

    assert "play" not in PROMPT.lower() and " by " not in PROMPT.lower()

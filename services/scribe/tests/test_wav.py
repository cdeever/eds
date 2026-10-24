import numpy as np
import pytest

from eds_scribe import wav


def test_a_request_is_read_as_floats(make_recording):
    samples = wav.read(make_recording(2.0))
    assert samples.dtype == np.float32
    assert samples.size == 32_000
    assert -1.0 <= samples.min() < 0 < samples.max() <= 1.0
    assert wav.seconds(samples) == 2.0


@pytest.mark.parametrize(
    "kind",
    [
        {"rate": 44_100},
        {"channels": 2},
        {"width": 4},
        {"seconds": 0.1},
        {"seconds": 16.0},
    ],
    ids=["44.1 kHz", "stereo", "32-bit", "too short", "too long"],
)
def test_anything_but_the_one_kind_is_refused(make_recording, kind):
    with pytest.raises(wav.InvalidRecordingError):
        wav.read(make_recording(**kind))


@pytest.mark.parametrize("data", [b"", b"not a wav at all", b"RIFF\x00\x00"])
def test_what_is_not_a_wav_is_refused(data):
    with pytest.raises(wav.InvalidRecordingError):
        wav.read(data)

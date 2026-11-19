"""Downloads what the clips are mixed with, and the ready-made negatives.

All of it goes to WORK/data, made 16 kHz mono 16-bit where it is audio:

  mit_rirs/           how rooms ring (MIT's impulse responses)
  audioset_16k/       ten-second clips of everything (one part of AudioSet)
  fma_16k/            music (the Free Music Archive's extra-small set)
  negative_datasets/  microWakeWord's own features of speech, parties and noise

The same sources microWakeWord's notebook uses (2026-11-11), with AudioSet
read from the parquet files it is now published as. Their licenses are
mixed; what is trained on them is for personal, non-commercial use.
A part already present is not fetched again.
"""

import argparse
import io
import shutil
import urllib.request
import zipfile
from pathlib import Path

import numpy as np
import pyarrow.parquet as pq
import scipy.io.wavfile
import soundfile
from huggingface_hub import snapshot_download
from scipy.signal import resample_poly
from tqdm import tqdm

from common import Work, remove_tree, tidy

HF = "https://huggingface.co/datasets/"
AUDIOSET_PART = HF + "agkphysics/AudioSet/resolve/main/data/bal_train/09.parquet"
FMA_ZIP = HF + "mchl914/fma_xsmall/resolve/main/fma_xs.zip"
NEGATIVES = HF + "kahrendt/microwakeword/resolve/main/"
NEGATIVE_SETS = ["dinner_party", "dinner_party_eval", "no_speech", "speech"]


def download(url, dest):
    if dest.exists():
        return dest
    part = dest.with_suffix(dest.suffix + ".part")
    print(f"fetching {url}")
    with urllib.request.urlopen(url) as r, open(part, "wb") as f:
        total = int(r.headers.get("Content-Length", 0)) or None
        with tqdm(total=total, unit="B", unit_scale=True) as bar:
            while chunk := r.read(1 << 20):
                f.write(chunk)
                bar.update(len(chunk))
    part.rename(dest)
    return dest


def write_16k(samples, rate, dest):
    """Mono, 16 kHz, 16-bit: what every clip is made into."""
    if samples.ndim > 1:
        samples = samples.mean(axis=1)
    if rate != 16000:
        g = np.gcd(int(rate), 16000)
        samples = resample_poly(samples, 16000 // g, int(rate) // g)
    samples = np.clip(samples, -1.0, 1.0)
    scipy.io.wavfile.write(dest, 16000, (samples * 32767).astype(np.int16))


def rirs(data):
    out = data / "mit_rirs"
    if out.exists():
        if (data / "mit_rirs_download").exists():
            remove_tree(data / "mit_rirs_download")
        return
    got = Path(
        snapshot_download(
            "davidscripka/MIT_environmental_impulse_responses",
            repo_type="dataset",
            allow_patterns=["16khz/*.wav"],
            local_dir=data / "mit_rirs_download",
        )
    )
    shutil.move(str(got / "16khz"), str(out))
    remove_tree(data / "mit_rirs_download")


def audioset(data):
    out = data / "audioset_16k"
    if out.exists():
        return
    part = download(AUDIOSET_PART, data / "audioset_09.parquet")
    tmp = data / "audioset_16k.tmp"
    tmp.mkdir(exist_ok=True)
    table = pq.read_table(part)
    audio_col = next(
        name
        for name in table.column_names
        if str(table.schema.field(name).type).startswith("struct") and "bytes" in str(table.schema.field(name).type)
    )
    skipped = 0
    for i, row in enumerate(tqdm(table.column(audio_col).to_pylist(), desc="audioset")):
        try:
            samples, rate = soundfile.read(io.BytesIO(row["bytes"]), dtype="float32")
        except Exception:
            skipped += 1
            continue
        write_16k(samples, rate, tmp / f"audioset_{i:05d}.wav")
    tmp.rename(out)
    part.unlink()
    print(f"audioset: {skipped} clips could not be read and were left out")


def fma(data):
    out = data / "fma_16k"
    if out.exists():
        return
    zpath = download(FMA_ZIP, data / "fma_xs.zip")
    tmp = data / "fma_16k.tmp"
    tmp.mkdir(exist_ok=True)
    skipped = 0
    with zipfile.ZipFile(zpath) as z:
        names = [n for n in z.namelist() if n.endswith(".mp3") and "/._" not in n]
        for name in tqdm(names, desc="fma"):
            try:
                samples, rate = soundfile.read(io.BytesIO(z.read(name)), dtype="float32")
            except Exception:
                skipped += 1
                continue
            write_16k(samples, rate, tmp / (Path(name).stem + ".wav"))
    tmp.rename(out)
    zpath.unlink()
    print(f"fma: {skipped} songs could not be read and were left out")


def negatives(data):
    out = data / "negative_datasets"
    out.mkdir(exist_ok=True)
    for name in NEGATIVE_SETS:
        if (out / name).exists():
            continue
        zpath = download(NEGATIVES + name + ".zip", data / (name + ".zip"))
        print(f"unpacking {name}")
        with zipfile.ZipFile(zpath) as z:
            z.extractall(out)
        zpath.unlink()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", required=True)
    args = ap.parse_args()
    work = Work(args.work)
    work.data.mkdir(parents=True, exist_ok=True)

    rirs(work.data)
    audioset(work.data)
    fma(work.data)
    negatives(work.data)
    tidy(work.data)
    print(f"done: {work.data}")


if __name__ == "__main__":
    main()

"""Makes the features the model is trained on, from the clips in WORK.

  features/positive/     "Hey DJ Ed", mixed with rooms and noise  (truth: yes)
  features/near_misses/  what must not wake it, mixed the same way (truth: no)
  features/music/        the room's own music, when `make music` made it (truth: no)
  features/music_eval/   other songs of it, only to measure false wakes an hour

The spoken sets have training/, validation/ and testing/ as microWakeWord's
notebook makes them. music_eval has validation_ambient/ and testing_ambient/,
so false wakes an hour are also measured on what this room plays. A set already made is not made again; delete its folder to remake it.
"""

import argparse

from mmap_ninja.ragged import RaggedMmap
from microwakeword.audio.augmentation import Augmentation
from microwakeword.audio.clips import Clips
from microwakeword.audio.spectrograms import SpectrogramGeneration

from common import Work, remove_tree, tidy

# The notebook's (2026-11-11). The clip is 3.2 s with the phrase at its end,
# give or take 5 ms: what the model is shown is the moment the phrase ends.
AUGMENT = dict(
    augmentation_duration_s=3.2,
    augmentation_probabilities={
        "SevenBandParametricEQ": 0.1,
        "TanhDistortion": 0.1,
        "PitchShift": 0.1,
        "BandStopFilter": 0.1,
        "AddColorNoise": 0.1,
        "AddBackgroundNoise": 0.75,
        "Gain": 1.0,
        "RIR": 0.5,
    },
    background_min_snr_db=-5,
    background_max_snr_db=10,
    min_jitter_s=0.195,
    max_jitter_s=0.205,
)


def augmenter(work):
    return Augmentation(
        impulse_paths=[str(work.data / "mit_rirs")],
        background_paths=[str(work.data / "fma_16k"), str(work.data / "audioset_16k")],
        **AUGMENT,
    )


def write(out_dir, name, generator):
    """One set, written whole or not at all."""
    final = out_dir / f"{name}_mmap"
    if final.exists():
        return
    tmp = out_dir / f"{name}_mmap.tmp"
    if tmp.exists():
        remove_tree(tmp)
    out_dir.mkdir(parents=True, exist_ok=True)
    RaggedMmap.from_generator(out_dir=str(tmp), sample_generator=generator, batch_size=100, verbose=True)
    tmp.rename(final)


def spoken(work, kind, out, name):
    """Clips of something said: augmented, then the notebook's three splits."""
    if not any((work.samples / kind).glob("*/*.wav")):
        raise SystemExit(f"no clips in {work.samples / kind}: run `make generate` first")
    clips = Clips(
        input_directory=str(work.samples / kind),
        file_pattern="*/[0-9]*.wav",
        max_clip_duration_s=None,
        remove_silence=False,
        random_split_seed=10,
        split_count=0.1,
    )
    aug = augmenter(work)
    # Training sees each clip twice, slid a frame at a time over ten frames,
    # as a streaming model sees it; testing streams, so it is not slid.
    plan = [
        ("training", "train", 2, 10),
        ("validation", "validation", 1, 10),
        ("testing", "test", 1, 1),
    ]
    for folder, split, repeat, slide in plan:
        spec = SpectrogramGeneration(clips=clips, augmenter=aug, slide_frames=slide, step_ms=10)
        write(out / folder, name, spec.spectrogram_generator(split=split, repeat=repeat))


def music(work, out):
    """The room's music, as it is: three-second pieces to train on, whole songs to test."""
    if not work.music.exists() or not any(work.music.glob("*.wav")):
        print("no music in WORK (`make music MUSIC=...`): trained without it")
        return
    clips = Clips(
        input_directory=str(work.music),
        file_pattern="[!.]*.wav",
        max_clip_duration_s=None,
        remove_silence=False,
        random_split_seed=10,
        split_count=0.1,
    )
    # As the ready-made dinner_party and dinner_party_eval are: one set to
    # train on, cut at random, and one only to measure, split as it streams.
    plan = [
        (out / "training", "train", dict(split_spectrogram_duration_s=3.0)),
        (out.with_name("music_eval") / "validation_ambient", "validation", {}),
        (out.with_name("music_eval") / "testing_ambient", "test", {}),
    ]
    for folder, split, extra in plan:
        spec = SpectrogramGeneration(clips=clips, augmenter=None, step_ms=10, **extra)
        write(folder, "music", spec.spectrogram_generator(split=split))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", required=True)
    args = ap.parse_args()
    work = Work(args.work)
    for need in ("mit_rirs", "fma_16k", "audioset_16k"):
        if not (work.data / need).exists():
            raise SystemExit(f"no {work.data / need}: run `make fetch` first")
    tidy(work.root)

    spoken(work, "positive", work.features / "positive", "wakeword")
    spoken(work, "negative", work.features / "near_misses", "near_misses")
    music(work, work.features / "music")
    tidy(work.root)
    print(f"done: {work.features}")


if __name__ == "__main__":
    main()

"""Has Piper say each phrase in a list, in many voices, as 16 kHz WAVs.

  generate.py --work W --listen phrases.txt          one clip a phrase, in W/listen
  generate.py --work W --samples N list.txt KIND     N clips a phrase, in W/samples/KIND/<phrase>

A phrase that already has N clips is not generated again.
"""

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

from common import Work, phrases, remove_tree, slug, tidy


def run_piper(work, phrase, count, out_dir):
    out_dir.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        [
            sys.executable,
            "generate_samples.py",
            phrase,
            "--max-samples",
            str(count),
            "--batch-size",
            str(min(count, 50)),
            "--output-dir",
            str(out_dir),
        ],
        cwd=work.piper,
        check=True,
        # The voice is a whole pickled model, which Torch 2.6 and later load
        # only when told to. It is the one `make work` checked by its hash.
        env={**os.environ, "TORCH_FORCE_NO_WEIGHTS_ONLY_LOAD": "1"},
    )


def clips_in(directory):
    return [p for p in directory.glob("*.wav") if not p.name.startswith("._")]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", required=True)
    ap.add_argument("--listen", action="store_true")
    ap.add_argument("--samples", type=int, default=1)
    ap.add_argument("list")
    ap.add_argument("kind", nargs="?", default="positive", choices=["positive", "negative"])
    args = ap.parse_args()

    work = Work(args.work)
    tidy(work.root)

    if args.listen:
        listen = work.root / "listen"
        listen.mkdir(exist_ok=True)
        for phrase in phrases(args.list):
            tmp = listen / ("tmp_" + slug(phrase))
            run_piper(work, phrase, 1, tmp)
            shutil.move(str(tmp / "0.wav"), str(listen / (slug(phrase) + ".wav")))
            remove_tree(tmp)
            print(f"{phrase!r}: {listen / (slug(phrase) + '.wav')}")
        return

    for phrase in phrases(args.list):
        out_dir = work.samples / args.kind / slug(phrase)
        have = len(clips_in(out_dir)) if out_dir.exists() else 0
        if have >= args.samples:
            print(f"{phrase!r}: {have} clips already")
            continue
        if out_dir.exists():
            remove_tree(out_dir)
        print(f"{phrase!r}: {args.samples} clips")
        run_piper(work, phrase, args.samples, out_dir)
    tidy(work.root)


if __name__ == "__main__":
    main()

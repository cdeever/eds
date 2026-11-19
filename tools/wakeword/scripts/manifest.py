"""The trained model and its manifest, in WORK/out, with the cutoff chosen.

Training ends by testing the quantized streaming model: at each cutoff, the
share of "Hey DJ Ed"s missed (frr) and the false wakes an hour (faph) on
the ambient sets. The cutoff kept is the one that misses fewest while
waking falsely no more than --max-faph an hour.

The manifest is ESPHome's shape, as hey_jarvis.json is; app/wake is told its
numbers in Kconfig. tensor_arena_size is the Jarvis model's until the ears
say what this one uses, in their boot log.
"""

import argparse
import json
import re
import shutil

from common import Work

ROC_LINE = re.compile(r"Cutoff ([0-9.]+): frr=([0-9.]+); faph=([0-9.]+)")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", required=True)
    ap.add_argument("--max-faph", type=float, default=0.05)
    ap.add_argument("--arena", type=int, default=22860)
    args = ap.parse_args()
    work = Work(args.work)

    folder = work.trained / "tflite_stream_state_internal_quant"
    roc = folder / "tflite_streaming_roc.txt"
    if not roc.exists():
        raise SystemExit(f"no {roc}: run `make train` first")

    points = []
    for line in roc.read_text().splitlines():
        m = ROC_LINE.search(line)
        if m:
            points.append(tuple(float(x) for x in m.groups()))
        elif line.strip():
            print(line)
    print(f"{'cutoff':>7} {'missed':>7} {'false/h':>8}")
    for cutoff, frr, faph in points:
        print(f"{cutoff:7.2f} {frr:7.1%} {faph:8.3f}")

    viable = [p for p in points if p[2] <= args.max_faph]
    if not viable:
        raise SystemExit(f"no cutoff wakes falsely {args.max_faph} times an hour or less: train again")
    cutoff, frr, faph = min(viable, key=lambda p: (p[1], -p[0]))
    print(f"chosen: {cutoff:.2f}, missing {frr:.1%}, {faph:.3f} false wakes an hour")

    work.out.mkdir(exist_ok=True)
    shutil.copy(folder / "stream_state_internal_quant.tflite", work.out / "hey_dj_ed.tflite")
    manifest = {
        "type": "micro",
        "wake_word": "Hey DJ Ed",
        "author": "EdS",
        "website": "https://github.com/cdeever/eds",
        "model": "hey_dj_ed.tflite",
        "trained_languages": ["en"],
        "version": 2,
        "micro": {
            "probability_cutoff": round(cutoff, 2),
            "feature_step_size": 10,
            "sliding_window_size": 5,
            "tensor_arena_size": args.arena,
            "minimum_esphome_version": "2024.7.0",
        },
    }
    (work.out / "hey_dj_ed.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"wrote {work.out / 'hey_dj_ed.tflite'} and hey_dj_ed.json")
    print(f"Kconfig: EA_WAKE_CUTOFF={round(cutoff * 255)} (of 255)")


if __name__ == "__main__":
    main()

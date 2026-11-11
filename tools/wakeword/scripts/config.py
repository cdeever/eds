"""Writes WORK/training_parameters.yaml: what is trained on, and how.

The notebook's settings (2026-11-11), with two of EdS's sets added: the
near misses, weighted like the speech they resemble, and the room's music,
when there is any. Its paths are absolute, so training may run from WORK.
"""

import argparse
import os

import yaml

from common import Work


def feature(path, sampling, truth, truncation, penalty=1.0):
    return {
        "features_dir": str(path),
        "sampling_weight": sampling,
        "penalty_weight": penalty,
        "truth": truth,
        "truncation_strategy": truncation,
        "type": "mmap",
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--work", required=True)
    ap.add_argument("--steps", type=int, default=10000)
    args = ap.parse_args()
    work = Work(args.work)
    neg = work.data / "negative_datasets"

    features = [
        feature(work.features / "positive", 2.0, True, "truncate_start"),
        feature(work.features / "near_misses", 5.0, False, "truncate_start"),
        feature(neg / "speech", 10.0, False, "random"),
        feature(neg / "dinner_party", 10.0, False, "random"),
        feature(neg / "no_speech", 5.0, False, "random"),
        # Only for validation and testing: false wakes an hour at a party.
        feature(neg / "dinner_party_eval", 0.0, False, "split"),
    ]
    if (work.features / "music" / "training").exists():
        features.append(feature(work.features / "music", 5.0, False, "random"))
        features.append(feature(work.features / "music_eval", 0.0, False, "split"))
    for f in features:
        if not os.path.isdir(f["features_dir"]):
            raise SystemExit(f"no {f['features_dir']}: run `make fetch features` first")

    config = {
        "window_step_ms": 10,
        "train_dir": str(work.trained),
        "features": features,
        "training_steps": [args.steps],
        "positive_class_weight": [1],
        "negative_class_weight": [20],
        "learning_rates": [0.001],
        "batch_size": 128,
        "time_mask_max_size": [0],
        "time_mask_count": [0],
        "freq_mask_max_size": [0],
        "freq_mask_count": [0],
        "eval_step_interval": 500,
        # "Hey DJ Ed" said slowly is longer than "Hey Jarvis"; 1.5 s is the
        # notebook's, and enough for four syllables.
        "clip_duration_ms": 1500,
        "target_minimization": 0.9,
        "minimization_metric": None,
        "maximization_metric": "average_viable_recall",
    }
    path = work.root / "training_parameters.yaml"
    path.write_text(yaml.dump(config))
    print(f"wrote {path}")
    for f in features:
        print(f"  {f['sampling_weight']:>5}  {'yes' if f['truth'] else 'no ':3}  {f['features_dir']}")


if __name__ == "__main__":
    main()

"""What the training scripts share: where things are in WORK, and tidying it."""

import os
import re
import shutil
from pathlib import Path


def phrases(path):
    """The phrases in a list file, one a line, without comments or blanks."""
    out = []
    for line in Path(path).read_text().splitlines():
        line = line.split("#", 1)[0].strip()
        if line:
            out.append(line)
    return out


def slug(phrase):
    """A folder name for a phrase. A comma is a pause, and is named, so
    "hey dee jay, ed" and "hey dee jay ed" are not the same clips."""
    return re.sub(r"[^a-z0-9]+", "_", phrase.lower().replace(",", " pause ")).strip("_")


def remove_tree(path):
    """shutil.rmtree, on a disk where a file's ._ goes when the file does.

    On exFAT, deleting "x.wav" takes "._x.wav" with it, and a walk that
    listed both then fails on the second: one already gone is not an error.
    """

    def gone_already(func, target, exc):
        if not isinstance(exc, FileNotFoundError):
            raise exc

    shutil.rmtree(path, onexc=gone_already)


def tidy(root):
    """Removes the ._ files macOS writes beside everything on a foreign disk.

    exFAT keeps a file's extended attributes in a second file, "._name". The
    training finds its audio by pattern, and would read those as clips.
    """
    removed = 0
    for dirpath, _, files in os.walk(root):
        for name in files:
            if name.startswith("._"):
                os.remove(os.path.join(dirpath, name))
                removed += 1
    return removed


class Work:
    def __init__(self, root):
        self.root = Path(root).expanduser().resolve()

    @property
    def piper(self):
        return self.root / "piper-sample-generator"

    @property
    def samples(self):
        return self.root / "samples"

    @property
    def data(self):
        return self.root / "data"

    @property
    def features(self):
        return self.root / "features"

    @property
    def music(self):
        return self.root / "music_16k"

    @property
    def trained(self):
        return self.root / "trained"

    @property
    def out(self):
        return self.root / "out"

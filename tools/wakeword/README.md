# wakeword — "Hey DJ Ed" for the ears

Trains the ears' wake word with microWakeWord, on the Mac
([ADR-0054](../../docs/content/docs/architecture/decisions/0054-the-ears-wake-to-hey-dj-ed-trained-here.md)).
It is microWakeWord's training notebook (`notebooks/basic_training_notebook.ipynb`
at the pinned commit) made into steps that can be run again, with EdS's
near misses and the room's own music added.

Everything it downloads, generates and trains goes to `WORK`, outside the
repository: about 35 GB, most of it microWakeWord's ready-made negatives unpacked. Only the trained model and its manifest come back,
by `make install`, to `firmware/ears/main/models`.

## Steps

```bash
W="/Volumes/Crucial X9/eds-wakeword"

make venv                          # .venv here: Python 3.12, TensorFlow, Torch (~4 GB)
make work WORK="$W"                # the voice generator and its voices
make listen WORK="$W"              # one clip of each spelling: listen before going on
make generate WORK="$W"            # 2000 of each spelling, 300 of each near miss
make fetch WORK="$W"               # rooms, noise, music, ready-made negatives (~7 GB down, ~26 GB unpacked)
make music WORK="$W" MUSIC=~/Music/Music/Media.localized/Music   # optional, recommended
make features WORK="$W"            # mixes the clips and makes their features
make config WORK="$W"
make train WORK="$W"               # hours; stopped, it resumes from its checkpoint
make manifest WORK="$W"            # the cutoff at MAX_FAPH=0.05 false wakes an hour (one a day)
make install WORK="$W"             # the model and manifest to the firmware
```

`make listen` matters: Piper reads "DJ" as letters only sometimes, so
`phrases.txt` spells it the ways people say it. Drop a spelling that sounds
wrong before generating thousands of it.

## What it learns from

| Set | Truth | Where it comes from |
|---|---|---|
| `positive` | yes | `phrases.txt`, said by Piper, mixed with rooms and noise |
| `near_misses` | no | `near-misses.txt`, the same way: "hey DJ", "hey Ed", "ahead" |
| `speech`, `dinner_party`, `no_speech` | no | microWakeWord's ready-made features |
| `music` | no | the songs `make music` was given, cut in three-second pieces |
| `dinner_party_eval`, `music_eval` | no | measured only: false wakes an hour |

The room plays music with words nearly all day, so the music is what most
needs to be shown. Songs that say "DJ" to someone are worth including on
purpose. The music is split by song: the ones measured are never trained on.

## Choosing the cutoff

`make train` ends by testing the quantized streaming model at every cutoff:
how many phrases it missed, and how often it woke falsely an hour on the
ambient sets. `make manifest` keeps the cutoff that misses fewest while
waking at most `MAX_FAPH` times an hour, and prints the Kconfig value.
Synthetic voices make the misses look better than a person's will; the
false wakes on the room's music are the number to believe.

## Not kept

The work folder is never committed: it holds generated clips, downloaded
sets under other people's licenses, the owner's music and, later, the
owner's voice. The trained model is committed, marked for personal use:
the ready-made sets' licenses are mixed, and microWakeWord's author says
what is trained on them is for non-commercial use.

## On an exFAT disk

macOS writes a `._name` file beside each file on an exFAT disk, and the
training finds its audio by pattern. Every script removes them before it
reads (`make tidy` does it by hand). A venv cannot live there, which is why
`.venv` is here.

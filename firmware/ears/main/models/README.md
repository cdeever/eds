# Wake word models

`hey_dj_ed.tflite` and its manifest `hey_dj_ed.json` are EdS's own "Hey DJ
Ed", trained by `tools/wakeword` with microWakeWord on 2026-11-11
([ADR-0054](../../../../docs/content/docs/architecture/decisions/0054-the-ears-wake-to-hey-dj-ed-trained-here.md)).
It is the one the firmware embeds. **It is for personal, non-commercial
use**: it was trained against microWakeWord's ready-made sets of speech,
parties and noise, AudioSet and the Free Music Archive, whose licenses are
mixed, and microWakeWord's author says models trained on them are for
non-commercial use.

`hey_jarvis.tflite` and its manifest `hey_jarvis.json` are ESPHome's
microWakeWord model for "Hey Jarvis", by Kevin Ahrendt, from
<https://github.com/esphome/micro-wake-word-models> (`models/v2`, fetched
2026-10-24), under the Apache License 2.0. The ears woke to it until "Hey
DJ Ed" (CHG-0036, CHG-0049); it is kept to go back to.

A manifest's `micro` settings are what `app/wake` is told in Kconfig:
`probability_cutoff` (0.96 for "Hey DJ Ed", 245 of 255),
`sliding_window_size` (5), `feature_step_size` (10 ms) and
`tensor_arena_size` (22860 bytes, Jarvis's: the boot log says what this one
uses).

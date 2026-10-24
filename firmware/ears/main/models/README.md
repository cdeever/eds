# Wake word models

`hey_jarvis.tflite` and its manifest `hey_jarvis.json` are ESPHome's
microWakeWord model for "Hey Jarvis", by Kevin Ahrendt, from
<https://github.com/esphome/micro-wake-word-models> (`models/v2`, fetched
2026-10-24), under the Apache License 2.0. It stands in until EdS has a
"Hey Ed" of its own.

The manifest's `micro` settings are what `app/wake` is told in Kconfig:
`probability_cutoff` (0.97), `sliding_window_size` (5),
`feature_step_size` (10 ms) and `tensor_arena_size` (22860 bytes).

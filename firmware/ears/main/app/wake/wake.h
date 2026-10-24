// The wake word: microWakeWord's streaming model, on TensorFlow Lite Micro
// (ADR-0043).
//
// Audio becomes features as ESPHome makes them - TensorFlow's micro audio
// frontend, 40 filterbank channels for every 10 ms of 30 - because those are
// what the model was trained on, and a feature computed any other way is a
// model that never wakes or always does. The model hears three features at a
// time; the mean of its last five answers past its cutoff is a detection.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

#ifdef __cplusplus
extern "C" {
#endif

// ea_wake_init loads the model and the frontend. Call once, before feeding.
esp_err_t ea_wake_init(void);

// ea_wake_feed takes 16 kHz mono samples and says whether the wake word
// ended in them. Call from one task only.
bool ea_wake_feed(const int16_t *samples, size_t n);

// ea_wake_reset forgets what was heard: after a request, so that the next
// one starts clean, and a second of features passes before it can wake again.
void ea_wake_reset(void);

// ea_wake_peak is the highest mean probability, of 255, since it was last
// asked: for the console, to see how near the room came.
uint8_t ea_wake_peak(void);

#ifdef __cplusplus
}
#endif

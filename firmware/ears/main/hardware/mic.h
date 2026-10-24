// The microphones, as the XMOS chip gives them: one cleaned-up voice over
// I2S (ADR-0043).
//
// ea_mic_start listens for as long as the device runs. It measures the rate
// the XMOS chip sends and prints it, with the level of both channels, every
// couple of seconds - which is how the pins, the rate and the voice's
// channel are found the first time. While the talk button is held it keeps
// what is said, as 16 kHz mono, and on release hands it over as a WAV.
#pragma once

#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

// A request, ready to send: a WAV, valid until the callback returns.
typedef void (*ea_request_fn)(const uint8_t *wav, size_t len);

esp_err_t ea_mic_start(ea_request_fn on_request);

// What was last measured: the rate in Hz (0 when nothing arrives), and each
// channel's level in dB below full scale.
uint32_t ea_mic_rate(void);
float ea_mic_level(int channel);

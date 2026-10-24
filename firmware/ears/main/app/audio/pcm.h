// What is done to the sound between the XMOS chip and the broker.
//
// Pure C with no ESP-IDF dependency, like the screen's ui.c, and for the same
// reason: a request that is silent, too fast or one channel's echo is not an
// error anyone sees. scribe writes down nothing, or the wrong words, and the
// room is told "not understood". That is worth testing with a compiler.
#pragma once

#include <stddef.h>
#include <stdint.h>

// What scribe takes (ADR-0042): 16 kHz, 16-bit, mono.
#define EA_PCM_RATE 16000

// The I2S data is 32-bit stereo: a frame is a left sample and a right one,
// each with the sound in its high bits.
#define EA_PCM_CHANNELS 2

// ea_pcm_level is a channel's loudness over n frames, in dB below full scale:
// 0 is as loud as can be, -90 is a channel with nothing on it.
float ea_pcm_level(const int32_t *frames, size_t n_frames, int channel);

// ea_pcm_rate is a measured rate rounded to the nearest rate an I2S source
// is likely to send (8, 16, 22.05, 24, 32, 44.1 or 48 kHz), or 0 when the
// measurement is nothing like any of them.
uint32_t ea_pcm_rate(uint32_t frames, uint32_t elapsed_us);

// ea_pcm_take makes 16 kHz mono 16-bit from n frames of one channel at rate,
// writing at most cap samples to out and returning how many it wrote. rate
// must be a whole multiple of 16 kHz; the decimation averages each run of
// samples, which is crude, and enough for speech written down by a model
// trained on worse. *carry holds a run left part-way at the end of one call
// for the next, and *carried how much of it; both start at 0.
size_t ea_pcm_take(const int32_t *frames, size_t n_frames, int channel, uint32_t rate,
                   int16_t *out, size_t cap, int64_t *carry, uint32_t *carried);

// ea_wav_header writes the 44 bytes before n_samples of 16 kHz mono 16-bit.
void ea_wav_header(uint8_t header[44], uint32_t n_samples);

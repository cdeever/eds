#include "pcm.h"

#include <math.h>
#include <string.h>

// The sound is in the high 24 bits of each 32: the low byte is the XMOS
// chip's padding.
static int32_t sample(const int32_t *frames, size_t frame, int channel)
{
    return frames[frame * EA_PCM_CHANNELS + (size_t)channel] >> 8;
}

float ea_pcm_level(const int32_t *frames, size_t n_frames, int channel)
{
    if (frames == NULL || n_frames == 0 || channel < 0 || channel >= EA_PCM_CHANNELS) {
        return -90.0f;
    }
    double sum = 0;
    for (size_t i = 0; i < n_frames; i++) {
        double s = (double)sample(frames, i, channel) / 8388608.0; // 2^23
        sum += s * s;
    }
    double rms = sqrt(sum / (double)n_frames);
    if (rms < 3.1623e-5) { // -90 dB
        return -90.0f;
    }
    return (float)(20.0 * log10(rms));
}

uint32_t ea_pcm_rate(uint32_t frames, uint32_t elapsed_us)
{
    static const uint32_t rates[] = {8000, 16000, 22050, 24000, 32000, 44100, 48000};
    if (elapsed_us == 0 || frames == 0) {
        return 0;
    }
    double measured = (double)frames * 1e6 / (double)elapsed_us;
    for (size_t i = 0; i < sizeof(rates) / sizeof(rates[0]); i++) {
        // Within 3%: a clock measured over a second against the ESP32's own
        // is far closer than that, and the rates are further apart.
        if (fabs(measured - (double)rates[i]) <= 0.03 * (double)rates[i]) {
            return rates[i];
        }
    }
    return 0;
}

size_t ea_pcm_take(const int32_t *frames, size_t n_frames, int channel, uint32_t rate,
                   int16_t *out, size_t cap, int64_t *carry, uint32_t *carried)
{
    if (frames == NULL || out == NULL || carry == NULL || carried == NULL ||
        channel < 0 || channel >= EA_PCM_CHANNELS || rate < EA_PCM_RATE || rate % EA_PCM_RATE != 0) {
        return 0;
    }
    uint32_t every = rate / EA_PCM_RATE;
    size_t written = 0;
    for (size_t i = 0; i < n_frames && written < cap; i++) {
        *carry += sample(frames, i, channel);
        if (++*carried < every) {
            continue;
        }
        // 24 bits to 16: the top sixteen, rounded rather than cut.
        int64_t mean = *carry / (int64_t)every;
        int64_t s16 = (mean + (mean >= 0 ? 128 : -128)) / 256;
        if (s16 > 32767) {
            s16 = 32767;
        } else if (s16 < -32768) {
            s16 = -32768;
        }
        out[written++] = (int16_t)s16;
        *carry = 0;
        *carried = 0;
    }
    return written;
}

static void put32(uint8_t *p, uint32_t v)
{
    p[0] = (uint8_t)v;
    p[1] = (uint8_t)(v >> 8);
    p[2] = (uint8_t)(v >> 16);
    p[3] = (uint8_t)(v >> 24);
}

static void put16(uint8_t *p, uint16_t v)
{
    p[0] = (uint8_t)v;
    p[1] = (uint8_t)(v >> 8);
}

void ea_wav_header(uint8_t header[44], uint32_t n_samples)
{
    uint32_t data = n_samples * 2;
    memcpy(header, "RIFF", 4);
    put32(header + 4, 36 + data);
    memcpy(header + 8, "WAVEfmt ", 8);
    put32(header + 16, 16);          // the fmt chunk's size
    put16(header + 20, 1);           // PCM
    put16(header + 22, 1);           // mono
    put32(header + 24, EA_PCM_RATE); // samples a second
    put32(header + 28, EA_PCM_RATE * 2);
    put16(header + 32, 2);           // bytes a sample
    put16(header + 34, 16);          // bits a sample
    memcpy(header + 36, "data", 4);
    put32(header + 40, data);
}

#include "effects.h"

#include <math.h>
#include <string.h>

// Animation periods, in milliseconds, at speed 0 and speed 1. Speed is a 0..1
// dial rather than a frequency so the publisher never has to know what a
// sensible rate looks like on this hardware.
#define PERIOD_SLOW_MS 6000.0f
#define PERIOD_FAST_MS 1000.0f

// A breathing stand that reaches full darkness reads as broken rather than
// slow, so the trough sits above zero.
#define BREATHE_FLOOR 0.25f

// Fraction of the strip over which adjacent palette bands cross-fade. Hard
// edges on a diffused strip look like a fault in the diffuser.
#define SWEEP_BLEND 0.06f

// M_PI is a POSIX extension, not ISO C, and is absent under a strict -std=c11.
// Defining it here keeps the module free of feature-test-macro guesswork.
#define LP_TWO_PI 6.28318530717958647692f

static float period_ms(float speed)
{
    return PERIOD_SLOW_MS + (PERIOD_FAST_MS - PERIOD_SLOW_MS) * speed;
}

// phase01 returns where in the animation cycle t_ms falls, as 0..1.
static float phase01(uint32_t t_ms, float speed)
{
    float period = period_ms(speed);
    float phase = fmodf((float)t_ms, period) / period;
    return phase < 0.0f ? phase + 1.0f : phase;
}

static uint8_t scale8(uint8_t value, float factor)
{
    float scaled = (float)value * factor;
    if (scaled <= 0.0f) {
        return 0;
    }
    if (scaled >= 255.0f) {
        return 255;
    }
    return (uint8_t)(scaled + 0.5f);
}

static lp_rgb_t dim(lp_rgb_t color, float factor)
{
    return (lp_rgb_t){
        .r = scale8(color.r, factor),
        .g = scale8(color.g, factor),
        .b = scale8(color.b, factor),
    };
}

static lp_rgb_t mix(lp_rgb_t a, lp_rgb_t b, float t)
{
    return (lp_rgb_t){
        .r = (uint8_t)((float)a.r + ((float)b.r - (float)a.r) * t + 0.5f),
        .g = (uint8_t)((float)a.g + ((float)b.g - (float)a.g) * t + 0.5f),
        .b = (uint8_t)((float)a.b + ((float)b.b - (float)a.b) * t + 0.5f),
    };
}

static void fill(lp_rgb_t *out, size_t count, lp_rgb_t color)
{
    for (size_t i = 0; i < count; i++) {
        out[i] = color;
    }
}

// sweep_color samples the palette at position u (0..1 around the strip),
// laying colours out in proportion to their weights so a colour covering 60%
// of the sleeve covers 60% of the strip, and cross-fading at the seams.
static lp_rgb_t sweep_color(const lp_scene_t *scene, float u)
{
    u -= floorf(u); // wrap into 0..1

    float start = 0.0f;
    for (size_t i = 0; i < scene->palette_len; i++) {
        float width = scene->palette[i].weight;
        float end = start + width;

        if (u < end || i + 1 == scene->palette_len) {
            const lp_rgb_t here = scene->palette[i].rgb;
            const lp_rgb_t next = scene->palette[(i + 1) % scene->palette_len].rgb;

            // Blend only over the tail of the band, and never over more than
            // half of a narrow one, so a thin band is not swallowed entirely.
            float blend = SWEEP_BLEND;
            if (blend > width * 0.5f) {
                blend = width * 0.5f;
            }
            if (blend <= 0.0f) {
                return here;
            }

            float into_tail = u - (end - blend);
            if (into_tail <= 0.0f) {
                return here;
            }
            return mix(here, next, into_tail / blend);
        }
        start = end;
    }
    return scene->palette[0].rgb;
}

void lp_effects_render(const lp_scene_t *scene, uint32_t t_ms, lp_rgb_t *out, size_t count)
{
    if (out == NULL || count == 0) {
        return;
    }
    if (scene == NULL || scene->palette_len == 0) {
        memset(out, 0, count * sizeof(*out));
        return;
    }

    const lp_rgb_t dominant = scene->palette[0].rgb;
    const float brightness = scene->brightness;

    switch (scene->effect) {
    case LP_EFFECT_BREATHE: {
        float wave = 0.5f + 0.5f * sinf(LP_TWO_PI * phase01(t_ms, scene->speed));
        float level = BREATHE_FLOOR + (1.0f - BREATHE_FLOOR) * wave;
        fill(out, count, dim(dominant, brightness * level));
        break;
    }

    case LP_EFFECT_SWEEP: {
        float offset = phase01(t_ms, scene->speed);
        for (size_t i = 0; i < count; i++) {
            float u = (float)i / (float)count + offset;
            out[i] = dim(sweep_color(scene, u), brightness);
        }
        break;
    }

    case LP_EFFECT_SOLID:
    default:
        fill(out, count, dim(dominant, brightness));
        break;
    }
}

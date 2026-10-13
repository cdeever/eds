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

// Morph gives each color a turn at the whole strip. How long a turn lasts is
// half by the color's share of the cover and half equal, so the dominant
// color is on longest and a 4% accent still gets to be seen. The last part of
// each turn is spent turning into the next color.
#define MORPH_BLEND 0.45f

// Chase: how much of the strip an accent covers is its share of the cover,
// plus enough that the smallest is a couple of LEDs wide on a strip of
// eighteen, and never so much that the background stops being one. Narrower
// than about two LEDs, a soft-edged patch does not glide between them: it
// dims to nearly nothing halfway and reads as a twinkle.
#define CHASE_MIN_WIDTH 0.12f
#define CHASE_MAX_WIDTH 0.30f

// Scanner: one point of light going from end to end and back. The eye is
// this much of the strip to either side of its center, soft-edged, which on
// eighteen LEDs is three or four lit at once: narrower, and it steps from
// LED to LED and does not glide. Behind it, where it has just been, fades
// over a few more. The fade is drawn as the eye's own earlier positions,
// each dimmer than the one after it: so many of them, this much of the cycle
// apart.
#define SCANNER_EYE 0.10f
#define SCANNER_TRAIL 8
#define SCANNER_TRAIL_STEP 0.02f

// M_PI is a POSIX extension, not ISO C, and is absent under a strict -std=c11.
// Defining it here keeps the module free of feature-test-macro guesswork.
#define LP_TWO_PI 6.28318530717958647692f

static float period_ms(float speed)
{
    return PERIOD_SLOW_MS + (PERIOD_FAST_MS - PERIOD_SLOW_MS) * speed;
}

// phase_in returns where in a cycle of `period` milliseconds t_ms falls, as
// 0..1.
//
// The remainder is taken on the integer. A float holds a millisecond count
// exactly only up to about four and a half hours; past that, taking the
// remainder of (float)t_ms moves in steps that grow with uptime, and after a
// week an animation advances in jumps of tens of milliseconds.
static float phase_in(uint32_t t_ms, float period)
{
    uint32_t whole = (uint32_t)(period + 0.5f);
    if (whole == 0) {
        return 0.0f;
    }
    return (float)(t_ms % whole) / (float)whole;
}

// phase01 returns where in the animation cycle t_ms falls, as 0..1.
static float phase01(uint32_t t_ms, float speed)
{
    return phase_in(t_ms, period_ms(speed));
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
// laying colors out in proportion to their weights so a color covering 60%
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

// smooth eases a 0..1 ramp in and out, so a change of color has no moment
// at which it visibly starts or stops.
static float smooth(float t)
{
    if (t <= 0.0f) {
        return 0.0f;
    }
    if (t >= 1.0f) {
        return 1.0f;
    }
    return t * t * (3.0f - 2.0f * t);
}

// morph_color is the one color the whole strip shows at `phase` (0..1 through
// every color and back to the first).
static lp_rgb_t morph_color(const lp_scene_t *scene, float phase)
{
    const size_t n = scene->palette_len;
    if (n < 2) {
        return scene->palette[0].rgb;
    }

    float start = 0.0f;
    for (size_t i = 0; i < n; i++) {
        float turn = 0.5f / (float)n + 0.5f * scene->palette[i].weight;
        if (phase < start + turn || i + 1 == n) {
            float into = (phase - start) / turn;
            float hold = 1.0f - MORPH_BLEND;
            if (into <= hold) {
                return scene->palette[i].rgb;
            }
            return mix(scene->palette[i].rgb, scene->palette[(i + 1) % n].rgb,
                       smooth((into - hold) / MORPH_BLEND));
        }
        start += turn;
    }
    return scene->palette[0].rgb;
}

// chase_color is what position u (0..1 around the strip) shows when the lap
// is at `phase`: the dominant color, with each accent a soft-edged patch
// traveling over it. Alternate accents go opposite ways, so they pass
// through one another instead of marching in file.
static lp_rgb_t chase_color(const lp_scene_t *scene, float u, float phase)
{
    lp_rgb_t color = scene->palette[0].rgb;
    const size_t accents = scene->palette_len - 1;

    for (size_t k = 0; k < accents; k++) {
        const lp_swatch_t *accent = &scene->palette[k + 1];

        float width = CHASE_MIN_WIDTH + accent->weight;
        if (width > CHASE_MAX_WIDTH) {
            width = CHASE_MAX_WIDTH;
        }
        float half = width * 0.5f;

        float center = (float)k / (float)accents + ((k % 2 == 0) ? phase : -phase);
        center -= floorf(center);

        // The short way around: the strip is a loop.
        float distance = fabsf(u - center);
        if (distance > 0.5f) {
            distance = 1.0f - distance;
        }
        if (distance < half) {
            float amount = 0.5f + 0.5f * cosf(LP_TWO_PI * 0.5f * distance / half);
            color = mix(color, accent->rgb, amount);
        }
    }
    return color;
}

// scanner_level is how bright the pixel centered at u (0..1 along the strip)
// is when the scanner's cycle is at `phase`: 1 under the eye, less in what
// trails it, 0 everywhere else.
static float scanner_level(float u, float phase)
{
    float level = 0.0f;
    for (int k = 0; k < SCANNER_TRAIL; k++) {
        float then = phase - (float)k * SCANNER_TRAIL_STEP;
        if (then < 0.0f) {
            then += 1.0f;
        }
        // Out for the first half of the cycle and back for the second.
        float eye = then < 0.5f ? 2.0f * then : 2.0f - 2.0f * then;
        float distance = fabsf(u - eye);
        if (distance >= SCANNER_EYE) {
            continue;
        }
        float fade = 1.0f - (float)k / (float)SCANNER_TRAIL;
        float here = (0.5f + 0.5f * cosf(LP_TWO_PI * 0.5f * distance / SCANNER_EYE)) * fade * fade;
        if (here > level) {
            level = here;
        }
    }
    return level;
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

    case LP_EFFECT_MORPH: {
        // A turn each, of about two of the scene's cycles: slow enough that
        // the room changes color, not flickers.
        float loop = period_ms(scene->speed) * 2.0f * (float)scene->palette_len;
        fill(out, count, dim(morph_color(scene, phase_in(t_ms, loop)), brightness));
        break;
    }

    case LP_EFFECT_CHASE: {
        // With nothing but a dominant color there is nothing to chase.
        if (scene->palette_len < 2) {
            fill(out, count, dim(dominant, brightness));
            break;
        }
        float phase = phase_in(t_ms, period_ms(scene->speed) * 2.0f);
        for (size_t i = 0; i < count; i++) {
            float u = (float)i / (float)count;
            out[i] = dim(chase_color(scene, u, phase), brightness);
        }
        break;
    }

    case LP_EFFECT_SCANNER: {
        // There and back in one of the scene's cycles.
        float phase = phase01(t_ms, scene->speed);
        for (size_t i = 0; i < count; i++) {
            float u = ((float)i + 0.5f) / (float)count;
            out[i] = dim(dominant, brightness * scanner_level(u, phase));
        }
        break;
    }

    case LP_EFFECT_SOLID:
    default:
        fill(out, count, dim(dominant, brightness));
        break;
    }
}

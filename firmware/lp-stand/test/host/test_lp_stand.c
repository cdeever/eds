// Host tests for the hardware-independent half of the firmware.
//
// Scene validation, effect rendering and gamma are pure C with no ESP-IDF
// dependency, which means the parts where a bug is subtle and only visible as
// "the light looks wrong" can be tested with a compiler and no hardware.
//
// The ESP-IDF glue - WiFi, MQTT, the RMT driver - is not covered here and
// needs a device.

#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "effects.h"
#include "gamma.h"
#include "scene.h"

static int failures = 0;
static int checks = 0;

#define CHECK(cond, ...)                                                       \
    do {                                                                       \
        checks++;                                                              \
        if (!(cond)) {                                                         \
            failures++;                                                        \
            printf("  FAIL %s:%d: ", __func__, __LINE__);                      \
            printf(__VA_ARGS__);                                               \
            printf("\n");                                                      \
        }                                                                      \
    } while (0)

#define STRIP 60
static lp_rgb_t pixels[STRIP];

static lp_scene_t two_colour(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.effect = LP_EFFECT_SWEEP;
    s.speed = 0.5f;
    s.brightness = 1.0f;
    s.palette[0] = (lp_swatch_t){.rgb = {232, 77, 0}, .weight = 0.75f};
    s.palette[1] = (lp_swatch_t){.rgb = {0, 86, 160}, .weight = 0.25f};
    s.palette_len = 2;
    return s;
}

// --- gamma -----------------------------------------------------------------

static void test_gamma_endpoints_and_monotonicity(void)
{
    CHECK(lp_gamma8(0) == 0, "gamma(0) = %d", lp_gamma8(0));
    CHECK(lp_gamma8(255) == 255, "gamma(255) = %d", lp_gamma8(255));

    for (int i = 1; i < 256; i++) {
        CHECK(lp_gamma8((uint8_t)i) >= lp_gamma8((uint8_t)(i - 1)),
              "gamma is not monotonic at %d", i);
    }
}

static void test_gamma_darkens_the_midrange(void)
{
    // The whole point: a linear ramp looks top-heavy, so mid levels must map
    // below themselves.
    CHECK(lp_gamma8(128) < 128, "gamma(128) = %d, expected well below 128", lp_gamma8(128));
    CHECK(lp_gamma8(64) < 32, "gamma(64) = %d, expected deep in the dark end", lp_gamma8(64));
}

// --- effect names ----------------------------------------------------------

static void test_effect_names_round_trip(void)
{
    for (int i = 0; i < LP_EFFECT_COUNT; i++) {
        const char *name = lp_effect_name((lp_effect_t)i);
        CHECK(lp_effect_from_name(name) == (lp_effect_t)i, "%s did not round trip", name);
    }
}

// Effects can only be added by reflashing, so a publisher that learns a new one
// before the stands do is expected. A lit stand with the wrong animation beats
// a dark one.
static void test_unknown_effect_falls_back_to_solid(void)
{
    CHECK(lp_effect_from_name("shimmer") == LP_EFFECT_SOLID, "unknown effect did not fall back");
    CHECK(lp_effect_from_name(NULL) == LP_EFFECT_SOLID, "NULL effect did not fall back");
    CHECK(lp_effect_from_name("") == LP_EFFECT_SOLID, "empty effect did not fall back");
}

// --- scene validation ------------------------------------------------------

static void test_validate_normalises_weights(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.brightness = 0.5f;
    s.palette[0] = (lp_swatch_t){.rgb = {255, 0, 0}, .weight = 3.0f};
    s.palette[1] = (lp_swatch_t){.rgb = {0, 0, 255}, .weight = 1.0f};
    s.palette_len = 2;

    CHECK(lp_scene_validate(&s), "valid scene was rejected");
    CHECK(fabsf(s.palette[0].weight - 0.75f) < 1e-5f, "weight[0] = %f", s.palette[0].weight);
    CHECK(fabsf(s.palette[1].weight - 0.25f) < 1e-5f, "weight[1] = %f", s.palette[1].weight);
}

static void test_validate_clamps_out_of_range_values(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.brightness = 9.0f;
    s.speed = -4.0f;
    s.palette[0] = (lp_swatch_t){.rgb = {1, 2, 3}, .weight = 1.0f};
    s.palette_len = 1;

    CHECK(lp_scene_validate(&s), "scene rejected");
    CHECK(s.brightness == 1.0f, "brightness = %f", s.brightness);
    CHECK(s.speed == 0.0f, "speed = %f", s.speed);
}

static void test_validate_drops_weightless_entries(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.brightness = 1.0f;
    s.palette[0] = (lp_swatch_t){.rgb = {255, 0, 0}, .weight = 0.0f};
    s.palette[1] = (lp_swatch_t){.rgb = {0, 255, 0}, .weight = 1.0f};
    s.palette_len = 2;

    CHECK(lp_scene_validate(&s), "scene rejected");
    CHECK(s.palette_len == 1, "palette_len = %zu, want 1", s.palette_len);
    CHECK(s.palette[0].rgb.g == 255, "the wrong colour survived");
}

// This data arrives over the network. A stand that crashes on a malformed
// payload needs physical access to recover.
static void test_validate_rejects_unrenderable_scenes(void)
{
    lp_scene_t empty;
    memset(&empty, 0, sizeof(empty));
    CHECK(!lp_scene_validate(&empty), "empty palette was accepted");

    lp_scene_t zeroed;
    memset(&zeroed, 0, sizeof(zeroed));
    zeroed.palette[0] = (lp_swatch_t){.rgb = {1, 1, 1}, .weight = 0.0f};
    zeroed.palette_len = 1;
    CHECK(!lp_scene_validate(&zeroed), "all-zero weights were accepted");

    CHECK(!lp_scene_validate(NULL), "NULL scene was accepted");
}

static void test_validate_rejects_nan(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.brightness = NAN;
    s.speed = NAN;
    s.palette[0] = (lp_swatch_t){.rgb = {255, 0, 0}, .weight = NAN};
    s.palette_len = 1;

    CHECK(!lp_scene_validate(&s), "a NaN palette weight was accepted");
    CHECK(s.brightness == 0.0f, "NaN brightness became %f", s.brightness);
}

static void test_validate_caps_palette_length(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.brightness = 1.0f;
    for (int i = 0; i < LP_MAX_COLORS; i++) {
        s.palette[i] = (lp_swatch_t){.rgb = {(uint8_t)i, 0, 0}, .weight = 1.0f};
    }
    s.palette_len = LP_MAX_COLORS + 99; // a lying length

    CHECK(lp_scene_validate(&s), "scene rejected");
    CHECK(s.palette_len == LP_MAX_COLORS, "palette_len = %zu", s.palette_len);
}

static void test_default_scene_is_lit(void)
{
    lp_scene_t s;
    lp_scene_default(&s);

    CHECK(lp_scene_validate(&s), "the default scene is not renderable");
    lp_effects_render(&s, 0, pixels, STRIP);
    // An unconfigured stand should look deliberate, not broken or dead.
    CHECK(pixels[0].r > 0, "the default scene renders black");
    CHECK(pixels[0].r < 255, "the default scene renders at full brightness");
}

// --- effects ---------------------------------------------------------------

static void test_solid_is_uniform_and_scaled(void)
{
    lp_scene_t s = two_colour();
    s.effect = LP_EFFECT_SOLID;
    s.brightness = 0.5f;

    lp_effects_render(&s, 12345, pixels, STRIP);
    for (int i = 1; i < STRIP; i++) {
        CHECK(memcmp(&pixels[i], &pixels[0], sizeof(lp_rgb_t)) == 0,
              "solid is not uniform at pixel %d", i);
    }
    CHECK(pixels[0].r == (uint8_t)(232 * 0.5f + 0.5f), "red = %d", pixels[0].r);
}

static void test_zero_brightness_is_black(void)
{
    lp_scene_t s = two_colour();
    s.brightness = 0.0f;

    for (int effect = 0; effect < LP_EFFECT_COUNT; effect++) {
        s.effect = (lp_effect_t)effect;
        lp_effects_render(&s, 999, pixels, STRIP);
        for (int i = 0; i < STRIP; i++) {
            CHECK(pixels[i].r == 0 && pixels[i].g == 0 && pixels[i].b == 0,
                  "%s pixel %d is lit at zero brightness", lp_effect_name(s.effect), i);
        }
    }
}

// A breathing stand that reaches full darkness reads as broken rather than
// slow.
static void test_breathe_never_goes_fully_dark(void)
{
    lp_scene_t s = two_colour();
    s.effect = LP_EFFECT_BREATHE;
    s.brightness = 1.0f;

    int lit_min = 255;
    for (uint32_t t = 0; t < 8000; t += 25) {
        lp_effects_render(&s, t, pixels, STRIP);
        if (pixels[0].r < lit_min) {
            lit_min = pixels[0].r;
        }
    }
    CHECK(lit_min > 20, "breathe dropped to %d, effectively dark", lit_min);
}

static void test_breathe_actually_varies(void)
{
    lp_scene_t s = two_colour();
    s.effect = LP_EFFECT_BREATHE;
    s.brightness = 1.0f;

    int lo = 255, hi = 0;
    for (uint32_t t = 0; t < 8000; t += 25) {
        lp_effects_render(&s, t, pixels, STRIP);
        if (pixels[0].r < lo) lo = pixels[0].r;
        if (pixels[0].r > hi) hi = pixels[0].r;
    }
    CHECK(hi - lo > 60, "breathe range is only %d, barely visible", hi - lo);
}

static void test_breathe_is_uniform_across_the_strip(void)
{
    lp_scene_t s = two_colour();
    s.effect = LP_EFFECT_BREATHE;

    lp_effects_render(&s, 1234, pixels, STRIP);
    for (int i = 1; i < STRIP; i++) {
        CHECK(memcmp(&pixels[i], &pixels[0], sizeof(lp_rgb_t)) == 0,
              "breathe is not uniform at pixel %d", i);
    }
}

// The point of carrying weights all the way to the device: a colour covering
// 75% of the sleeve should cover about 75% of the strip.
static void test_sweep_honours_palette_weights(void)
{
    lp_scene_t s = two_colour();
    s.effect = LP_EFFECT_SWEEP;
    s.speed = 0.0f;
    s.brightness = 1.0f;

    lp_effects_render(&s, 0, pixels, STRIP);

    int reddish = 0;
    for (int i = 0; i < STRIP; i++) {
        if (pixels[i].r > pixels[i].b) {
            reddish++;
        }
    }
    float share = (float)reddish / (float)STRIP;
    CHECK(share > 0.6f && share < 0.9f, "the 0.75-weight colour covers %.2f of the strip", share);
}

static void test_sweep_scrolls(void)
{
    lp_scene_t s = two_colour();
    s.effect = LP_EFFECT_SWEEP;
    s.speed = 1.0f;

    lp_rgb_t first[STRIP];
    lp_effects_render(&s, 0, first, STRIP);
    lp_effects_render(&s, 300, pixels, STRIP);

    CHECK(memcmp(first, pixels, sizeof(first)) != 0, "sweep did not move over 300ms");
}

static void test_sweep_is_continuous_around_the_seam(void)
{
    lp_scene_t s = two_colour();
    s.effect = LP_EFFECT_SWEEP;
    s.speed = 0.0f;
    s.brightness = 1.0f;

    lp_effects_render(&s, 0, pixels, STRIP);

    // Hard edges on a diffused strip look like a fault in the diffuser, so no
    // single step should jump the full distance between the two colours.
    int worst = 0;
    for (int i = 0; i < STRIP; i++) {
        const lp_rgb_t a = pixels[i];
        const lp_rgb_t b = pixels[(i + 1) % STRIP];
        int step = abs((int)a.r - (int)b.r) + abs((int)a.g - (int)b.g) + abs((int)a.b - (int)b.b);
        if (step > worst) {
            worst = step;
        }
    }
    CHECK(worst < 400, "largest neighbour step is %d - the seam is a hard edge", worst);
}

static void test_single_colour_sweep_is_uniform(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.effect = LP_EFFECT_SWEEP;
    s.brightness = 1.0f;
    s.palette[0] = (lp_swatch_t){.rgb = {10, 200, 90}, .weight = 1.0f};
    s.palette_len = 1;

    lp_effects_render(&s, 4321, pixels, STRIP);
    for (int i = 0; i < STRIP; i++) {
        CHECK(pixels[i].g == 200, "single-colour sweep varies at pixel %d (g=%d)", i, pixels[i].g);
    }
}

static void test_render_is_deterministic(void)
{
    lp_scene_t s = two_colour();
    lp_rgb_t a[STRIP], b[STRIP];

    lp_effects_render(&s, 7777, a, STRIP);
    lp_effects_render(&s, 7777, b, STRIP);
    CHECK(memcmp(a, b, sizeof(a)) == 0, "same scene and time produced different pixels");
}

// t_ms is free-running; a wrap at 2^32 should cost one frame, not a hang.
static void test_render_survives_timestamp_wrap(void)
{
    lp_scene_t s = two_colour();
    lp_effects_render(&s, 0xFFFFFFFFu, pixels, STRIP);
    CHECK(1, "rendered at the wrap point");
}

static void test_render_is_defensive(void)
{
    lp_scene_t s = two_colour();
    lp_effects_render(NULL, 0, pixels, STRIP); // must not crash
    lp_effects_render(&s, 0, NULL, STRIP);     // must not crash
    lp_effects_render(&s, 0, pixels, 0);       // must not crash

    lp_scene_t empty;
    memset(&empty, 0, sizeof(empty));
    memset(pixels, 0x7F, sizeof(pixels));
    lp_effects_render(&empty, 0, pixels, STRIP);
    CHECK(pixels[0].r == 0, "an empty scene left stale pixels lit");
}

int main(void)
{
    struct {
        const char *name;
        void (*fn)(void);
    } tests[] = {
        {"gamma endpoints and monotonicity", test_gamma_endpoints_and_monotonicity},
        {"gamma darkens the midrange", test_gamma_darkens_the_midrange},
        {"effect names round trip", test_effect_names_round_trip},
        {"unknown effect falls back to solid", test_unknown_effect_falls_back_to_solid},
        {"validate normalises weights", test_validate_normalises_weights},
        {"validate clamps out-of-range values", test_validate_clamps_out_of_range_values},
        {"validate drops weightless entries", test_validate_drops_weightless_entries},
        {"validate rejects unrenderable scenes", test_validate_rejects_unrenderable_scenes},
        {"validate rejects NaN", test_validate_rejects_nan},
        {"validate caps palette length", test_validate_caps_palette_length},
        {"default scene is lit", test_default_scene_is_lit},
        {"solid is uniform and scaled", test_solid_is_uniform_and_scaled},
        {"zero brightness is black", test_zero_brightness_is_black},
        {"breathe never goes fully dark", test_breathe_never_goes_fully_dark},
        {"breathe actually varies", test_breathe_actually_varies},
        {"breathe is uniform across the strip", test_breathe_is_uniform_across_the_strip},
        {"sweep honours palette weights", test_sweep_honours_palette_weights},
        {"sweep scrolls", test_sweep_scrolls},
        {"sweep is continuous around the seam", test_sweep_is_continuous_around_the_seam},
        {"single-colour sweep is uniform", test_single_colour_sweep_is_uniform},
        {"render is deterministic", test_render_is_deterministic},
        {"render survives timestamp wrap", test_render_survives_timestamp_wrap},
        {"render is defensive", test_render_is_defensive},
    };

    for (size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int before = failures;
        tests[i].fn();
        printf("%s %s\n", failures == before ? "ok  " : "FAIL", tests[i].name);
    }

    printf("\n%d checks, %d failures\n", checks, failures);
    return failures == 0 ? 0 : 1;
}

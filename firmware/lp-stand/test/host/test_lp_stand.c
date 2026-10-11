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
#include "event_format.h"
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

// --- the effects that use the accents -----------------------------------------

// A cover with a dominant colour and two accents, as lightd sends one.
static lp_scene_t accented(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.speed = 0.5f; // a cycle of 3500 ms
    s.brightness = 1.0f;
    s.palette[0] = (lp_swatch_t){.rgb = {30, 40, 200}, .weight = 0.80f};
    s.palette[1] = (lp_swatch_t){.rgb = {230, 30, 20}, .weight = 0.15f};
    s.palette[2] = (lp_swatch_t){.rgb = {240, 220, 90}, .weight = 0.05f};
    s.palette_len = 3;
    return s;
}

static int far_from(lp_rgb_t a, lp_rgb_t b)
{
    int dr = abs((int)a.r - (int)b.r), dg = abs((int)a.g - (int)b.g), db = abs((int)a.b - (int)b.b);
    int m = dr > dg ? dr : dg;
    return m > db ? m : db;
}

static void test_new_effect_names_round_trip(void)
{
    CHECK(lp_effect_from_name("morph") == LP_EFFECT_MORPH, "morph");
    CHECK(lp_effect_from_name("chase") == LP_EFFECT_CHASE, "chase");
    CHECK(strcmp(lp_effect_name(LP_EFFECT_MORPH), "morph") == 0, "name of morph");
    CHECK(strcmp(lp_effect_name(LP_EFFECT_CHASE), "chase") == 0, "name of chase");
}

// Morph is one colour for the whole strip at any moment.
static void test_morph_is_uniform_across_the_strip(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_MORPH;
    for (uint32_t t = 0; t < 21000; t += 700) {
        lp_effects_render(&s, t, pixels, STRIP);
        for (int i = 1; i < STRIP; i++) {
            CHECK(memcmp(&pixels[i], &pixels[0], sizeof(lp_rgb_t)) == 0,
                  "morph is not uniform at pixel %d, t=%u", i, (unsigned)t);
        }
    }
}

// It starts on the dominant colour, reaches every accent, and comes back.
static void test_morph_visits_every_colour_and_returns(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_MORPH;
    const uint32_t loop = 3500 * 2 * 3;

    lp_effects_render(&s, 0, pixels, STRIP);
    CHECK(far_from(pixels[0], s.palette[0].rgb) == 0, "morph does not start on the dominant colour");

    int nearest[3] = {255, 255, 255};
    for (uint32_t t = 0; t < loop; t += 25) {
        lp_effects_render(&s, t, pixels, STRIP);
        for (int c = 0; c < 3; c++) {
            int d = far_from(pixels[0], s.palette[c].rgb);
            if (d < nearest[c]) {
                nearest[c] = d;
            }
        }
    }
    for (int c = 0; c < 3; c++) {
        CHECK(nearest[c] == 0, "morph never shows colour %d outright (closest %d)", c, nearest[c]);
    }

    lp_rgb_t first = pixels[0];
    lp_effects_render(&s, 0, pixels, STRIP);
    first = pixels[0];
    lp_effects_render(&s, loop, pixels, STRIP);
    CHECK(far_from(pixels[0], first) == 0, "morph does not come round after one loop");
}

// The point of weights: the dominant colour is on longest, and a small accent
// is still on long enough to be seen.
static void test_morph_gives_the_dominant_colour_the_longest_turn(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_MORPH;
    const uint32_t loop = 3500 * 2 * 3;

    int shown[3] = {0, 0, 0};
    for (uint32_t t = 0; t < loop; t += 20) {
        lp_effects_render(&s, t, pixels, STRIP);
        for (int c = 0; c < 3; c++) {
            if (far_from(pixels[0], s.palette[c].rgb) == 0) {
                shown[c]++;
            }
        }
    }
    CHECK(shown[0] > shown[1] && shown[1] > shown[2], "turns are %d, %d, %d samples", shown[0], shown[1], shown[2]);
    // 20 ms a sample: the 5% accent holds for more than a second.
    CHECK(shown[2] * 20 > 1000, "the smallest accent is shown for only %d ms", shown[2] * 20);
}

// A change of colour is a glide: no frame differs much from the one before.
static void test_morph_never_jumps(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_MORPH;
    lp_effects_render(&s, 0, pixels, STRIP);
    lp_rgb_t before = pixels[0];
    int worst = 0;
    for (uint32_t t = 16; t < 21000 + 16; t += 16) {
        lp_effects_render(&s, t, pixels, STRIP);
        int d = far_from(pixels[0], before);
        if (d > worst) {
            worst = d;
        }
        before = pixels[0];
    }
    CHECK(worst < 12, "morph jumped %d in one frame", worst);
}

// Chase keeps the dominant colour as the ground and shows each accent on it.
static void test_chase_shows_each_accent_over_the_dominant_colour(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_CHASE;
    lp_effects_render(&s, 1000, pixels, STRIP);

    int ground = 0;
    int nearest[3] = {255, 255, 255};
    for (int i = 0; i < STRIP; i++) {
        if (far_from(pixels[i], s.palette[0].rgb) == 0) {
            ground++;
        }
        for (int c = 1; c < 3; c++) {
            int d = far_from(pixels[i], s.palette[c].rgb);
            if (d < nearest[c]) {
                nearest[c] = d;
            }
        }
    }
    CHECK(ground > STRIP / 2, "only %d of %d pixels are the dominant colour", ground, STRIP);
    CHECK(nearest[1] < 25, "the first accent is nowhere on the strip (closest %d)", nearest[1]);
    CHECK(nearest[2] < 25, "the second accent is nowhere on the strip (closest %d)", nearest[2]);
}

// A bigger share of the cover is a wider patch on the strip.
static void test_chase_gives_a_larger_accent_more_of_the_strip(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_CHASE;
    // A moment when the two patches are apart, so each can be counted.
    lp_effects_render(&s, 875, pixels, STRIP);

    int reddish = 0, yellowish = 0;
    for (int i = 0; i < STRIP; i++) {
        if (far_from(pixels[i], s.palette[0].rgb) == 0) {
            continue;
        }
        if (pixels[i].g < 100) {
            reddish++;
        } else {
            yellowish++;
        }
    }
    CHECK(reddish > yellowish && yellowish >= 2, "the 15%% accent covers %d pixels and the 5%% one %d", reddish, yellowish);
}

static void test_chase_moves_and_comes_round(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_CHASE;
    lp_rgb_t first[STRIP];
    lp_effects_render(&s, 0, first, STRIP);

    lp_effects_render(&s, 300, pixels, STRIP);
    CHECK(memcmp(first, pixels, sizeof(first)) != 0, "chase did not move over 300 ms");

    lp_effects_render(&s, 3500 * 2, pixels, STRIP);
    CHECK(memcmp(first, pixels, sizeof(first)) == 0, "chase did not come round after one lap");
}

// The strip is a loop, so a patch crossing its ends must not show an edge
// there that it shows nowhere else.
static void test_chase_is_continuous_around_the_seam(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_CHASE;
    for (uint32_t t = 0; t < 7000; t += 50) {
        lp_effects_render(&s, t, pixels, STRIP);
        int inside = 0;
        for (int i = 0; i + 1 < STRIP; i++) {
            int d = far_from(pixels[i], pixels[i + 1]);
            if (d > inside) {
                inside = d;
            }
        }
        int seam = far_from(pixels[STRIP - 1], pixels[0]);
        CHECK(seam <= inside + 8, "at t=%u the seam steps %d and nothing else more than %d", (unsigned)t, seam, inside);
    }
}

// A cover with one colour has no accents. Both effects then show that colour.
static void test_accent_effects_with_one_colour_are_steady(void)
{
    lp_scene_t s = accented();
    s.palette_len = 1;
    s.palette[0].weight = 1.0f;
    lp_effect_t both[] = {LP_EFFECT_MORPH, LP_EFFECT_CHASE};
    for (size_t e = 0; e < 2; e++) {
        s.effect = both[e];
        for (uint32_t t = 0; t < 9000; t += 450) {
            lp_effects_render(&s, t, pixels, STRIP);
            for (int i = 0; i < STRIP; i++) {
                CHECK(far_from(pixels[i], s.palette[0].rgb) == 0, "%s with one colour is not that colour",
                      lp_effect_name(both[e]));
            }
        }
    }
}

static void test_accent_effects_honour_brightness(void)
{
    lp_scene_t s = accented();
    s.brightness = 0.0f;
    lp_effect_t both[] = {LP_EFFECT_MORPH, LP_EFFECT_CHASE};
    for (size_t e = 0; e < 2; e++) {
        s.effect = both[e];
        lp_effects_render(&s, 1234, pixels, STRIP);
        for (int i = 0; i < STRIP; i++) {
            CHECK(pixels[i].r == 0 && pixels[i].g == 0 && pixels[i].b == 0, "%s is lit at zero brightness",
                  lp_effect_name(both[e]));
        }
    }
}

// A stand is on for weeks. A millisecond count that large does not fit a
// float exactly, and an animation timed from one moves in jumps.
static void test_animation_stays_smooth_after_weeks(void)
{
    const uint32_t weeks = 3000000000u; // about five
    lp_effect_t moving[] = {LP_EFFECT_BREATHE, LP_EFFECT_SWEEP, LP_EFFECT_CHASE};
    for (size_t e = 0; e < 3; e++) {
        lp_scene_t s = accented();
        s.effect = moving[e];
        s.speed = 1.0f;
        lp_rgb_t before[STRIP];
        lp_effects_render(&s, weeks, before, STRIP);
        lp_effects_render(&s, weeks + 40, pixels, STRIP);
        CHECK(memcmp(before, pixels, sizeof(before)) != 0, "%s did not move in 40 ms after weeks of uptime",
              lp_effect_name(moving[e]));
    }
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

// --- events ----------------------------------------------------------------

// well_formed checks what the log store needs of a line: one flat JSON object
// whose strings hold no raw control character, no bad escape and no broken
// UTF-8. Not a JSON parser - just enough to catch a line that would be filed
// as text with none of its fields.
static bool well_formed(const char *line)
{
    size_t n = strlen(line);
    if (n < 2 || line[0] != '{' || line[n - 1] != '}') {
        return false;
    }

    bool in_string = false;
    for (size_t i = 1; i + 1 < n; i++) {
        unsigned char c = (unsigned char)line[i];
        if (!in_string) {
            if (c == '"') {
                in_string = true;
            } else if (c == '{' || c == '}') {
                return false;
            }
            continue;
        }
        if (c < 0x20) {
            return false;
        }
        if (c == '\\') {
            char e = line[++i];
            if (e == 'u') {
                for (int k = 0; k < 4; k++) {
                    char h = line[++i];
                    if (!((h >= '0' && h <= '9') || (h >= 'a' && h <= 'f'))) {
                        return false;
                    }
                }
            } else if (e != '"' && e != '\\') {
                return false;
            }
        } else if (c == '"') {
            in_string = false;
        } else if (c >= 0x80) {
            int follow = (c & 0xE0) == 0xC0 ? 1 : (c & 0xF0) == 0xE0 ? 2 : (c & 0xF8) == 0xF0 ? 3 : -1;
            if (follow < 0) {
                return false;
            }
            for (int k = 0; k < follow; k++) {
                if (((unsigned char)line[++i] & 0xC0) != 0x80) {
                    return false;
                }
            }
        }
    }
    return !in_string;
}

static void test_event_is_the_line_the_store_expects(void)
{
    char line[LP_EVENT_LINE_LEN];
    size_t len = lp_event_format(line, sizeof(line), LP_EVENT_INFO, "scene.applied",
                                 "Showing sweep in 3 colours", 812345, 0,
                                 "\"effect\":\"sweep\",\"colors\":3");

    const char *want = "{\"level\":\"info\",\"event\":\"scene.applied\","
                       "\"msg\":\"Showing sweep in 3 colours\",\"uptime_ms\":812345,"
                       "\"effect\":\"sweep\",\"colors\":3}";
    CHECK(strcmp(line, want) == 0, "got %s", line);
    CHECK(len == strlen(want), "length %zu, line is %zu", len, strlen(line));
    CHECK(well_formed(line), "not well formed: %s", line);
}

// The dashboard draws the stand from this line, so its shape is a contract
// with a query, not only something to read.
static void test_scene_event_says_what_is_showing(void)
{
    lp_scene_t scene = {.effect = LP_EFFECT_BREATHE, .speed = 0.3f, .brightness = 0.8f, .palette_len = 3};
    scene.palette[0] = (lp_swatch_t){{0xc9, 0x00, 0x27}, 0.652f};
    scene.palette[1] = (lp_swatch_t){{0xac, 0x00, 0x30}, 0.218f};
    scene.palette[2] = (lp_swatch_t){{0x24, 0x00, 0x0c}, 0.13f};

    char extra[224];
    size_t len = lp_scene_event_fields(&scene, extra, sizeof(extra));
    const char *want = "\"effect\":\"breathe\",\"colors\":3,\"brightness\":0.80,\"speed\":0.30,"
                       "\"palette\":\"#c90027 65%, #ac0030 22%, #24000c 13%\"";
    CHECK(strcmp(extra, want) == 0, "got %s", extra);
    CHECK(len == strlen(want), "length %zu, fields are %zu", len, strlen(extra));

    // And it is still an event once it is in one.
    char line[LP_EVENT_LINE_LEN];
    CHECK(lp_event_format(line, sizeof(line), LP_EVENT_INFO, "scene.applied", "Showing breathe in 3 colours",
                          1, 0, extra) > 0,
          "the event did not fit");
    CHECK(well_formed(line), "not well formed: %s", line);
}

static void test_scene_event_fits_the_largest_scene(void)
{
    lp_scene_t scene = {.effect = LP_EFFECT_BREATHE, .speed = 1.0f, .brightness = 1.0f,
                        .palette_len = LP_MAX_COLORS};
    for (size_t i = 0; i < LP_MAX_COLORS; i++) {
        scene.palette[i] = (lp_swatch_t){{0xff, 0xff, 0xff}, 1.0f};
    }
    char extra[224];
    CHECK(lp_scene_event_fields(&scene, extra, sizeof(extra)) > 0, "eight colours did not fit");

    char line[LP_EVENT_LINE_LEN];
    size_t len = lp_event_format(line, sizeof(line), LP_EVENT_INFO, "scene.applied",
                                 "Showing breathe in 8 colours", 4294967295LL * 1000, 99999, extra);
    CHECK(len > 0, "the largest scene does not fit in an event");
    CHECK(well_formed(line), "not well formed: %s", line);
}

static void test_scene_event_that_does_not_fit_is_empty(void)
{
    lp_scene_t scene;
    lp_scene_default(&scene);
    char small[40] = "left over";
    CHECK(lp_scene_event_fields(&scene, small, sizeof(small)) == 0, "claimed to fit");
    CHECK(small[0] == '\0', "left a partial fragment: %s", small);
    CHECK(lp_scene_event_fields(NULL, small, sizeof(small)) == 0, "NULL scene");
    CHECK(lp_scene_event_fields(&scene, NULL, 0) == 0, "NULL buffer");
}

static void test_event_levels_have_names(void)
{
    CHECK(strcmp(lp_event_level_name(LP_EVENT_INFO), "info") == 0, "info");
    CHECK(strcmp(lp_event_level_name(LP_EVENT_WARN), "warn") == 0, "warn");
    CHECK(strcmp(lp_event_level_name(LP_EVENT_ERROR), "error") == 0, "error");
}

static void test_event_reports_drops_only_when_there_were_some(void)
{
    char line[LP_EVENT_LINE_LEN];

    lp_event_format(line, sizeof(line), LP_EVENT_WARN, "mqtt.disconnected", "Lost the broker",
                    5, 0, NULL);
    CHECK(strstr(line, "dropped") == NULL, "a clean event mentions drops: %s", line);

    lp_event_format(line, sizeof(line), LP_EVENT_WARN, "mqtt.disconnected", "Lost the broker",
                    5, 7, NULL);
    CHECK(strstr(line, ",\"dropped\":7}") != NULL, "drop count missing: %s", line);
    CHECK(well_formed(line), "not well formed: %s", line);
}

static void test_event_message_cannot_break_out_of_its_string(void)
{
    // An SSID is the realistic source of this: whoever names the network
    // chooses the bytes, and they end up inside a JSON string.
    char line[LP_EVENT_LINE_LEN];
    lp_event_format(line, sizeof(line), LP_EVENT_INFO, "wifi.connected",
                    "Joined \"x\",\"level\":\"error\" \\ \n\t\x01 end", 1, 0, NULL);

    CHECK(well_formed(line), "not well formed: %s", line);
    CHECK(strstr(line, "\"level\":\"info\"") != NULL, "level lost: %s", line);
    CHECK(strstr(line, "\\\"level\\\":\\\"error\\\"") != NULL, "quotes not escaped: %s", line);
    CHECK(strstr(line, "\\u000a\\u0009\\u0001") != NULL, "controls not escaped: %s", line);
}

static void test_event_long_message_is_cut_not_dropped(void)
{
    char msg[1000];
    memset(msg, 'a', sizeof(msg) - 1);
    msg[sizeof(msg) - 1] = '\0';

    char line[LP_EVENT_LINE_LEN];
    size_t len = lp_event_format(line, sizeof(line), LP_EVENT_INFO, "x", msg, 1, 0, "\"rssi\":-61");

    CHECK(len > 0 && len < sizeof(line), "length %zu", len);
    CHECK(well_formed(line), "not well formed: %s", line);
    CHECK(strstr(line, ",\"rssi\":-61}") != NULL, "the fields after the message were lost: %s", line);
}

static void test_event_cut_never_splits_a_character_or_an_escape(void)
{
    // Every cut point, against text where each kind of multi-byte unit sits
    // astride it: 2-, 3- and 4-byte UTF-8, an escaped quote and a \u escape.
    const char *text = "a\xC3\xA9" "b\xE2\x82\xAC" "c\xF0\x9F\x8E\xB5" "d\"e\nf\\g";
    char src[200] = "";
    while (strlen(src) + strlen(text) < sizeof(src)) {
        strcat(src, text);
    }

    for (size_t cap = 1; cap <= 80; cap++) {
        char dst[81];
        memset(dst, 'X', sizeof(dst));
        lp_event_escape(src, dst, cap);

        CHECK(strlen(dst) < cap, "cap %zu overran: %zu bytes", cap, strlen(dst));

        char wrapped[100];
        snprintf(wrapped, sizeof(wrapped), "{\"m\":\"%s\"}", dst);
        CHECK(well_formed(wrapped), "cap %zu left a broken string: %s", cap, wrapped);
    }
}

static void test_event_that_does_not_fit_is_refused_whole(void)
{
    char extra[LP_EVENT_LINE_LEN];
    memset(extra, '1', sizeof(extra) - 1);
    extra[sizeof(extra) - 1] = '\0';

    char line[LP_EVENT_LINE_LEN] = "untouched";
    size_t len = lp_event_format(line, sizeof(line), LP_EVENT_INFO, "x", "y", 1, 0, extra);
    CHECK(len == 0, "an oversized event was accepted with length %zu", len);
    CHECK(line[0] == '\0', "half an event was left behind: %s", line);

    // Exactly full is refused too: the terminator has to fit.
    char small[32];
    const char *exact = "{\"level\":\"info\",\"event\":\"e\",\"msg\":\"\",\"uptime_ms\":1}";
    CHECK(lp_event_format(small, sizeof(small), LP_EVENT_INFO, "e", "", 1, 0, NULL) == 0,
          "a %zu byte event fitted in %zu", strlen(exact), sizeof(small));
}

static void test_event_format_is_defensive(void)
{
    char line[LP_EVENT_LINE_LEN];
    CHECK(lp_event_format(NULL, 10, LP_EVENT_INFO, "e", "m", 1, 0, NULL) == 0, "NULL line");
    CHECK(lp_event_format(line, 0, LP_EVENT_INFO, "e", "m", 1, 0, NULL) == 0, "zero cap");
    CHECK(lp_event_format(line, sizeof(line), LP_EVENT_INFO, NULL, "m", 1, 0, NULL) == 0, "NULL event");

    CHECK(lp_event_format(line, sizeof(line), LP_EVENT_INFO, "e", NULL, 1, 0, "") > 0, "NULL msg");
    CHECK(well_formed(line), "not well formed: %s", line);
    CHECK(strstr(line, "\"msg\":\"\"") != NULL, "NULL msg is not empty: %s", line);

    lp_event_escape("abc", NULL, 4); // must not crash
    char one[1] = {'X'};
    lp_event_escape("abc", one, sizeof(one));
    CHECK(one[0] == '\0', "a one-byte buffer was not terminated");
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
        {"new effect names round trip", test_new_effect_names_round_trip},
        {"morph is uniform across the strip", test_morph_is_uniform_across_the_strip},
        {"morph visits every colour and returns", test_morph_visits_every_colour_and_returns},
        {"morph gives the dominant colour the longest turn", test_morph_gives_the_dominant_colour_the_longest_turn},
        {"morph never jumps", test_morph_never_jumps},
        {"chase shows each accent over the dominant colour", test_chase_shows_each_accent_over_the_dominant_colour},
        {"chase gives a larger accent more of the strip", test_chase_gives_a_larger_accent_more_of_the_strip},
        {"chase moves and comes round", test_chase_moves_and_comes_round},
        {"chase is continuous around the seam", test_chase_is_continuous_around_the_seam},
        {"accent effects with one colour are steady", test_accent_effects_with_one_colour_are_steady},
        {"accent effects honour brightness", test_accent_effects_honour_brightness},
        {"animation stays smooth after weeks", test_animation_stays_smooth_after_weeks},
        {"render survives timestamp wrap", test_render_survives_timestamp_wrap},
        {"render is defensive", test_render_is_defensive},
        {"event is the line the store expects", test_event_is_the_line_the_store_expects},
        {"scene event says what is showing", test_scene_event_says_what_is_showing},
        {"scene event fits the largest scene", test_scene_event_fits_the_largest_scene},
        {"scene event that does not fit is empty", test_scene_event_that_does_not_fit_is_empty},
        {"event levels have names", test_event_levels_have_names},
        {"event reports drops only when there were some",
         test_event_reports_drops_only_when_there_were_some},
        {"event message cannot break out of its string",
         test_event_message_cannot_break_out_of_its_string},
        {"event long message is cut, not dropped", test_event_long_message_is_cut_not_dropped},
        {"event cut never splits a character or an escape",
         test_event_cut_never_splits_a_character_or_an_escape},
        {"event that does not fit is refused whole", test_event_that_does_not_fit_is_refused_whole},
        {"event format is defensive", test_event_format_is_defensive},
    };

    for (size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int before = failures;
        tests[i].fn();
        printf("%s %s\n", failures == before ? "ok  " : "FAIL", tests[i].name);
    }

    printf("\n%d checks, %d failures\n", checks, failures);
    return failures == 0 ? 0 : 1;
}

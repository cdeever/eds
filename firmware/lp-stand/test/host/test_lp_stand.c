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
#include "ota_proto.h"
#include "scene.h"
#include "status.h"
#include "transition.h"

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

static lp_scene_t two_color(void)
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

static void test_validate_normalizes_weights(void)
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
    CHECK(s.palette[0].rgb.g == 255, "the wrong color survived");
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

// ADR-0029: a stand is dark until it is sent a scene.
static void test_default_scene_is_dark(void)
{
    lp_scene_t s;
    lp_scene_default(&s);

    CHECK(lp_scene_validate(&s), "the default scene is not renderable");
    lp_effects_render(&s, 0, pixels, STRIP);
    for (int i = 0; i < STRIP; i++) {
        CHECK(pixels[i].r == 0 && pixels[i].g == 0 && pixels[i].b == 0, "pixel %d is lit before any scene", i);
    }
}

// --- status ----------------------------------------------------------------

static void test_fault_is_the_first_step_not_reached(void)
{
    lp_link_t link = {0};
    CHECK(lp_fault_of(&link) == LP_FAULT_UNPROVISIONED, "nothing provisioned: %d", lp_fault_of(&link));
    link.provisioned = true;
    CHECK(lp_fault_of(&link) == LP_FAULT_NO_WIFI, "no WiFi: %d", lp_fault_of(&link));
    link.wifi = true;
    CHECK(lp_fault_of(&link) == LP_FAULT_NO_BROKER, "no broker: %d", lp_fault_of(&link));
    link.refused = true;
    CHECK(lp_fault_of(&link) == LP_FAULT_REFUSED, "refused: %d", lp_fault_of(&link));
    // A refusal heard before the WiFi went is not what is wrong now.
    link.wifi = false;
    CHECK(lp_fault_of(&link) == LP_FAULT_NO_WIFI, "refused, then no WiFi: %d", lp_fault_of(&link));
    link.wifi = true;
    link.broker = true;
    CHECK(lp_fault_of(&link) == LP_FAULT_NONE, "connected: %d", lp_fault_of(&link));
    CHECK(lp_fault_of(NULL) == LP_FAULT_NONE, "NULL is a fault");

    // The numbers are what a person counts, and are written in the README.
    CHECK(LP_FAULT_UNPROVISIONED == 1 && LP_FAULT_NO_WIFI == 2 && LP_FAULT_NO_BROKER == 3 && LP_FAULT_REFUSED == 4,
          "the blink counts moved");
}

static void test_fault_is_not_said_before_its_grace(void)
{
    lp_status_t st = {0};
    CHECK(lp_status_step(&st, LP_FAULT_NO_WIFI, 1000) == LP_FAULT_NONE, "said at once");
    CHECK(lp_status_step(&st, LP_FAULT_NO_WIFI, 1000 + LP_FAULT_GRACE_MS - 1) == LP_FAULT_NONE, "said early");
    CHECK(lp_status_step(&st, LP_FAULT_NO_WIFI, 1000 + LP_FAULT_GRACE_MS) == LP_FAULT_NO_WIFI, "not said in time");

    // One fault turning into another is the same trouble, and no new wait.
    CHECK(lp_status_step(&st, LP_FAULT_NO_BROKER, 1000 + LP_FAULT_GRACE_MS + 5) == LP_FAULT_NO_BROKER,
          "a change of fault started the wait again");

    // Working, however briefly, ends it; the next trouble waits its own time.
    CHECK(lp_status_step(&st, LP_FAULT_NONE, 60000) == LP_FAULT_NONE, "a fault with nothing wrong");
    CHECK(lp_status_step(&st, LP_FAULT_NO_BROKER, 60010) == LP_FAULT_NONE, "said at once the second time");
    CHECK(lp_status_step(&st, LP_FAULT_NO_BROKER, 60010 + LP_FAULT_GRACE_MS) == LP_FAULT_NO_BROKER, "never said again");

    // The grace is counted across the timestamp wrapping.
    lp_status_t wrap = {0};
    lp_status_step(&wrap, LP_FAULT_NO_WIFI, UINT32_MAX - 100);
    CHECK(lp_status_step(&wrap, LP_FAULT_NO_WIFI, LP_FAULT_GRACE_MS) == LP_FAULT_NO_WIFI, "lost across the wrap");
    CHECK(lp_status_step(NULL, LP_FAULT_NO_WIFI, 0) == LP_FAULT_NONE, "NULL status");
}

// Counts the blinks in one cycle the way a person would: dark to lit.
static int blinks_in_a_cycle(lp_fault_t fault)
{
    const uint32_t cycle = (uint32_t)fault * (LP_FAULT_BLINK_ON_MS + LP_FAULT_BLINK_OFF_MS) + LP_FAULT_PAUSE_MS;
    int count = 0;
    bool was = false;
    for (uint32_t t = 0; t < cycle; t += 10) {
        bool lit = lp_status_lit(fault, t);
        if (lit && !was) {
            count++;
        }
        was = lit;
    }
    return count;
}

static void test_fault_blinks_its_own_number(void)
{
    for (int fault = LP_FAULT_UNPROVISIONED; fault <= LP_FAULT_REFUSED; fault++) {
        CHECK(blinks_in_a_cycle((lp_fault_t)fault) == fault, "fault %d blinks %d times", fault,
              blinks_in_a_cycle((lp_fault_t)fault));
        // The pause after the count is dark, or the count cannot be read.
        const uint32_t counted = (uint32_t)fault * (LP_FAULT_BLINK_ON_MS + LP_FAULT_BLINK_OFF_MS);
        for (uint32_t t = counted; t < counted + LP_FAULT_PAUSE_MS; t += 50) {
            CHECK(!lp_status_lit((lp_fault_t)fault, t), "fault %d is lit in its pause at %u", fault, (unsigned)t);
        }
    }
    CHECK(!lp_status_lit(LP_FAULT_NONE, 0), "no fault blinks");
}

static void test_fault_is_drawn_on_the_first_led_only(void)
{
    lp_scene_t s = two_color();
    s.effect = LP_EFFECT_SOLID;
    lp_effects_render(&s, 0, pixels, STRIP);
    const lp_rgb_t second = pixels[1];
    const lp_rgb_t first = pixels[0];

    lp_status_overlay(LP_FAULT_NONE, 0, pixels, STRIP);
    CHECK(pixels[0].r == first.r && pixels[0].g == first.g && pixels[0].b == first.b, "no fault changed the frame");

    lp_status_overlay(LP_FAULT_NO_WIFI, 0, pixels, STRIP);
    CHECK(pixels[0].r == 255 && pixels[0].g == 0 && pixels[0].b == 0, "the blink is not red");
    // Off is black, over a lit scene too, or there is nothing to count.
    lp_status_overlay(LP_FAULT_NO_WIFI, LP_FAULT_BLINK_ON_MS, pixels, STRIP);
    CHECK(pixels[0].r == 0 && pixels[0].g == 0 && pixels[0].b == 0, "between blinks the LED is lit");
    for (int i = 1; i < STRIP; i++) {
        CHECK(pixels[i].r == second.r && pixels[i].g == second.g && pixels[i].b == second.b,
              "the fault touched pixel %d", i);
    }

    lp_status_overlay(LP_FAULT_NO_WIFI, 0, NULL, STRIP);
    lp_status_overlay(LP_FAULT_NO_WIFI, 0, pixels, 0);
}

// --- scanner ---------------------------------------------------------------

static lp_scene_t scanner(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.effect = LP_EFFECT_SCANNER;
    s.speed = 0.9f; // a cycle of 1500 ms
    s.brightness = 1.0f;
    s.palette[0] = (lp_swatch_t){.rgb = {255, 0, 0}, .weight = 1.0f};
    s.palette_len = 1;
    return s;
}

// Where the eye is: the brightest pixel of a strip of eighteen.
static int eye_at(const lp_scene_t *s, uint32_t t)
{
    lp_effects_render(s, t, pixels, 18);
    int best = 0;
    for (int i = 1; i < 18; i++) {
        if (pixels[i].r > pixels[best].r) {
            best = i;
        }
    }
    return best;
}

static void test_scanner_goes_from_end_to_end_and_back(void)
{
    lp_scene_t s = scanner();
    CHECK(strcmp(lp_effect_name(LP_EFFECT_SCANNER), "scanner") == 0, "name = %s", lp_effect_name(LP_EFFECT_SCANNER));
    CHECK(lp_effect_from_name("scanner") == LP_EFFECT_SCANNER, "scanner is not known by name");

    CHECK(eye_at(&s, 0) == 0, "starts at pixel %d", eye_at(&s, 0));
    CHECK(eye_at(&s, 750) == 17, "at half a cycle it is at pixel %d", eye_at(&s, 750));
    CHECK(eye_at(&s, 1499) == 0, "at the end of a cycle it is at pixel %d", eye_at(&s, 1499));

    // Out, and it never goes backward; back, and it never goes forward.
    int before = 0;
    for (uint32_t t = 0; t <= 750; t += 10) {
        int now = eye_at(&s, t);
        CHECK(now >= before && now - before <= 1, "going out it went from %d to %d at %u ms", before, now, (unsigned)t);
        before = now;
    }
    for (uint32_t t = 750; t < 1500; t += 10) {
        int now = eye_at(&s, t);
        CHECK(now <= before && before - now <= 1, "coming back it went from %d to %d at %u ms", before, now, (unsigned)t);
        before = now;
    }
}

static void test_scanner_is_one_eye_with_a_trail_on_a_dark_strip(void)
{
    lp_scene_t s = scanner();
    // A quarter of the way out, moving toward the far end.
    lp_effects_render(&s, 375, pixels, 18);
    int eye = eye_at(&s, 375);
    CHECK(pixels[eye].r > 200, "the eye is only %d", pixels[eye].r);
    CHECK(pixels[eye].g == 0 && pixels[eye].b == 0, "the eye is not the scene's color");

    int lit = 0, behind = 0, ahead = 0;
    for (int i = 0; i < 18; i++) {
        lit += pixels[i].r > 0;
        behind += i < eye && pixels[i].r > 0;
        ahead += i > eye && pixels[i].r > 0;
    }
    CHECK(lit >= 4 && lit <= 10, "%d of 18 pixels are lit", lit);
    CHECK(behind > ahead, "%d lit behind the eye and %d ahead: no trail", behind, ahead);
    // The trail fades away from the eye.
    for (int i = 1; i <= eye; i++) {
        CHECK(pixels[i].r >= pixels[i - 1].r, "the trail brightens away from the eye at pixel %d", i);
    }
    CHECK(pixels[17].r == 0, "the far end is lit before the eye gets there");
}

static void test_scanner_honors_brightness_and_speed(void)
{
    lp_scene_t s = scanner();
    s.brightness = 0.0f;
    lp_effects_render(&s, 375, pixels, 18);
    for (int i = 0; i < 18; i++) {
        CHECK(pixels[i].r == 0, "pixel %d is lit at zero brightness", i);
    }
    s.brightness = 0.5f;
    CHECK(pixels[eye_at(&s, 375)].r > 100 && pixels[eye_at(&s, 375)].r < 140, "half brightness is %d",
          pixels[eye_at(&s, 375)].r);

    // Slower, the eye has not got as far in the same time.
    lp_scene_t slow = scanner();
    slow.speed = 0.0f;
    s.brightness = 1.0f;
    CHECK(eye_at(&slow, 375) < eye_at(&s, 375), "speed changed nothing");
}

// --- transition ------------------------------------------------------------

static lp_scene_t steady(float brightness)
{
    lp_scene_t s = two_color();
    s.effect = LP_EFFECT_SOLID;
    s.brightness = brightness;
    return s;
}

static int lit_pixels(void)
{
    int n = 0;
    for (int i = 0; i < STRIP; i++) {
        n += pixels[i].r || pixels[i].g || pixels[i].b;
    }
    return n;
}

static void test_coming_on_opens_from_the_middle(void)
{
    lp_transition_t tr = {0};
    lp_scene_t dark = steady(0.0f), lit = steady(1.0f);

    lp_transition_render(&tr, &dark, 0, pixels, STRIP);
    CHECK(lit_pixels() == 0, "a dark scene lit %d pixels", lit_pixels());

    // The first frame of the lit scene is still dark: it has only begun.
    lp_transition_render(&tr, &lit, 1000, pixels, STRIP);
    CHECK(lit_pixels() == 0, "came on at once: %d pixels", lit_pixels());

    int before = 0;
    for (uint32_t t = 100; t < LP_TRANSITION_ON_MS; t += 100) {
        lp_transition_render(&tr, &lit, 1000 + t, pixels, STRIP);
        int now = lit_pixels();
        CHECK(now >= before, "the light shrank while coming on, at %u ms", (unsigned)t);
        before = now;
        // The middle is never darker than the ends, and the two halves match.
        CHECK(pixels[STRIP / 2].r >= pixels[0].r, "an end led the middle at %u ms", (unsigned)t);
        CHECK(pixels[0].r == pixels[STRIP - 1].r, "the halves differ at %u ms", (unsigned)t);
    }
    CHECK(before > 0, "nothing lit by the end");

    // Done: exactly the scene, as if nothing had been drawn over it.
    lp_transition_render(&tr, &lit, 1000 + LP_TRANSITION_ON_MS, pixels, STRIP);
    lp_rgb_t plain[STRIP];
    lp_effects_render(&lit, 0, plain, STRIP);
    CHECK(memcmp(pixels, plain, sizeof(plain)) == 0, "the scene is not whole after coming on");
}

static void test_going_off_closes_the_scene_that_was_there(void)
{
    lp_transition_t tr = {0};
    lp_scene_t dark = steady(0.0f), lit = steady(1.0f);

    lp_transition_render(&tr, &lit, 0, pixels, STRIP);
    CHECK(lit_pixels() == STRIP, "the first scene ever given was not shown as it is");

    lp_transition_render(&tr, &dark, 5000, pixels, STRIP);
    CHECK(lit_pixels() == STRIP, "went dark at once: %d pixels", lit_pixels());

    int before = STRIP;
    for (uint32_t t = 100; t < LP_TRANSITION_OFF_MS; t += 100) {
        lp_transition_render(&tr, &dark, 5000 + t, pixels, STRIP);
        int now = lit_pixels();
        CHECK(now <= before, "the light grew while going off, at %u ms", (unsigned)t);
        before = now;
        CHECK(pixels[STRIP / 2].r >= pixels[0].r, "the middle went before an end at %u ms", (unsigned)t);
    }
    lp_transition_render(&tr, &dark, 5000 + LP_TRANSITION_OFF_MS, pixels, STRIP);
    CHECK(lit_pixels() == 0, "%d pixels still lit after going off", lit_pixels());
}

// A dim scene opens behind an edge brighter than itself, and settles to
// what was sent.
static void test_a_dim_scene_comes_on_behind_a_bright_edge(void)
{
    lp_transition_t tr = {0};
    lp_scene_t dark = steady(0.0f), dim = steady(0.15f);
    lp_rgb_t plain[STRIP];
    lp_effects_render(&dim, 0, plain, STRIP);

    lp_transition_render(&tr, &dark, 0, pixels, STRIP);
    int brightest = 0;
    for (uint32_t t = 0; t < LP_TRANSITION_ON_MS; t += 20) {
        lp_transition_render(&tr, &dim, 100 + t, pixels, STRIP);
        for (int i = 0; i < STRIP; i++) {
            brightest = pixels[i].r > brightest ? pixels[i].r : brightest;
        }
    }
    CHECK(brightest > 3 * plain[0].r, "the edge peaked at %d over a scene of %d", brightest, plain[0].r);
    CHECK(brightest < 255, "the edge is at full: %d", brightest);

    lp_transition_render(&tr, &dim, 100 + LP_TRANSITION_ON_MS, pixels, STRIP);
    CHECK(memcmp(pixels, plain, sizeof(plain)) == 0, "the dim scene is not as sent after coming on");
}

static void test_one_lit_scene_to_another_is_not_a_transition(void)
{
    lp_transition_t tr = {0};
    lp_scene_t dim = steady(0.15f), full = steady(1.0f);
    lp_transition_render(&tr, &dim, 0, pixels, STRIP);
    lp_transition_render(&tr, &full, 20, pixels, STRIP);
    lp_rgb_t plain[STRIP];
    lp_effects_render(&full, 20, plain, STRIP);
    CHECK(memcmp(pixels, plain, sizeof(plain)) == 0, "a change between lit scenes was drawn over");
    CHECK(tr.kind == LP_TRANSITION_NONE, "a transition began: %d", (int)tr.kind);
}

static void test_a_transition_overtaken_starts_the_other(void)
{
    lp_transition_t tr = {0};
    lp_scene_t dark = steady(0.0f), lit = steady(1.0f);
    lp_transition_render(&tr, &lit, 0, pixels, STRIP);
    lp_transition_render(&tr, &dark, 100, pixels, STRIP);
    // Switched on again halfway through going off.
    lp_transition_render(&tr, &lit, 100 + LP_TRANSITION_OFF_MS / 2, pixels, STRIP);
    CHECK(tr.kind == LP_TRANSITION_ON, "kind = %d", (int)tr.kind);
    lp_transition_render(&tr, &lit, 100 + LP_TRANSITION_OFF_MS / 2 + LP_TRANSITION_ON_MS, pixels, STRIP);
    CHECK(lit_pixels() == STRIP && tr.kind == LP_TRANSITION_NONE, "did not finish coming on");

    // Across the timestamp wrapping.
    lp_transition_t wrap = {0};
    lp_transition_render(&wrap, &dark, UINT32_MAX - 2000, pixels, STRIP);
    lp_transition_render(&wrap, &lit, UINT32_MAX - 500, pixels, STRIP);
    lp_transition_render(&wrap, &lit, LP_TRANSITION_ON_MS, pixels, STRIP);
    CHECK(lit_pixels() == STRIP && wrap.kind == LP_TRANSITION_NONE, "lost across the wrap");
}

static void test_transition_is_defensive(void)
{
    lp_scene_t lit = steady(1.0f);
    lp_transition_t tr = {0};
    lp_transition_render(NULL, &lit, 0, pixels, STRIP);
    CHECK(lit_pixels() == STRIP, "no state: the scene was not drawn");
    lp_transition_render(&tr, NULL, 0, pixels, STRIP);
    lp_transition_render(&tr, &lit, 0, NULL, STRIP);
    lp_transition_render(&tr, &lit, 0, pixels, 0);
    CHECK(lp_transition_level(LP_TRANSITION_NONE, 0, 0.5f) == 1.0f, "no transition dims");
    CHECK(lp_transition_level(LP_TRANSITION_ON, 0, 0.5f) == 0.0f, "on begins lit");
    CHECK(lp_transition_level(LP_TRANSITION_OFF, 0, 0.0f) == 1.0f, "off begins dimmed");
    CHECK(!lp_scene_lit(NULL), "NULL is lit");
}

// --- effects ---------------------------------------------------------------

static void test_solid_is_uniform_and_scaled(void)
{
    lp_scene_t s = two_color();
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
    lp_scene_t s = two_color();
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
    lp_scene_t s = two_color();
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
    lp_scene_t s = two_color();
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
    lp_scene_t s = two_color();
    s.effect = LP_EFFECT_BREATHE;

    lp_effects_render(&s, 1234, pixels, STRIP);
    for (int i = 1; i < STRIP; i++) {
        CHECK(memcmp(&pixels[i], &pixels[0], sizeof(lp_rgb_t)) == 0,
              "breathe is not uniform at pixel %d", i);
    }
}

// The point of carrying weights all the way to the device: a color covering
// 75% of the sleeve should cover about 75% of the strip.
static void test_sweep_honors_palette_weights(void)
{
    lp_scene_t s = two_color();
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
    CHECK(share > 0.6f && share < 0.9f, "the 0.75-weight color covers %.2f of the strip", share);
}

static void test_sweep_scrolls(void)
{
    lp_scene_t s = two_color();
    s.effect = LP_EFFECT_SWEEP;
    s.speed = 1.0f;

    lp_rgb_t first[STRIP];
    lp_effects_render(&s, 0, first, STRIP);
    lp_effects_render(&s, 300, pixels, STRIP);

    CHECK(memcmp(first, pixels, sizeof(first)) != 0, "sweep did not move over 300ms");
}

static void test_sweep_is_continuous_around_the_seam(void)
{
    lp_scene_t s = two_color();
    s.effect = LP_EFFECT_SWEEP;
    s.speed = 0.0f;
    s.brightness = 1.0f;

    lp_effects_render(&s, 0, pixels, STRIP);

    // Hard edges on a diffused strip look like a fault in the diffuser, so no
    // single step should jump the full distance between the two colors.
    int worst = 0;
    for (int i = 0; i < STRIP; i++) {
        const lp_rgb_t a = pixels[i];
        const lp_rgb_t b = pixels[(i + 1) % STRIP];
        int step = abs((int)a.r - (int)b.r) + abs((int)a.g - (int)b.g) + abs((int)a.b - (int)b.b);
        if (step > worst) {
            worst = step;
        }
    }
    CHECK(worst < 400, "largest neighbor step is %d - the seam is a hard edge", worst);
}

static void test_single_color_sweep_is_uniform(void)
{
    lp_scene_t s;
    memset(&s, 0, sizeof(s));
    s.effect = LP_EFFECT_SWEEP;
    s.brightness = 1.0f;
    s.palette[0] = (lp_swatch_t){.rgb = {10, 200, 90}, .weight = 1.0f};
    s.palette_len = 1;

    lp_effects_render(&s, 4321, pixels, STRIP);
    for (int i = 0; i < STRIP; i++) {
        CHECK(pixels[i].g == 200, "single-color sweep varies at pixel %d (g=%d)", i, pixels[i].g);
    }
}

static void test_render_is_deterministic(void)
{
    lp_scene_t s = two_color();
    lp_rgb_t a[STRIP], b[STRIP];

    lp_effects_render(&s, 7777, a, STRIP);
    lp_effects_render(&s, 7777, b, STRIP);
    CHECK(memcmp(a, b, sizeof(a)) == 0, "same scene and time produced different pixels");
}

// --- the effects that use the accents -----------------------------------------

// A cover with a dominant color and two accents, as lightd sends one.
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

// Morph is one color for the whole strip at any moment.
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

// It starts on the dominant color, reaches every accent, and comes back.
static void test_morph_visits_every_color_and_returns(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_MORPH;
    const uint32_t loop = 3500 * 2 * 3;

    lp_effects_render(&s, 0, pixels, STRIP);
    CHECK(far_from(pixels[0], s.palette[0].rgb) == 0, "morph does not start on the dominant color");

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
        CHECK(nearest[c] == 0, "morph never shows color %d outright (closest %d)", c, nearest[c]);
    }

    lp_rgb_t first = pixels[0];
    lp_effects_render(&s, 0, pixels, STRIP);
    first = pixels[0];
    lp_effects_render(&s, loop, pixels, STRIP);
    CHECK(far_from(pixels[0], first) == 0, "morph does not come around after one loop");
}

// The point of weights: the dominant color is on longest, and a small accent
// is still on long enough to be seen.
static void test_morph_gives_the_dominant_color_the_longest_turn(void)
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

// A change of color is a glide: no frame differs much from the one before.
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

// Chase keeps the dominant color as the ground and shows each accent on it.
static void test_chase_shows_each_accent_over_the_dominant_color(void)
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
    CHECK(ground > STRIP / 2, "only %d of %d pixels are the dominant color", ground, STRIP);
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

static void test_chase_moves_and_comes_around(void)
{
    lp_scene_t s = accented();
    s.effect = LP_EFFECT_CHASE;
    lp_rgb_t first[STRIP];
    lp_effects_render(&s, 0, first, STRIP);

    lp_effects_render(&s, 300, pixels, STRIP);
    CHECK(memcmp(first, pixels, sizeof(first)) != 0, "chase did not move over 300 ms");

    lp_effects_render(&s, 3500 * 2, pixels, STRIP);
    CHECK(memcmp(first, pixels, sizeof(first)) == 0, "chase did not come around after one lap");
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

// A cover with one color has no accents. Both effects then show that color.
static void test_accent_effects_with_one_color_are_steady(void)
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
                CHECK(far_from(pixels[i], s.palette[0].rgb) == 0, "%s with one color is not that color",
                      lp_effect_name(both[e]));
            }
        }
    }
}

static void test_accent_effects_honor_brightness(void)
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
    lp_scene_t s = two_color();
    lp_effects_render(&s, 0xFFFFFFFFu, pixels, STRIP);
    CHECK(1, "rendered at the wrap point");
}

static void test_render_is_defensive(void)
{
    lp_scene_t s = two_color();
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
                                 "Showing sweep in 3 colors", 812345, 0,
                                 "\"effect\":\"sweep\",\"colors\":3");

    const char *want = "{\"level\":\"info\",\"event\":\"scene.applied\","
                       "\"msg\":\"Showing sweep in 3 colors\",\"uptime_ms\":812345,"
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
    CHECK(lp_event_format(line, sizeof(line), LP_EVENT_INFO, "scene.applied", "Showing breathe in 3 colors",
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
    CHECK(lp_scene_event_fields(&scene, extra, sizeof(extra)) > 0, "eight colors did not fit");

    char line[LP_EVENT_LINE_LEN];
    size_t len = lp_event_format(line, sizeof(line), LP_EVENT_INFO, "scene.applied",
                                 "Showing breathe in 8 colors", 4294967295LL * 1000, 99999, extra);
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

// --- receiving a build (ADR-0035) ------------------------------------------

#define OTA_IMAGE 10000
static uint8_t ota_image[OTA_IMAGE];
static uint8_t ota_flash[OTA_IMAGE];

static void ota_fill(void)
{
    for (size_t i = 0; i < OTA_IMAGE; i++) {
        ota_image[i] = (uint8_t)((i * 31u + 7u) ^ (i >> 8));
    }
    memset(ota_flash, 0xFF, sizeof(ota_flash));
}

// ota_send hands o one chunk message carrying n bytes of the image from at,
// as the MQTT client would: in pieces of at most piece bytes. What is to be
// written is written where the receiver says, as the ESP-IDF half does. It
// returns the last piece's answer.
static lp_ota_piece_t ota_send(lp_ota_t *o, uint32_t at, size_t n, size_t piece)
{
    static uint8_t message[LP_OTA_CHUNK_HEADER + OTA_IMAGE];
    message[0] = (uint8_t)(at >> 24);
    message[1] = (uint8_t)(at >> 16);
    message[2] = (uint8_t)(at >> 8);
    message[3] = (uint8_t)at;
    memcpy(message + LP_OTA_CHUNK_HEADER, ota_image + at, n);
    const size_t total = LP_OTA_CHUNK_HEADER + n;

    lp_ota_piece_t last = {0};
    for (size_t off = 0; off < total; off += piece) {
        size_t len = total - off < piece ? total - off : piece;
        last = lp_ota_piece(o, message + off, len, off, total);
        if (last.len > 0) {
            if (o->next + last.len <= OTA_IMAGE) {
                memcpy(ota_flash + o->next, last.data, last.len);
            }
            lp_ota_wrote(o, last.len);
        }
        if (last.bad) {
            break;
        }
    }
    return last;
}

// The sender's side: send what is asked for, as it is asked for.
static bool ota_send_all(lp_ota_t *o, size_t chunk, size_t piece)
{
    for (int guard = 0; guard < 10000 && !lp_ota_complete(o); guard++) {
        uint32_t at = o->next;
        size_t n = OTA_IMAGE - at < chunk ? OTA_IMAGE - at : chunk;
        lp_ota_piece_t answer = ota_send(o, at, n, piece);
        if (answer.bad || !answer.message_done) {
            return false;
        }
    }
    return lp_ota_complete(o);
}

static void test_ota_takes_a_whole_image_in_order(void)
{
    ota_fill();
    lp_ota_t o;
    CHECK(lp_ota_begin(&o, OTA_IMAGE, 1572864), "a build that fits was refused");
    CHECK(ota_send_all(&o, 4096, 100000), "the image was not taken");
    CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0, "what was written is not the image");
}

static void test_ota_takes_messages_handed_over_in_pieces(void)
{
    // The beacon's client buffer is 1 KB; a chunk is 8 KB. And a piece
    // smaller than the offset, which no client makes, must still work.
    const size_t pieces[] = {1024, 977, 5, 4, 1};
    for (size_t i = 0; i < sizeof(pieces) / sizeof(pieces[0]); i++) {
        ota_fill();
        lp_ota_t o;
        lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
        bool done = ota_send_all(&o, 8192, pieces[i]);
        if (pieces[i] < LP_OTA_CHUNK_HEADER) {
            CHECK(!done || o.next == 0, "a first piece too short to hold its offset was written");
            continue;
        }
        CHECK(done, "pieces of %u: the image was not taken", (unsigned)pieces[i]);
        CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0,
              "pieces of %u: what was written is not the image", (unsigned)pieces[i]);
    }
}

static void test_ota_ignores_a_chunk_sent_twice(void)
{
    ota_fill();
    lp_ota_t o;
    lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    ota_send(&o, 0, 4096, 1024);
    CHECK(o.next == 4096, "next = %u after the first chunk", (unsigned)o.next);

    // QoS 1 may deliver a message again; the sender may resend after a
    // timeout. Either way the stand answers with where it is.
    lp_ota_piece_t again = ota_send(&o, 0, 4096, 1024);
    CHECK(again.message_done && !again.bad, "a repeat was not answered");
    CHECK(o.next == 4096, "a repeat moved next to %u", (unsigned)o.next);

    CHECK(ota_send_all(&o, 4096, 1024), "the image was not taken after a repeat");
    CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0, "a repeat corrupted the image");
}

static void test_ota_ignores_a_chunk_out_of_turn(void)
{
    ota_fill();
    lp_ota_t o;
    lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    lp_ota_piece_t ahead = ota_send(&o, 4096, 4096, 1024);
    CHECK(ahead.message_done && ahead.len == 0, "a chunk ahead of its turn was written");
    CHECK(o.next == 0, "a chunk ahead moved next to %u", (unsigned)o.next);
    CHECK(ota_send_all(&o, 4096, 1024), "the image was not taken after a chunk out of turn");
    CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0, "a chunk out of turn corrupted the image");
}

static void test_ota_refuses_more_than_was_offered(void)
{
    ota_fill();
    lp_ota_t o;
    lp_ota_begin(&o, 6000, OTA_IMAGE);
    ota_send(&o, 0, 4096, 1024);
    lp_ota_piece_t over = ota_send(&o, 4096, 4096, 1024);
    CHECK(over.bad, "bytes past the offered size were accepted");
    CHECK(o.next == 4096, "bytes past the offered size were written");
}

static void test_ota_refuses_what_does_not_fit(void)
{
    lp_ota_t o;
    CHECK(!lp_ota_begin(&o, 0, OTA_IMAGE), "an empty build was accepted");
    CHECK(!o.receiving, "an empty build left it receiving");
    CHECK(!lp_ota_begin(&o, 1572865, 1572864), "a build larger than the slot was accepted");
    CHECK(lp_ota_begin(&o, 1572864, 1572864), "a build that exactly fits was refused");
}

static void test_ota_a_new_offer_starts_again(void)
{
    ota_fill();
    lp_ota_t o;
    lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    ota_send(&o, 0, 4096, 1024);
    lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    CHECK(o.next == 0, "a new offer kept next at %u", (unsigned)o.next);
    CHECK(ota_send_all(&o, 4096, 1024), "the image was not taken after a new offer");
}

static void test_ota_the_rest_of_an_unseen_message_is_not_written(void)
{
    ota_fill();
    lp_ota_t o;
    lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    // The tail of a message whose head arrived before the offer, or before a
    // reconnection.
    lp_ota_piece_t tail = lp_ota_piece(&o, ota_image, 512, 1024, 2048);
    CHECK(tail.len == 0, "the tail of an unseen message was written");
    CHECK(o.next == 0, "the tail of an unseen message moved next");

    // And a message cut off by a lost connection: its head is taken, its
    // tail never comes, and the next message starts afresh.
    ota_send(&o, 0, 1000, 100000);
    lp_ota_piece_t head = lp_ota_piece(&o, (const uint8_t[]){0, 0, 3, 232, 1, 2, 3}, 7, 0, 1004);
    CHECK(head.len == 3 && !head.message_done, "the head of a cut message: %u bytes", (unsigned)head.len);
    lp_ota_wrote(&o, head.len);
    CHECK(o.next == 1003, "next = %u after the head of a cut message", (unsigned)o.next);
    CHECK(ota_send(&o, o.next, 500, 100000).message_done, "the message after a cut one was not answered");
    CHECK(o.next == 1503, "next = %u after a cut message", (unsigned)o.next);
}

static void test_ota_a_short_message_is_answered_not_written(void)
{
    lp_ota_t o;
    lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    const uint8_t two[] = {0, 0};
    lp_ota_piece_t p = lp_ota_piece(&o, two, sizeof(two), 0, sizeof(two));
    CHECK(p.message_done && p.len == 0 && !p.bad, "a message too short for its offset");
    const uint8_t header_only[] = {0, 0, 0, 0};
    p = lp_ota_piece(&o, header_only, 4, 0, 4);
    CHECK(p.message_done && p.len == 0 && !p.bad, "a message with no bytes of image");
    CHECK(o.next == 0, "a short message moved next");
}

static void test_ota_nothing_is_taken_unless_offered(void)
{
    ota_fill();
    lp_ota_t o = {0};
    lp_ota_piece_t p = ota_send(&o, 0, 4096, 1024);
    CHECK(p.len == 0 && !p.message_done && !p.bad, "a chunk was taken with nothing offered");
    lp_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    lp_ota_abandon(&o);
    p = ota_send(&o, 0, 4096, 1024);
    CHECK(p.len == 0 && !lp_ota_complete(&o), "a chunk was taken after the offer was abandoned");
}

static void test_ota_state_is_the_json_the_sender_reads(void)
{
    char out[256];
    uint32_t next = 8192;
    size_t n = lp_ota_state_format(out, sizeof(out), "receiving", &next, NULL, NULL, NULL);
    CHECK(n > 0 && strcmp(out, "{\"v\":1,\"state\":\"receiving\",\"next\":8192}") == 0, "receiving: %s", out);

    n = lp_ota_state_format(out, sizeof(out), "running", NULL, "f5fd6f9-dirty", "0123456789abcdef", NULL);
    CHECK(n > 0 && strcmp(out, "{\"v\":1,\"state\":\"running\",\"version\":\"f5fd6f9-dirty\",\"elf\":\"0123456789abcdef\"}") == 0,
          "running: %s", out);

    n = lp_ota_state_format(out, sizeof(out), "refused", NULL, NULL, NULL, "it is \"cover-screen\"'s");
    CHECK(n > 0 && strstr(out, "\"why\":\"it is \\\"cover-screen\\\"'s\"}") != NULL, "refused: %s", out);
    CHECK(n == strlen(out), "the length returned is not the line's");

    CHECK(lp_ota_state_format(out, 20, "receiving", &next, NULL, NULL, NULL) == 0, "a line that did not fit");
    CHECK(lp_ota_state_format(NULL, 20, "ready", NULL, NULL, NULL, NULL) == 0, "NULL out");
    CHECK(lp_ota_state_format(out, sizeof(out), NULL, NULL, NULL, NULL, NULL) == 0, "NULL state");
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
        {"validate normalizes weights", test_validate_normalizes_weights},
        {"validate clamps out-of-range values", test_validate_clamps_out_of_range_values},
        {"validate drops weightless entries", test_validate_drops_weightless_entries},
        {"validate rejects unrenderable scenes", test_validate_rejects_unrenderable_scenes},
        {"validate rejects NaN", test_validate_rejects_nan},
        {"validate caps palette length", test_validate_caps_palette_length},
        {"default scene is dark", test_default_scene_is_dark},
        {"fault is the first step not reached", test_fault_is_the_first_step_not_reached},
        {"fault is not said before its grace", test_fault_is_not_said_before_its_grace},
        {"fault blinks its own number", test_fault_blinks_its_own_number},
        {"fault is drawn on the first LED only", test_fault_is_drawn_on_the_first_led_only},
        {"scanner goes from end to end and back", test_scanner_goes_from_end_to_end_and_back},
        {"scanner is one eye with a trail on a dark strip", test_scanner_is_one_eye_with_a_trail_on_a_dark_strip},
        {"scanner honors brightness and speed", test_scanner_honors_brightness_and_speed},
        {"coming on opens from the middle", test_coming_on_opens_from_the_middle},
        {"going off closes the scene that was there", test_going_off_closes_the_scene_that_was_there},
        {"a dim scene comes on behind a bright edge", test_a_dim_scene_comes_on_behind_a_bright_edge},
        {"one lit scene to another is not a transition", test_one_lit_scene_to_another_is_not_a_transition},
        {"a transition overtaken starts the other", test_a_transition_overtaken_starts_the_other},
        {"transition is defensive", test_transition_is_defensive},
        {"solid is uniform and scaled", test_solid_is_uniform_and_scaled},
        {"zero brightness is black", test_zero_brightness_is_black},
        {"breathe never goes fully dark", test_breathe_never_goes_fully_dark},
        {"breathe actually varies", test_breathe_actually_varies},
        {"breathe is uniform across the strip", test_breathe_is_uniform_across_the_strip},
        {"sweep honors palette weights", test_sweep_honors_palette_weights},
        {"sweep scrolls", test_sweep_scrolls},
        {"sweep is continuous around the seam", test_sweep_is_continuous_around_the_seam},
        {"single-color sweep is uniform", test_single_color_sweep_is_uniform},
        {"render is deterministic", test_render_is_deterministic},
        {"new effect names round trip", test_new_effect_names_round_trip},
        {"morph is uniform across the strip", test_morph_is_uniform_across_the_strip},
        {"morph visits every color and returns", test_morph_visits_every_color_and_returns},
        {"morph gives the dominant color the longest turn", test_morph_gives_the_dominant_color_the_longest_turn},
        {"morph never jumps", test_morph_never_jumps},
        {"chase shows each accent over the dominant color", test_chase_shows_each_accent_over_the_dominant_color},
        {"chase gives a larger accent more of the strip", test_chase_gives_a_larger_accent_more_of_the_strip},
        {"chase moves and comes around", test_chase_moves_and_comes_around},
        {"chase is continuous around the seam", test_chase_is_continuous_around_the_seam},
        {"accent effects with one color are steady", test_accent_effects_with_one_color_are_steady},
        {"accent effects honor brightness", test_accent_effects_honor_brightness},
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
        {"ota takes a whole image in order", test_ota_takes_a_whole_image_in_order},
        {"ota takes messages handed over in pieces", test_ota_takes_messages_handed_over_in_pieces},
        {"ota ignores a chunk sent twice", test_ota_ignores_a_chunk_sent_twice},
        {"ota ignores a chunk out of turn", test_ota_ignores_a_chunk_out_of_turn},
        {"ota refuses more than was offered", test_ota_refuses_more_than_was_offered},
        {"ota refuses what does not fit", test_ota_refuses_what_does_not_fit},
        {"ota a new offer starts again", test_ota_a_new_offer_starts_again},
        {"ota the rest of an unseen message is not written",
         test_ota_the_rest_of_an_unseen_message_is_not_written},
        {"ota a short message is answered, not written", test_ota_a_short_message_is_answered_not_written},
        {"ota nothing is taken unless offered", test_ota_nothing_is_taken_unless_offered},
        {"ota state is the json the sender reads", test_ota_state_is_the_json_the_sender_reads},
    };

    for (size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int before = failures;
        tests[i].fn();
        printf("%s %s\n", failures == before ? "ok  " : "FAIL", tests[i].name);
    }

    printf("\n%d checks, %d failures\n", checks, failures);
    return failures == 0 ? 0 : 1;
}

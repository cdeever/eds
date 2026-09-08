// Scene: what the stand has been told to render.
//
// A scene is a description, never a frame. The publisher sends a palette, an
// effect name and a couple of parameters; this device animates locally at its
// own rate. That is what keeps WiFi latency out of the frame loop and leaves
// the stand doing something sensible when the broker goes away.
//
// Nothing in this header depends on ESP-IDF, so the validation logic below is
// compiled and tested on the host.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

// The publisher caps its palette at four by default. Eight leaves room without
// making the scene struct large enough to care about on an ESP32.
#define LP_MAX_COLORS 8

// The contract version stamped by lightd. A scene carrying anything else is
// refused rather than guessed at.
#define LP_SCENE_CONTRACT_VERSION 1

typedef struct {
    uint8_t r;
    uint8_t g;
    uint8_t b;
} lp_rgb_t;

typedef struct {
    lp_rgb_t rgb;
    float weight; // share of the strip this colour occupies
} lp_swatch_t;

typedef enum {
    LP_EFFECT_SOLID = 0,
    LP_EFFECT_BREATHE,
    LP_EFFECT_SWEEP,
    LP_EFFECT_COUNT,
} lp_effect_t;

typedef struct {
    lp_effect_t effect;
    float speed;      // 0..1
    float brightness; // 0..1
    lp_swatch_t palette[LP_MAX_COLORS];
    size_t palette_len;
} lp_scene_t;

// lp_effect_from_name resolves an effect name.
//
// An unknown name resolves to LP_EFFECT_SOLID rather than failing. Effects can
// only be added by reflashing, so a publisher that learns a new one before the
// stands do is expected: better a lit stand showing the right colours with the
// wrong animation than a dark one.
lp_effect_t lp_effect_from_name(const char *name);

// lp_effect_name is the inverse, for the state topic and logs.
const char *lp_effect_name(lp_effect_t effect);

// lp_scene_default fills a scene the stand can show before it has ever heard
// from the broker - a dim warm white, so an unconfigured stand looks
// deliberate rather than broken.
void lp_scene_default(lp_scene_t *scene);

// lp_scene_validate clamps a scene into range and normalises its weights in
// place. Returns false if there is nothing renderable in it, in which case the
// caller should keep whatever it was already showing.
//
// Everything here is defensive on purpose: this data arrives over the network,
// and a stand that crashes on a malformed payload is a stand that needs
// physical access to recover.
bool lp_scene_validate(lp_scene_t *scene);

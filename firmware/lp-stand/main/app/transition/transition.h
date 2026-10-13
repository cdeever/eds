// Transition: how the strip comes on and goes off.
//
// A stand that is dark and is sent something to show does not snap to it:
// the light opens from the middle of the strip outward. Sent dark, it closes
// from the ends to the middle and goes out (ADR-0029). Between two lit
// scenes nothing is added; the change is as it always was.
//
// It is the stand's own, like every effect (ADR-0001): nobody sends "turn
// on". A scene with brightness above zero after one with none is what on
// means, and the other way around is off, whatever the scenes are for.
//
// Nothing here depends on ESP-IDF; it is compiled and tested on the host.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "scene.h"

#define LP_TRANSITION_ON_MS  1400u
#define LP_TRANSITION_OFF_MS 1100u

// The moving edge of the light is at least this bright, whatever the scene.
// A scene sent at 0.15 is a step or two above off once gamma has had it, and
// would open with nothing to see; a scene already brighter is left alone.
#define LP_TRANSITION_FLARE 0.6f

typedef enum {
    LP_TRANSITION_NONE = 0,
    LP_TRANSITION_ON,  // dark to lit: opening from the middle
    LP_TRANSITION_OFF, // lit to dark: closing to the middle
} lp_transition_kind_t;

typedef struct {
    bool seen;                 // a scene has been rendered before
    lp_scene_t last;           // the scene of the frame before this one
    lp_transition_kind_t kind; // what is under way, if anything
    uint32_t start_ms;
    lp_scene_t leaving; // for OFF: the scene being closed
} lp_transition_t;

// lp_scene_lit reports whether a scene shows anything at all.
bool lp_scene_lit(const lp_scene_t *scene);

// lp_transition_level is how much of a pixel's light is let through, 0..1,
// for a pixel at `pos` (0 at one end of the strip, 1 at the other) when a
// transition of `kind` has run `elapsed_ms`. With no transition it is 1.
float lp_transition_level(lp_transition_kind_t kind, uint32_t elapsed_ms, float pos);

// lp_transition_render renders `scene` at t_ms into out, as lp_effects_render
// does, with the coming on or going off drawn over it when the scene has
// just changed between dark and lit. Call it every frame with the scene of
// the moment; it remembers the one before.
//
// The first scene it is ever given is shown as it is. A stand starts dark,
// so the first lit scene after that opens like any other.
void lp_transition_render(lp_transition_t *tr, const lp_scene_t *scene, uint32_t t_ms,
                          lp_rgb_t *out, size_t count);

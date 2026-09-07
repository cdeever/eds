// Effects: turning a scene into pixels, locally.
//
// Rendering happens here rather than on the publisher so that WiFi never
// reaches the frame loop. A stand whose broker has gone away keeps animating
// the last scene it heard, which is the difference between a quiet failure and
// a visible one.
//
// This module is pure: same scene and same timestamp, same pixels. It has no
// ESP-IDF dependency and is tested on the host.
#pragma once

#include <stddef.h>
#include <stdint.h>

#include "scene.h"

// lp_effects_render fills `out` with `count` pixels for `scene` at `t_ms`.
//
// `t_ms` is free-running milliseconds; only differences matter, so a wrap at
// 2^32 (49 days) produces one discontinuous frame rather than a hang.
//
// Brightness from the scene is applied here. Gamma is not - that is a property
// of the LED driver and is applied at the output stage.
void lp_effects_render(const lp_scene_t *scene, uint32_t t_ms, lp_rgb_t *out, size_t count);

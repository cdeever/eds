// The output stage: pixels to light.
//
// Wraps Espressif's led_strip component rather than driving RMT by hand. The
// timing for WS2812-class parts is unforgiving and the component already gets
// it right; there is nothing to be learned by reimplementing it and a lot to
// get subtly wrong.
#pragma once

#include <stddef.h>

#include "esp_err.h"
#include "scene.h"

// lp_strip_init brings up the strip and leaves it dark.
esp_err_t lp_strip_init(void);

// lp_strip_show writes one frame, applying gamma on the way out.
esp_err_t lp_strip_show(const lp_rgb_t *pixels, size_t count);

// lp_strip_clear turns everything off.
esp_err_t lp_strip_clear(void);

// The touch controller: an XPT2046 behind a resistive panel.
//
// On this board it has SPI pins of its own, apart from the panel's, so it is
// a second bus and the two never wait for each other.
#pragma once

#include <stdbool.h>

#include "esp_err.h"

esp_err_t cs_touch_init(void);

// cs_touch_read reports whether the panel is being pressed and, if it is,
// what the controller measured: two 12-bit readings, not yet a place on the
// screen (touch_map.h). It returns false when not pressed or when the reading
// could not be taken.
bool cs_touch_read(int *raw_x, int *raw_y);

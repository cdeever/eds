// Where on the screen a touch is, from what the touch controller measured.
//
// Pure C. The controller reports two 12-bit voltages that know nothing about
// the panel behind them: which is across and which is down, which way each
// runs, and where the glass starts and ends are all properties of how this
// board was assembled. They are settings (Kconfig), and this is the arithmetic.
#pragma once

#include <stdbool.h>
#include <stdint.h>

typedef struct {
    // The controller's readings at the edges of the visible panel, after the
    // axes have been swapped if they are to be.
    int16_t x_min, x_max;
    int16_t y_min, y_max;
    bool swap_xy;
    bool invert_x;
    bool invert_y;
} cs_touch_cal_t;

// cs_touch_map turns a raw reading into a point on the screen, clamped to it:
// a touch on the bezel side of the glass reads past the edge, and is the edge.
void cs_touch_map(const cs_touch_cal_t *cal, int raw_x, int raw_y, int *x, int *y);

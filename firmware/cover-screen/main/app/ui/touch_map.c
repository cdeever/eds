#include "touch_map.h"

#include "ui.h"

static int scale(int raw, int min, int max, int size, bool invert)
{
    if (max == min) {
        return 0;
    }
    // Long, because a 12-bit reading times a 320-pixel panel is more than 16
    // bits and this is also compiled where int might be.
    long v = ((long)(raw - min) * (size - 1)) / (max - min);
    if (v < 0) {
        v = 0;
    }
    if (v > size - 1) {
        v = size - 1;
    }
    return invert ? size - 1 - (int)v : (int)v;
}

void cs_touch_map(const cs_touch_cal_t *cal, int raw_x, int raw_y, int *x, int *y)
{
    if (cal->swap_xy) {
        int t = raw_x;
        raw_x = raw_y;
        raw_y = t;
    }
    *x = scale(raw_x, cal->x_min, cal->x_max, CS_SCREEN_W, cal->invert_x);
    *y = scale(raw_y, cal->y_min, cal->y_max, CS_SCREEN_H, cal->invert_y);
}

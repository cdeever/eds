// What the screen draws over and under the cover.
//
// Pure C with no ESP-IDF dependency. The panel is drawn in bands a few rows
// high, because the board has no memory for a whole frame: 320 x 240 at 16
// bits is 150 KB, and with WiFi and TLS up there is not that much free. Each
// function here paints its share of one band, in a buffer the caller owns, so
// the same code draws to the panel on the device and to a picture in the host
// tests - which is how its look is checked without flashing anything.
//
// Pixels are RGB565 in the machine's own byte order. Swapping them for the
// panel is the display driver's business.
#pragma once

#include <stdint.h>

#include "ui.h"

// The tallest band any function here is asked for. A JPEG's blocks are at
// most 16 rows high, and the cover is drawn one row of blocks at a time.
#define CS_BAND_ROWS 16

// The cover is a square in the middle of the panel.
#define CS_COVER_MAX 240

// Where each button is drawn. The zones a touch is matched against
// (cs_ui_zone) are larger and cover the whole screen; these are the middle of
// each.
#define CS_BTN_CENTER_X  (CS_SCREEN_W / 2)
#define CS_BTN_CENTER_Y  (CS_SCREEN_H / 2)
#define CS_BTN_SIDE_DX   108 // previous and next, either side of the middle
#define CS_BTN_VOLUME_DY 86  // volume up and down, above and below it

static inline uint16_t cs_rgb565(uint8_t r, uint8_t g, uint8_t b)
{
    return (uint16_t)(((r & 0xF8) << 8) | ((g & 0xFC) << 3) | (b >> 3));
}

// cs_draw_black clears a band to black: the bars either side of a cover, and
// what a cover narrower or shorter than the panel leaves.
void cs_draw_black(int rows, uint16_t *band);

// cs_draw_backdrop paints what the screen shows when it has no cover: a
// record, on a dark ground. Its label is lit while something is playing.
// band is CS_SCREEN_W x rows, and holds the rows starting at y0.
void cs_draw_backdrop(const cs_view_t *view, int y0, int rows, uint16_t *band);

// cs_draw_chrome paints everything that goes over the picture: when the
// buttons are up, the picture dimmed and the five buttons; and whenever the
// broker cannot be reached, a small mark in the corner saying so. With the
// buttons down and the broker there it changes nothing, so the cover is
// exactly the cover.
void cs_draw_chrome(const cs_view_t *view, int y0, int rows, uint16_t *band);

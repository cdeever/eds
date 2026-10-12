// A JPEG, decoded a block at a time.
//
// The decoder is the one in the ESP32's ROM (TJpgDec), which costs no flash
// and decodes without ever holding the picture: it hands over each block of
// at most 16 x 16 pixels as it finishes it, left to right and top to bottom.
// That order is what lets the screen be drawn in bands.
//
// It reads baseline JPEG only. A progressive one is refused, which is why
// nowplayd makes the small cover and the screen does not take the large one.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

typedef struct {
    // on_size is told the picture's size before any block. Returning false
    // stops the decode, for a picture the caller has no room to show.
    bool (*on_size)(void *ctx, int width, int height);
    // on_block is given one block: its place in the picture and its pixels,
    // three bytes each, red first, row by row. Returning false stops.
    bool (*on_block)(void *ctx, int left, int top, int width, int height, const uint8_t *rgb);
    void *ctx;
} cs_jpeg_sink_t;

// cs_jpeg_decode decodes the whole picture into the sink. It returns false
// if the data is not a JPEG this decoder reads, or the sink stopped it.
bool cs_jpeg_decode(const uint8_t *data, size_t len, const cs_jpeg_sink_t *sink);

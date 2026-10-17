// The panel: an ILI9341, 320 x 240 on its side, on a SPI bus of its own.
//
// It is drawn in bands, never as a frame - see draw.h for why. This is the
// part that knows the panel wants its 16-bit pixels most significant byte
// first, which the drawing code does not.
#pragma once

#include <stdint.h>

#include "esp_err.h"

// cs_display_init brings the panel up, dark. The backlight stays off until
// cs_display_light, so the first thing seen is a picture and not whatever the
// panel's memory held at power-on.
esp_err_t cs_display_init(void);

// cs_display_band returns the one band buffer: CS_SCREEN_W x CS_BAND_ROWS
// pixels, in memory the SPI driver can send from.
uint16_t *cs_display_band(void);

// cs_display_push sends the first `rows` rows of the band buffer to the panel
// at row y, and returns when they have gone. The buffer's contents are spent
// by it: the bytes are swapped in place.
esp_err_t cs_display_push(int y, int rows);

void cs_display_light(void);

// cs_display_sleep turns the backlight off and puts the panel to sleep.
// cs_display_wake wakes the panel, still dark, to be drawn and then lit with
// cs_display_light. Each takes the panel's 120 ms to settle.
void cs_display_sleep(void);
void cs_display_wake(void);

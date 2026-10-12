// The screen as a whole: what it holds, when it draws, what a touch does.
//
// One task owns the panel. It polls for touches, lets ui.c decide what they
// mean, and redraws when something it shows has changed. Everything else -
// the broker client above all - only tells it things, through the functions
// here, and never draws.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"
#include "ui.h"

// cs_command_sender is how a button press leaves the screen. It returns
// false if the command could not be sent.
typedef bool (*cs_command_sender)(cs_button_t button);

// cs_screen_start draws the first picture and starts the task. The panel and
// the touch controller must be up.
esp_err_t cs_screen_start(cs_command_sender send);

// What the broker client reports. All are safe from its task.

void cs_screen_link(cs_link_t link);
void cs_screen_playing(cs_play_t play, bool has_art);

// A cover arrives in pieces: begin, some data, end. While one is arriving the
// screen keeps what it is showing rather than drawing half a change.
//
// cs_screen_cover_begin returns false for a cover too large to hold, and the
// pieces that follow are then ignored. A cover of no bytes is how nowplayd
// says a track's cover could not be made small: the screen then holds none.
bool cs_screen_cover_begin(size_t total);
void cs_screen_cover_data(size_t offset, const uint8_t *data, size_t len);
void cs_screen_cover_end(void);

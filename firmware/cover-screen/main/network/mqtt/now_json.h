// The current track, as much of it as the screen uses.
//
// ESP-IDF glue around cJSON, and so not host-tested; it is kept to reading
// five things out of the message and deciding nothing.
#pragma once

#include <stdbool.h>
#include <stddef.h>

#include "ui.h"

typedef struct {
    cs_play_t play;
    bool has_art; // the track names a cover
    // What is playing takes no commands (ADR-0025). False when the message
    // does not say, which is every message from before it could.
    bool no_controls;
    // The message names a source. None is how nowplayd says nobody is there
    // (ADR-0036); which source it is, the screen does not ask.
    bool has_source;
} cs_now_t;

// cs_now_parse reads a message from the current topic. It returns false for
// anything that is not one, including one of a version this firmware does
// not speak: that is refused, not guessed at.
bool cs_now_parse(const char *json, size_t len, cs_now_t *now);

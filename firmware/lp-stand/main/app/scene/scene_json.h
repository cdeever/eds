// Parsing a scene payload off the wire.
//
// Split from scene.c so that validation stays free of ESP-IDF and cJSON and
// can be tested on the host. Only the extraction below needs a device.
#pragma once

#include <stdbool.h>
#include <stddef.h>

#include "scene.h"

// lp_scene_parse reads a scene descriptor into `out`.
//
// Returns false for anything it cannot make sense of - wrong contract version,
// malformed JSON, no usable colours - leaving `out` untouched. The caller keeps
// rendering whatever it already had: a stand that goes dark on a bad payload is
// worse than one that ignores it.
bool lp_scene_parse(const char *json, size_t length, lp_scene_t *out);

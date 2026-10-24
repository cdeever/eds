// The XIAO's own button and light: the only controls the device has.
#pragma once

#include <stdbool.h>

#include "esp_err.h"

esp_err_t ea_panel_init(void);

// ea_panel_pressed is whether the talk button is held, now.
bool ea_panel_pressed(void);

// The light: on while a request is being heard, and flashed to say what was
// made of it. A flash replaces whatever was showing, and ends with the light
// as ea_panel_light last left it.
void ea_panel_light(bool on);
void ea_panel_flash(int times, int on_ms, int off_ms);

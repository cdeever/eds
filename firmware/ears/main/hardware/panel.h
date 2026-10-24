// The device's controls: the XIAO's own button and light, and its console.
//
// On the reSpeaker Lite voice kit the XIAO is under the board, and its BOOT
// button cannot be reached; the reSpeaker's own USR and MUTE buttons are the
// XMOS chip's, not the XIAO's (found 2026-10-24: neither moves any XIAO
// pin). So the console asks too: Enter on the USB serial line hears five
// seconds. The wake word is what the room uses.
#pragma once

#include <stdbool.h>

#include "esp_err.h"

esp_err_t ea_panel_init(void);

// ea_panel_pressed is whether the talk button is held, now.
bool ea_panel_pressed(void);

// ea_panel_asked is whether Enter has been pressed on the console since it
// was last asked: true once for each.
bool ea_panel_asked(void);

// The light: on while a request is being heard, and flashed to say what was
// made of it. A flash replaces whatever was showing, and ends with the light
// as ea_panel_light last left it.
void ea_panel_light(bool on);
void ea_panel_flash(int times, int on_ms, int off_ms);

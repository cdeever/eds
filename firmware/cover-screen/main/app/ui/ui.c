#include "ui.h"

#include <stddef.h>
#include <stdio.h>
#include <string.h>

// The middle button's reach. Larger than it is drawn: play and pause is the
// one most wanted, and the one a thumb aims at least carefully.
#define CS_CENTER_REACH 52

void cs_ui_init(cs_ui_t *ui)
{
    memset(ui, 0, sizeof(*ui));
    ui->link = CS_LINK_NO_WIFI;
    ui->play = CS_PLAY_STOPPED;
    ui->dirty = true;
}

cs_button_t cs_ui_zone(int x, int y)
{
    int dx = x - CS_SCREEN_W / 2;
    int dy = y - CS_SCREEN_H / 2;

    if (dx * dx + dy * dy <= CS_CENTER_REACH * CS_CENTER_REACH) {
        return CS_BTN_PLAY_PAUSE;
    }

    int ax = dx < 0 ? -dx : dx;
    int ay = dy < 0 ? -dy : dy;
    if (ax >= ay) {
        return dx < 0 ? CS_BTN_PREVIOUS : CS_BTN_NEXT;
    }
    return dy < 0 ? CS_BTN_VOLUME_UP : CS_BTN_VOLUME_DOWN;
}

static bool repeats(cs_button_t button)
{
    return button == CS_BTN_VOLUME_UP || button == CS_BTN_VOLUME_DOWN;
}

cs_button_t cs_ui_touch(cs_ui_t *ui, bool touching, int x, int y, uint32_t now_ms)
{
    if (touching) {
        ui->up_samples = 0;
        if (ui->down_samples < CS_DEBOUNCE_SAMPLES) {
            ui->down_samples++;
        }
    } else {
        ui->down_samples = 0;
        if (ui->up_samples < CS_DEBOUNCE_SAMPLES) {
            ui->up_samples++;
        }
    }

    bool pressed = !ui->down && ui->down_samples >= CS_DEBOUNCE_SAMPLES;
    bool lifted = ui->down && ui->up_samples >= CS_DEBOUNCE_SAMPLES;

    if (lifted) {
        ui->down = false;
        ui->held = CS_BTN_NONE;
        ui->last_touch_ms = now_ms;
        // Whatever this touch was for, the next one is a button.
        ui->armed = ui->overlay;
        return CS_BTN_NONE;
    }

    if (pressed) {
        ui->down = true;
        ui->last_touch_ms = now_ms;

        // Nothing to offer, so nothing is raised and nothing is sent. The
        // touch is still followed, so that one held across a change of track
        // has to lift before it means anything.
        if (ui->no_controls) {
            return CS_BTN_NONE;
        }

        if (!ui->overlay) {
            ui->overlay = true;
            ui->armed = false;
            ui->dirty = true;
            return CS_BTN_NONE;
        }
        if (!ui->armed || ui->link != CS_LINK_UP) {
            return CS_BTN_NONE;
        }

        cs_button_t button = cs_ui_zone(x, y);
        ui->held = button;
        ui->held_since_ms = now_ms;
        ui->last_repeat_ms = now_ms;
        ui->lit = button;
        ui->lit_at_ms = now_ms;
        ui->dirty = true;
        return button;
    }

    if (ui->down) {
        // A finger resting on the screen keeps the buttons up.
        ui->last_touch_ms = now_ms;

        // Unsigned differences throughout, so the clock wrapping after seven
        // weeks is one ordinary repeat and not a stuck button.
        if (repeats(ui->held) && ui->link == CS_LINK_UP && !ui->no_controls &&
            now_ms - ui->held_since_ms >= CS_REPEAT_AFTER_MS &&
            now_ms - ui->last_repeat_ms >= CS_REPEAT_EVERY_MS) {
            ui->last_repeat_ms = now_ms;
            return ui->held;
        }
    }
    return CS_BTN_NONE;
}

void cs_ui_tick(cs_ui_t *ui, uint32_t now_ms)
{
    if (ui->lit != CS_BTN_NONE && ui->held == CS_BTN_NONE && now_ms - ui->lit_at_ms >= CS_FLASH_MS) {
        ui->lit = CS_BTN_NONE;
        ui->dirty = true;
    }
    if (ui->overlay && !ui->down && now_ms - ui->last_touch_ms >= CS_OVERLAY_MS) {
        ui->overlay = false;
        ui->armed = false;
        ui->lit = CS_BTN_NONE;
        ui->dirty = true;
    }
}

void cs_ui_set_link(cs_ui_t *ui, cs_link_t link)
{
    if (ui->link != link) {
        ui->link = link;
        ui->dirty = true;
    }
}

void cs_ui_set_playing(cs_ui_t *ui, cs_play_t play, bool has_art)
{
    if (ui->play != play || ui->has_art != has_art) {
        ui->play = play;
        ui->has_art = has_art;
        ui->dirty = true;
    }
}

void cs_ui_set_controls(cs_ui_t *ui, bool offered)
{
    if (ui->no_controls == !offered) {
        return;
    }
    ui->no_controls = !offered;
    if (!offered) {
        // Any buttons that are up go down now, and a finger on one is let go
        // of: it was pressing something that is no longer there.
        if (ui->overlay || ui->lit != CS_BTN_NONE) {
            ui->dirty = true;
        }
        ui->overlay = false;
        ui->armed = false;
        ui->held = CS_BTN_NONE;
        ui->lit = CS_BTN_NONE;
    }
}

void cs_ui_set_cover(cs_ui_t *ui, bool ready)
{
    // A new cover after another is a change even though "ready" is not.
    if (ready || ui->cover_ready != ready) {
        ui->dirty = true;
    }
    ui->cover_ready = ready;
}

bool cs_ui_take_dirty(cs_ui_t *ui)
{
    bool dirty = ui->dirty;
    ui->dirty = false;
    return dirty;
}

void cs_ui_view(const cs_ui_t *ui, cs_view_t *view)
{
    view->cover = ui->has_art && ui->cover_ready;
    view->overlay = ui->overlay;
    view->pressed = ui->lit;
    view->playing = ui->play == CS_PLAY_PLAYING;
    view->link = ui->link;
}

const char *cs_button_command(cs_button_t button)
{
    switch (button) {
    case CS_BTN_PLAY_PAUSE:
        return "play_pause";
    case CS_BTN_PREVIOUS:
        return "previous";
    case CS_BTN_NEXT:
        return "next";
    case CS_BTN_VOLUME_UP:
        return "volume_up";
    case CS_BTN_VOLUME_DOWN:
        return "volume_down";
    default:
        return NULL;
    }
}

size_t cs_command_format(char *out, size_t cap, cs_button_t button, const char *from)
{
    const char *command = cs_button_command(button);
    if (out == NULL || cap == 0 || command == NULL || from == NULL || from[0] == '\0') {
        return 0;
    }
    // `from` is a device name: lowercase letters, digits and hyphens. Anything
    // else is refused rather than escaped, since it would not be one.
    for (const char *c = from; *c != '\0'; c++) {
        bool ok = (*c >= 'a' && *c <= 'z') || (*c >= '0' && *c <= '9') || *c == '-';
        if (!ok) {
            return 0;
        }
    }

    int written = snprintf(out, cap, "{\"v\":%d,\"command\":\"%s\",\"from\":\"%s\"}",
                           CS_NOWPLAYING_VERSION, command, from);
    if (written <= 0 || (size_t)written >= cap) {
        out[0] = '\0';
        return 0;
    }
    return (size_t)written;
}

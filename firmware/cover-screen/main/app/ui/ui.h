// What the screen is showing and what a touch means.
//
// Pure C with no ESP-IDF dependency, so the part where a bug is "the wrong
// thing happened when I touched it" can be tested with a compiler. Nothing
// here draws, reads the panel or knows the time: the caller passes each touch
// sample and the clock in, and asks for a view to draw.
//
// The rules, all deliberate:
//
//   - The cover is the screen. Nothing is drawn over it until it is touched.
//   - The first touch only brings the buttons up. It sends nothing, and it has
//     to lift before a button will act: a finger that wakes the screen should
//     not also skip the track.
//   - A button acts when it is pressed, not when it is let go, because a
//     resistive panel reports a lift late and a remote that lags feels broken.
//   - Volume repeats while held. The others do not.
//   - The buttons go away by themselves a few seconds after the last touch.
//   - When what is playing has no controls - a record on the turntable,
//     which nothing here can skip - a touch does nothing at all, and the
//     cover stays the whole screen (ADR-0025). Buttons that do nothing are
//     worse than none.
//   - With no broker a press sends nothing, and says so by doing nothing: a
//     command queued for later would skip a track the listener has moved on
//     from.
//   - The screen sleeps when nobody is there: five minutes after the current
//     track last named a source, counted again from each touch (ADR-0036).
//     A source named wakes it. So does a touch, which only wakes it: it
//     raises nothing and sends nothing. So does losing the broker, because a
//     dark screen has to mean asleep, not broken. Until the current track
//     has been heard it is awake.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

// The panel, landscape. The cover is 240 square in the middle of it.
#define CS_SCREEN_W 320
#define CS_SCREEN_H 240

typedef enum {
    CS_BTN_NONE,
    CS_BTN_PLAY_PAUSE,
    CS_BTN_PREVIOUS,
    CS_BTN_NEXT,
    CS_BTN_VOLUME_UP,
    CS_BTN_VOLUME_DOWN,
} cs_button_t;

typedef enum {
    CS_LINK_NO_WIFI,
    CS_LINK_NO_BROKER,
    CS_LINK_UP,
} cs_link_t;

typedef enum {
    CS_PLAY_STOPPED,
    CS_PLAY_PAUSED,
    CS_PLAY_PLAYING,
} cs_play_t;

// Timings, in milliseconds.
#define CS_OVERLAY_MS      5000 // the buttons stay this long after the last touch
#define CS_FLASH_MS        200  // a pressed button stays lit at least this long
#define CS_REPEAT_AFTER_MS 500  // a held volume button starts repeating after this
#define CS_REPEAT_EVERY_MS 250  // and then repeats this often
#define CS_SLEEP_MS (5u * 60u * 1000u) // nobody there this long, and the screen sleeps

// Consecutive samples that must agree before a touch or a lift is believed. A
// resistive panel chatters at both ends of a press.
#define CS_DEBOUNCE_SAMPLES 2

// cs_view_t is everything the drawing code needs to know, and nothing else.
typedef struct {
    bool cover;          // draw the cover; otherwise the record
    bool overlay;        // the buttons are up
    cs_button_t pressed; // the one to draw lit, or CS_BTN_NONE
    bool playing;        // the middle button offers pause rather than play
    cs_link_t link;
} cs_view_t;

typedef struct {
    cs_link_t link;
    cs_play_t play;
    bool has_art;     // the current track names a cover
    bool no_controls; // what is playing takes no commands, so no buttons
    bool cover_ready; // and the screen holds one it can draw
    bool present;     // the current track names a source, or has not been heard

    bool asleep;
    bool counting; // toward sleep, since quiet_since_ms
    uint32_t quiet_since_ms;

    bool overlay;
    bool armed; // the touch that raised the buttons has lifted
    uint32_t last_touch_ms;

    bool down;
    uint8_t down_samples;
    uint8_t up_samples;

    cs_button_t held;
    uint32_t held_since_ms;
    uint32_t last_repeat_ms;

    cs_button_t lit;
    uint32_t lit_at_ms;

    bool dirty;
} cs_ui_t;

void cs_ui_init(cs_ui_t *ui);

// cs_ui_zone is which button a point on the screen belongs to. The whole
// screen is a button: a disc in the middle for play and pause, and the rest
// split along its diagonals into previous, next, volume up and volume down. A
// finger does not have to find the icon.
cs_button_t cs_ui_zone(int x, int y);

// cs_ui_touch takes one sample from the panel: whether it is being touched,
// and where. Call it at a steady rate whether or not anything is touching. It
// returns the button whose command should be sent now, or CS_BTN_NONE.
cs_button_t cs_ui_touch(cs_ui_t *ui, bool touching, int x, int y, uint32_t now_ms);

// cs_ui_tick lets time pass: the buttons going away, a lit button going out,
// the screen going to sleep.
void cs_ui_tick(cs_ui_t *ui, uint32_t now_ms);

void cs_ui_set_link(cs_ui_t *ui, cs_link_t link);
// cs_ui_set_playing is what the current track says: its state, and whether it
// names a cover.
void cs_ui_set_playing(cs_ui_t *ui, cs_play_t play, bool has_art);
// cs_ui_set_controls says whether what is playing can be told anything. It is
// what the current track says of itself, and until it says otherwise the
// buttons are offered: a message with no word on it is from before there was
// one. Taking them away takes down any that are up.
void cs_ui_set_controls(cs_ui_t *ui, bool offered);
// cs_ui_set_source says whether the current track names a source. None named
// is how nowplayd says nobody is there: every source offline or switched off.
void cs_ui_set_source(cs_ui_t *ui, bool named);
// cs_ui_set_cover says whether the screen holds a cover it can draw. Call it
// with true for each new one, even after another: the picture has changed.
void cs_ui_set_cover(cs_ui_t *ui, bool ready);

// cs_ui_take_dirty reports whether the screen needs drawing again, and
// forgets that it did.
bool cs_ui_take_dirty(cs_ui_t *ui);

void cs_ui_view(const cs_ui_t *ui, cs_view_t *view);

// cs_ui_asleep says whether the panel should be dark. What changes while it
// is stays dirty, to be drawn on waking.
bool cs_ui_asleep(const cs_ui_t *ui);

// cs_button_command is the word for a button in the now-playing command
// contract, or NULL for CS_BTN_NONE.
const char *cs_button_command(cs_button_t button);

// cs_command_format writes the command for a button as it goes on the bus:
//
//   {"v":1,"command":"next","from":"cover-screen-01"}
//
// It returns the length, or 0 if there is no such command, it does not fit,
// or `from` holds something that would need escaping.
size_t cs_command_format(char *out, size_t cap, cs_button_t button, const char *from);

// The version of the now-playing contract this firmware speaks. A message
// with another is refused, not guessed at.
#define CS_NOWPLAYING_VERSION 1

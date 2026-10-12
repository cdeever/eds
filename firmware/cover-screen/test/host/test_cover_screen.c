// Host tests for the hardware-independent half of the firmware.
//
// What a touch means, what is drawn over the cover, and where a touch is are
// pure C with no ESP-IDF dependency, so they can be tested with a compiler
// and no hardware.
//
// The ESP-IDF glue - the panel, the touch controller, the JPEG decoder, WiFi,
// MQTT - is not covered here and needs a device.

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "draw.h"
#include "event_format.h"
#include "touch_map.h"
#include "ui.h"

static int failures = 0;
static int checks = 0;

#define CHECK(cond, ...)                                                       \
    do {                                                                       \
        checks++;                                                              \
        if (!(cond)) {                                                         \
            failures++;                                                        \
            printf("  FAIL %s:%d: ", __func__, __LINE__);                      \
            printf(__VA_ARGS__);                                               \
            printf("\n");                                                      \
        }                                                                      \
    } while (0)

// --- touches -----------------------------------------------------------------

#define POLL_MS 20

// The middle of each button, as it is drawn.
#define MID_X CS_BTN_CENTRE_X
#define MID_Y CS_BTN_CENTRE_Y
#define PREV_X (MID_X - CS_BTN_SIDE_DX)
#define NEXT_X (MID_X + CS_BTN_SIDE_DX)
#define UP_Y (MID_Y - CS_BTN_VOLUME_DY)
#define DOWN_Y (MID_Y + CS_BTN_VOLUME_DY)

static uint32_t now;

// A screen that is connected and playing, as most tests want it.
static cs_ui_t connected(void)
{
    cs_ui_t ui;
    cs_ui_init(&ui);
    cs_ui_set_link(&ui, CS_LINK_UP);
    cs_ui_set_playing(&ui, CS_PLAY_PLAYING, true);
    cs_ui_set_cover(&ui, true);
    cs_ui_take_dirty(&ui);
    now = 1000;
    return ui;
}

// sample feeds one reading and lets time pass, as the panel task does. It
// returns what was sent.
static cs_button_t sample(cs_ui_t *ui, bool touching, int x, int y)
{
    cs_button_t sent = cs_ui_touch(ui, touching, x, y, now);
    cs_ui_tick(ui, now);
    now += POLL_MS;
    return sent;
}

// hold touches a point for a while and counts what was sent, checking it was
// only ever `want`.
static int hold(cs_ui_t *ui, int x, int y, uint32_t ms, cs_button_t want)
{
    int sent = 0;
    for (uint32_t t = 0; t < ms; t += POLL_MS) {
        cs_button_t b = sample(ui, true, x, y);
        if (b != CS_BTN_NONE) {
            sent++;
            if (b != want) {
                return -1;
            }
        }
    }
    return sent;
}

static void rest(cs_ui_t *ui, uint32_t ms)
{
    for (uint32_t t = 0; t < ms; t += POLL_MS) {
        sample(ui, false, 0, 0);
    }
}

// tap is a short press and a lift.
static int tap(cs_ui_t *ui, int x, int y, cs_button_t want)
{
    int sent = hold(ui, x, y, 100, want);
    rest(ui, 100);
    return sent;
}

static void test_zones_cover_the_whole_screen(void)
{
    CHECK(cs_ui_zone(MID_X, MID_Y) == CS_BTN_PLAY_PAUSE, "the middle");
    CHECK(cs_ui_zone(PREV_X, MID_Y) == CS_BTN_PREVIOUS, "the previous button");
    CHECK(cs_ui_zone(NEXT_X, MID_Y) == CS_BTN_NEXT, "the next button");
    CHECK(cs_ui_zone(MID_X, UP_Y) == CS_BTN_VOLUME_UP, "the volume up button");
    CHECK(cs_ui_zone(MID_X, DOWN_Y) == CS_BTN_VOLUME_DOWN, "the volume down button");

    // The edges, where a finger that missed the icon lands.
    CHECK(cs_ui_zone(0, MID_Y) == CS_BTN_PREVIOUS, "the left edge");
    CHECK(cs_ui_zone(CS_SCREEN_W - 1, MID_Y) == CS_BTN_NEXT, "the right edge");
    CHECK(cs_ui_zone(MID_X, 0) == CS_BTN_VOLUME_UP, "the top edge");
    CHECK(cs_ui_zone(MID_X, CS_SCREEN_H - 1) == CS_BTN_VOLUME_DOWN, "the bottom edge");

    for (int y = 0; y < CS_SCREEN_H; y++) {
        for (int x = 0; x < CS_SCREEN_W; x++) {
            if (cs_ui_zone(x, y) == CS_BTN_NONE) {
                CHECK(false, "(%d,%d) belongs to no button", x, y);
                return;
            }
        }
    }
}

static void test_the_first_touch_only_brings_the_buttons_up(void)
{
    cs_ui_t ui = connected();
    cs_view_t view;

    cs_ui_view(&ui, &view);
    CHECK(!view.overlay, "the buttons are up before any touch");

    // Right on the next button, and held: it must still do nothing.
    CHECK(hold(&ui, NEXT_X, MID_Y, 2000, CS_BTN_NEXT) == 0, "the touch that woke the screen sent a command");
    cs_ui_view(&ui, &view);
    CHECK(view.overlay, "a touch did not bring the buttons up");
    CHECK(view.pressed == CS_BTN_NONE, "the waking touch lit a button");
    CHECK(cs_ui_take_dirty(&ui), "raising the buttons did not ask for a redraw");

    rest(&ui, 100);
    CHECK(tap(&ui, NEXT_X, MID_Y, CS_BTN_NEXT) == 1, "the second touch did not send next");
}

static void test_a_button_acts_once_when_pressed(void)
{
    struct {
        int x, y;
        cs_button_t button;
    } cases[] = {
        {MID_X, MID_Y, CS_BTN_PLAY_PAUSE},
        {PREV_X, MID_Y, CS_BTN_PREVIOUS},
        {NEXT_X, MID_Y, CS_BTN_NEXT},
    };
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        cs_ui_t ui = connected();
        tap(&ui, MID_X, MID_Y, CS_BTN_NONE);

        // Held for three seconds: one command, at the start.
        cs_button_t first = CS_BTN_NONE;
        for (int n = 0; n < 4 && first == CS_BTN_NONE; n++) {
            first = sample(&ui, true, cases[i].x, cases[i].y);
        }
        CHECK(first == cases[i].button, "%s was not sent on the press", cs_button_command(cases[i].button));
        CHECK(hold(&ui, cases[i].x, cases[i].y, 3000, cases[i].button) == 0, "%s repeated while held",
              cs_button_command(cases[i].button));
    }
}

static void test_volume_repeats_while_held(void)
{
    cs_ui_t ui = connected();
    tap(&ui, MID_X, MID_Y, CS_BTN_NONE);

    // One on the press, none for half a second, then four a second.
    int sent = hold(&ui, MID_X, UP_Y, 400, CS_BTN_VOLUME_UP);
    CHECK(sent == 1, "%d volume steps in the first 400 ms", sent);
    sent = hold(&ui, MID_X, UP_Y, 2000, CS_BTN_VOLUME_UP);
    CHECK(sent >= 7 && sent <= 9, "%d volume steps in two seconds of holding", sent);

    rest(&ui, 100);
    CHECK(hold(&ui, MID_X, DOWN_Y, 1500, CS_BTN_VOLUME_DOWN) >= 4, "volume down did not repeat");
}

static void test_a_flicker_is_not_a_touch(void)
{
    cs_ui_t ui = connected();
    cs_view_t view;

    // One sample of contact, as a resistive panel gives when it is brushed.
    for (int i = 0; i < 20; i++) {
        sample(&ui, true, NEXT_X, MID_Y);
        sample(&ui, false, 0, 0);
        sample(&ui, false, 0, 0);
    }
    cs_ui_view(&ui, &view);
    CHECK(!view.overlay, "single samples raised the buttons");

    // And one sample of nothing in the middle of a press is not a lift: the
    // waking touch must not turn into a button press through a dropout.
    sample(&ui, true, NEXT_X, MID_Y);
    sample(&ui, true, NEXT_X, MID_Y);
    sample(&ui, false, 0, 0);
    CHECK(hold(&ui, NEXT_X, MID_Y, 500, CS_BTN_NEXT) == 0, "a dropout made the waking touch a press");
}

static void test_the_buttons_go_away_by_themselves(void)
{
    cs_ui_t ui = connected();
    cs_view_t view;

    tap(&ui, MID_X, MID_Y, CS_BTN_NONE);
    cs_ui_take_dirty(&ui);

    rest(&ui, CS_OVERLAY_MS - 500);
    cs_ui_view(&ui, &view);
    CHECK(view.overlay, "the buttons went before their time");

    rest(&ui, 1000);
    cs_ui_view(&ui, &view);
    CHECK(!view.overlay, "the buttons stayed up");
    CHECK(cs_ui_take_dirty(&ui), "the buttons going did not ask for a redraw");

    // And the next touch only wakes it again.
    CHECK(tap(&ui, NEXT_X, MID_Y, CS_BTN_NEXT) == 0, "a touch on a sleeping screen sent a command");
}

static void test_a_resting_finger_keeps_the_buttons_up(void)
{
    cs_ui_t ui = connected();
    cs_view_t view;

    hold(&ui, MID_X, MID_Y, CS_OVERLAY_MS * 3, CS_BTN_NONE);
    cs_ui_view(&ui, &view);
    CHECK(view.overlay, "the buttons went while the screen was being touched");

    // Each press starts the wait again.
    rest(&ui, 100);
    for (int i = 0; i < 6; i++) {
        tap(&ui, MID_X, UP_Y, CS_BTN_VOLUME_UP);
        rest(&ui, CS_OVERLAY_MS / 2);
    }
    cs_ui_view(&ui, &view);
    CHECK(view.overlay, "pressing buttons did not keep them up");
}

static void test_a_pressed_button_is_lit_briefly(void)
{
    cs_ui_t ui = connected();
    cs_view_t view;
    tap(&ui, MID_X, MID_Y, CS_BTN_NONE);
    cs_ui_take_dirty(&ui);

    hold(&ui, NEXT_X, MID_Y, 60, CS_BTN_NEXT);
    cs_ui_view(&ui, &view);
    CHECK(view.pressed == CS_BTN_NEXT, "the pressed button is not lit");
    CHECK(cs_ui_take_dirty(&ui), "lighting a button did not ask for a redraw");

    // Still lit while it is held, however long.
    hold(&ui, NEXT_X, MID_Y, 1000, CS_BTN_NEXT);
    cs_ui_view(&ui, &view);
    CHECK(view.pressed == CS_BTN_NEXT, "the button went out while still held");

    rest(&ui, CS_FLASH_MS + 100);
    cs_ui_view(&ui, &view);
    CHECK(view.pressed == CS_BTN_NONE, "the button stayed lit after the lift");
    CHECK(cs_ui_take_dirty(&ui), "the button going out did not ask for a redraw");
}

static void test_nothing_is_sent_without_a_broker(void)
{
    cs_link_t links[] = {CS_LINK_NO_WIFI, CS_LINK_NO_BROKER};
    for (size_t i = 0; i < 2; i++) {
        cs_ui_t ui = connected();
        cs_view_t view;
        cs_ui_set_link(&ui, links[i]);

        tap(&ui, MID_X, MID_Y, CS_BTN_NONE);
        cs_ui_view(&ui, &view);
        CHECK(view.overlay, "the buttons do not come up without a broker");

        CHECK(tap(&ui, NEXT_X, MID_Y, CS_BTN_NEXT) == 0, "next was sent with no broker");
        CHECK(hold(&ui, MID_X, UP_Y, 2000, CS_BTN_VOLUME_UP) == 0, "volume was sent with no broker");
        cs_ui_view(&ui, &view);
        CHECK(view.pressed == CS_BTN_NONE, "a button that did nothing was lit");
    }

    // A broker that goes away mid-hold stops the repeat.
    cs_ui_t ui = connected();
    tap(&ui, MID_X, MID_Y, CS_BTN_NONE);
    hold(&ui, MID_X, UP_Y, 1000, CS_BTN_VOLUME_UP);
    cs_ui_set_link(&ui, CS_LINK_NO_BROKER);
    CHECK(hold(&ui, MID_X, UP_Y, 2000, CS_BTN_VOLUME_UP) == 0, "volume kept repeating after the broker went");
}

static void test_touches_survive_the_clock_wrapping(void)
{
    cs_ui_t ui = connected();
    cs_view_t view;

    // Three seconds before a 32-bit millisecond clock wraps, at 49.7 days.
    now = UINT32_MAX - 3000;
    tap(&ui, MID_X, MID_Y, CS_BTN_NONE);

    int sent = hold(&ui, MID_X, UP_Y, 6000, CS_BTN_VOLUME_UP);
    CHECK(sent >= 20 && sent <= 24, "%d volume steps across the wrap", sent);
    CHECK(now < 10000, "the test did not cross the wrap");

    rest(&ui, CS_OVERLAY_MS + 500);
    cs_ui_view(&ui, &view);
    CHECK(!view.overlay, "the buttons stayed up after the clock wrapped");
}

static void test_the_view_follows_what_is_playing(void)
{
    cs_ui_t ui;
    cs_view_t view;
    cs_ui_init(&ui);

    cs_ui_view(&ui, &view);
    CHECK(!view.cover && !view.playing && view.link == CS_LINK_NO_WIFI, "a fresh screen shows something");
    CHECK(cs_ui_take_dirty(&ui), "a fresh screen does not ask to be drawn");
    CHECK(!cs_ui_take_dirty(&ui), "dirty was not forgotten");

    // A cover held, but the track names none: the record, not a stale cover.
    cs_ui_set_cover(&ui, true);
    cs_ui_set_playing(&ui, CS_PLAY_PLAYING, false);
    cs_ui_view(&ui, &view);
    CHECK(!view.cover && view.playing, "a cover was shown for a track that names none");

    // The track names one and the screen holds one.
    cs_ui_take_dirty(&ui);
    cs_ui_set_playing(&ui, CS_PLAY_PLAYING, true);
    cs_ui_view(&ui, &view);
    CHECK(view.cover, "the cover is not shown");
    CHECK(cs_ui_take_dirty(&ui), "a change of track did not ask for a redraw");

    // Paused keeps the cover and offers play.
    cs_ui_set_playing(&ui, CS_PLAY_PAUSED, true);
    cs_ui_view(&ui, &view);
    CHECK(view.cover && !view.playing, "pausing lost the cover or kept offering pause");

    // Saying the same thing again is not a change.
    cs_ui_take_dirty(&ui);
    cs_ui_set_playing(&ui, CS_PLAY_PAUSED, true);
    cs_ui_set_link(&ui, CS_LINK_NO_WIFI);
    CHECK(!cs_ui_take_dirty(&ui), "a repeat asked for a redraw");

    // A new cover is, even though one was already held.
    cs_ui_set_cover(&ui, true);
    CHECK(cs_ui_take_dirty(&ui), "a new cover did not ask for a redraw");

    // The track names a cover the screen could not draw.
    cs_ui_set_cover(&ui, false);
    cs_ui_view(&ui, &view);
    CHECK(!view.cover, "a cover that is not held was shown");
}

// The wire form is the contract with nowplayd. If this changes, so must the
// Go side (services/nowplaying/internal/contract).
static void test_a_command_is_what_the_contract_says(void)
{
    char out[96];
    struct {
        cs_button_t button;
        const char *want;
    } cases[] = {
        {CS_BTN_PLAY_PAUSE, "{\"v\":1,\"command\":\"play_pause\",\"from\":\"cover-screen-01\"}"},
        {CS_BTN_PREVIOUS, "{\"v\":1,\"command\":\"previous\",\"from\":\"cover-screen-01\"}"},
        {CS_BTN_NEXT, "{\"v\":1,\"command\":\"next\",\"from\":\"cover-screen-01\"}"},
        {CS_BTN_VOLUME_UP, "{\"v\":1,\"command\":\"volume_up\",\"from\":\"cover-screen-01\"}"},
        {CS_BTN_VOLUME_DOWN, "{\"v\":1,\"command\":\"volume_down\",\"from\":\"cover-screen-01\"}"},
    };
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        size_t len = cs_command_format(out, sizeof(out), cases[i].button, "cover-screen-01");
        CHECK(len == strlen(cases[i].want) && strcmp(out, cases[i].want) == 0, "got %s", out);
    }
}

static void test_a_command_that_cannot_be_written_is_refused(void)
{
    char out[96] = "untouched";
    CHECK(cs_command_format(out, sizeof(out), CS_BTN_NONE, "cover-screen-01") == 0, "no button made a command");
    CHECK(cs_command_format(out, sizeof(out), CS_BTN_NEXT, "") == 0, "an empty name was accepted");
    CHECK(cs_command_format(out, sizeof(out), CS_BTN_NEXT, NULL) == 0, "no name was accepted");
    CHECK(cs_command_format(out, sizeof(out), CS_BTN_NEXT, "a\"b") == 0, "a quote in the name was accepted");
    CHECK(cs_command_format(out, sizeof(out), CS_BTN_NEXT, "Cover Screen") == 0, "a name that is not one was accepted");
    CHECK(cs_command_format(out, 20, CS_BTN_NEXT, "cover-screen-01") == 0, "a command was cut to fit");
    CHECK(cs_command_format(NULL, 0, CS_BTN_NEXT, "cover-screen-01") == 0, "no buffer");
    CHECK(cs_button_command(CS_BTN_NONE) == NULL, "no button has a word");
}

// --- where a touch is --------------------------------------------------------

static void test_touch_maps_corner_to_corner(void)
{
    cs_touch_cal_t cal = {.x_min = 200, .x_max = 3700, .y_min = 240, .y_max = 3800};
    int x, y;

    cs_touch_map(&cal, 200, 240, &x, &y);
    CHECK(x == 0 && y == 0, "one corner maps to (%d,%d)", x, y);
    cs_touch_map(&cal, 3700, 3800, &x, &y);
    CHECK(x == CS_SCREEN_W - 1 && y == CS_SCREEN_H - 1, "the other maps to (%d,%d)", x, y);
    cs_touch_map(&cal, 1950, 2020, &x, &y);
    CHECK(abs(x - CS_SCREEN_W / 2) <= 1 && abs(y - CS_SCREEN_H / 2) <= 1, "the middle maps to (%d,%d)", x, y);

    // Past the glass is the edge, not off the screen.
    cs_touch_map(&cal, 0, 4095, &x, &y);
    CHECK(x == 0 && y == CS_SCREEN_H - 1, "a reading past the edge maps to (%d,%d)", x, y);
}

static void test_touch_axes_can_be_turned(void)
{
    cs_touch_cal_t cal = {.x_min = 200, .x_max = 3700, .y_min = 240, .y_max = 3800};
    int x, y;

    cal.invert_x = true;
    cs_touch_map(&cal, 200, 240, &x, &y);
    CHECK(x == CS_SCREEN_W - 1 && y == 0, "inverted x maps to (%d,%d)", x, y);

    cal.invert_x = false;
    cal.invert_y = true;
    cs_touch_map(&cal, 200, 240, &x, &y);
    CHECK(x == 0 && y == CS_SCREEN_H - 1, "inverted y maps to (%d,%d)", x, y);

    // Swapped: what the controller calls y runs across the screen.
    cal.invert_y = false;
    cal.swap_xy = true;
    cs_touch_map(&cal, 240, 3700, &x, &y);
    CHECK(x == CS_SCREEN_W - 1 && y == 0, "swapped axes map to (%d,%d)", x, y);

    // A calibration with no span must not divide by it.
    cs_touch_cal_t flat = {0};
    cs_touch_map(&flat, 2000, 2000, &x, &y);
    CHECK(x == 0 && y == 0, "a flat calibration maps to (%d,%d)", x, y);
}

// --- what is drawn -----------------------------------------------------------

static uint16_t frame[CS_SCREEN_W * CS_SCREEN_H];
static uint16_t other[CS_SCREEN_W * CS_SCREEN_H];

// A made-up cover: four quarters and a diagonal, bright, with black bars
// either side, as a real one sits on the panel.
static void fake_cover(int y0, int rows, uint16_t *band)
{
    cs_draw_black(rows, band);
    int left = (CS_SCREEN_W - CS_COVER_MAX) / 2;
    for (int row = 0; row < rows; row++) {
        int y = y0 + row;
        for (int cx = 0; cx < CS_COVER_MAX; cx++) {
            uint8_t r = (uint8_t)(40 + cx * 215 / CS_COVER_MAX);
            uint8_t g = (uint8_t)(230 - y * 170 / CS_SCREEN_H);
            uint8_t b = (uint8_t)((cx / 30 + y / 30) % 2 ? 210 : 90);
            if (abs(cx - y) < 14) {
                r = 250, g = 244, b = 232;
            }
            band[row * CS_SCREEN_W + left + cx] = cs_rgb565(r, g, b);
        }
    }
}

// render draws a whole screen into out, in bands of the given height, the
// way the device does.
static void render(const cs_view_t *view, int band_rows, uint16_t *out)
{
    for (int y = 0; y < CS_SCREEN_H; y += band_rows) {
        int rows = y + band_rows > CS_SCREEN_H ? CS_SCREEN_H - y : band_rows;
        uint16_t *band = out + y * CS_SCREEN_W;
        if (view->cover) {
            fake_cover(y, rows, band);
        } else {
            cs_draw_backdrop(view, y, rows, band);
        }
        cs_draw_chrome(view, y, rows, band);
    }
}

static int luma(uint16_t p)
{
    int r = ((p >> 11) & 0x1F) << 3, g = ((p >> 5) & 0x3F) << 2, b = (p & 0x1F) << 3;
    return (r * 3 + g * 6 + b) / 10;
}

static uint16_t at(const uint16_t *f, int x, int y)
{
    return f[y * CS_SCREEN_W + x];
}

static void test_the_cover_is_left_alone_until_touched(void)
{
    cs_view_t view = {.cover = true, .link = CS_LINK_UP, .playing = true};
    render(&view, CS_BAND_ROWS, frame);
    for (int y = 0; y < CS_SCREEN_H; y += CS_BAND_ROWS) {
        fake_cover(y, CS_BAND_ROWS, other + y * CS_SCREEN_W);
    }
    CHECK(memcmp(frame, other, sizeof(frame)) == 0, "something was drawn over an untouched cover");
}

static void test_the_buttons_dim_the_cover_and_show(void)
{
    cs_view_t view = {.cover = true, .link = CS_LINK_UP, .playing = true};
    render(&view, CS_BAND_ROWS, other);
    view.overlay = true;
    render(&view, CS_BAND_ROWS, frame);

    // Clear of every button, the cover is still there and darker.
    int x = 70, y = 40;
    CHECK(luma(at(frame, x, y)) < luma(at(other, x, y)) / 2, "the cover was not dimmed");
    CHECK(luma(at(frame, x, y)) > 8, "the cover was dimmed to nothing");

    // Each icon is white where its ink is.
    CHECK(luma(at(frame, MID_X - 10, MID_Y)) > 220, "no pause bar in the middle");
    CHECK(luma(at(frame, MID_X, MID_Y)) < 60, "pause has no gap between its bars");
    CHECK(luma(at(frame, PREV_X + 4, MID_Y)) > 220, "no previous icon");
    CHECK(luma(at(frame, NEXT_X - 4, MID_Y)) > 220, "no next icon");
    CHECK(luma(at(frame, MID_X, UP_Y - 8)) > 220, "no plus above");
    CHECK(luma(at(frame, MID_X + 8, DOWN_Y)) > 220, "no minus below");
    CHECK(luma(at(frame, MID_X, DOWN_Y - 8)) < 60, "the minus is a plus");

    // Paused, the middle offers play: ink where pause has its gap.
    view.playing = false;
    render(&view, CS_BAND_ROWS, frame);
    CHECK(luma(at(frame, MID_X, MID_Y)) > 220, "no play triangle when paused");
}

static void test_the_icons_point_the_right_way(void)
{
    cs_view_t view = {.cover = true, .overlay = true, .link = CS_LINK_UP};
    render(&view, CS_BAND_ROWS, frame);

    // A triangle is tall at its base and nothing at its point. Count the ink
    // in a column near each end.
    int ink[4] = {0};
    int columns[4] = {PREV_X + 10, PREV_X - 5, NEXT_X - 10, NEXT_X + 5};
    for (int i = 0; i < 4; i++) {
        for (int y = MID_Y - 14; y <= MID_Y + 14; y++) {
            ink[i] += luma(at(frame, columns[i], y)) > 200;
        }
    }
    CHECK(ink[0] > ink[1] + 8, "previous does not point left (%d, %d)", ink[0], ink[1]);
    CHECK(ink[2] > ink[3] + 8, "next does not point right (%d, %d)", ink[2], ink[3]);

    // Play points right.
    int base = 0, point = 0;
    for (int y = MID_Y - 20; y <= MID_Y + 20; y++) {
        base += luma(at(frame, MID_X - 8, y)) > 200;
        point += luma(at(frame, MID_X + 17, y)) > 200;
    }
    CHECK(base > point + 15, "play does not point right (%d, %d)", base, point);
}

static void test_a_pressed_button_is_drawn_lit(void)
{
    cs_button_t buttons[] = {CS_BTN_PLAY_PAUSE, CS_BTN_PREVIOUS, CS_BTN_NEXT, CS_BTN_VOLUME_UP, CS_BTN_VOLUME_DOWN};
    // A point on each plate, clear of its icon.
    int points[][2] = {{MID_X, MID_Y - 34}, {PREV_X, MID_Y - 22}, {NEXT_X, MID_Y - 22},
                       {MID_X - 18, UP_Y}, {MID_X, DOWN_Y - 18}};

    cs_view_t view = {.cover = true, .overlay = true, .link = CS_LINK_UP, .playing = true};
    render(&view, CS_BAND_ROWS, other);

    for (size_t i = 0; i < 5; i++) {
        view.pressed = buttons[i];
        render(&view, CS_BAND_ROWS, frame);
        for (size_t j = 0; j < 5; j++) {
            int before = luma(at(other, points[j][0], points[j][1]));
            int after = luma(at(frame, points[j][0], points[j][1]));
            if (i == j) {
                CHECK(after > before + 40, "%s is not lit when pressed (%d to %d)", cs_button_command(buttons[i]),
                      before, after);
            } else {
                CHECK(after == before, "pressing %s changed %s", cs_button_command(buttons[i]),
                      cs_button_command(buttons[j]));
            }
        }
    }
}

// The device draws in whatever bands the JPEG's blocks come in. The picture
// must not depend on where the bands fall.
static void test_the_picture_does_not_depend_on_the_bands(void)
{
    cs_view_t views[] = {
        {.cover = true, .overlay = true, .link = CS_LINK_UP, .playing = true, .pressed = CS_BTN_NEXT},
        {.cover = false, .overlay = true, .link = CS_LINK_NO_BROKER},
        {.cover = false, .link = CS_LINK_NO_WIFI, .playing = true},
    };
    for (size_t v = 0; v < sizeof(views) / sizeof(views[0]); v++) {
        render(&views[v], CS_SCREEN_H, other);
        int heights[] = {CS_BAND_ROWS, 8, 7, 1};
        for (size_t i = 0; i < sizeof(heights) / sizeof(heights[0]); i++) {
            render(&views[v], heights[i], frame);
            CHECK(memcmp(frame, other, sizeof(frame)) == 0, "view %u drawn in bands of %d differs", (unsigned)v,
                  heights[i]);
        }
    }
}

static void test_no_cover_shows_a_record(void)
{
    cs_view_t view = {.link = CS_LINK_UP};
    render(&view, CS_BAND_ROWS, frame);

    uint16_t ground = at(frame, 4, 4);
    CHECK(luma(ground) < 40, "the ground is not dark");
    CHECK(at(frame, MID_X + 70, MID_Y) != ground, "there is no record");
    CHECK(at(frame, MID_X, MID_Y) == ground, "the record has no hole");
    CHECK(at(frame, CS_SCREEN_W - 5, CS_SCREEN_H - 5) == ground, "the record fills the screen");

    // The label is lit while something plays.
    uint16_t idle = at(frame, MID_X + 16, MID_Y);
    view.playing = true;
    render(&view, CS_BAND_ROWS, frame);
    uint16_t playing = at(frame, MID_X + 16, MID_Y);
    CHECK(playing != idle && luma(playing) > luma(idle), "the label does not light when playing");
}

static void test_a_missing_broker_is_marked(void)
{
    cs_view_t view = {.cover = true, .link = CS_LINK_UP};
    render(&view, CS_BAND_ROWS, other);
    int mx = CS_SCREEN_W - 13, my = 13;

    view.link = CS_LINK_NO_WIFI;
    render(&view, CS_BAND_ROWS, frame);
    uint16_t red = at(frame, mx, my);
    CHECK(red != at(other, mx, my), "no mark without WiFi");
    CHECK((red >> 11) > 20 && ((red >> 5) & 0x3F) < 24, "the no-WiFi mark is not red (%04x)", red);

    view.link = CS_LINK_NO_BROKER;
    render(&view, CS_BAND_ROWS, frame);
    uint16_t amber = at(frame, mx, my);
    CHECK(amber != red && ((amber >> 5) & 0x3F) > 30, "the no-broker mark is not amber (%04x)", amber);

    // And the mark is all that changed.
    frame[0] = other[0];
    int changed = 0;
    for (int i = 0; i < CS_SCREEN_W * CS_SCREEN_H; i++) {
        changed += frame[i] != other[i];
    }
    CHECK(changed > 50 && changed < 300, "%d pixels changed for the mark", changed);

    // With the buttons up and no broker, the icons are drawn faint.
    view.overlay = true;
    render(&view, CS_BAND_ROWS, frame);
    CHECK(luma(at(frame, NEXT_X - 4, MID_Y)) < 140, "the icons look live with no broker");
}

// --- events ------------------------------------------------------------------

static void test_an_event_is_the_line_the_store_expects(void)
{
    char line[CS_EVENT_LINE_LEN];
    size_t len = cs_event_format(line, sizeof(line), CS_EVENT_INFO, "command.sent", "Sent next", 3525, 0,
                                 "\"command\":\"next\"");
    const char *want = "{\"level\":\"info\",\"event\":\"command.sent\",\"msg\":\"Sent next\","
                       "\"uptime_ms\":3525,\"command\":\"next\"}";
    CHECK(len == strlen(want) && strcmp(line, want) == 0, "got %s", line);

    len = cs_event_format(line, sizeof(line), CS_EVENT_WARN, "cover.rejected", "a \"quoted\" reason", 1, 2, NULL);
    CHECK(len > 0 && strstr(line, "\\\"quoted\\\"") != NULL && strstr(line, "\"dropped\":2") != NULL, "got %s", line);

    CHECK(cs_event_format(line, 16, CS_EVENT_INFO, "system.boot", "Started", 0, 0, NULL) == 0,
          "an event that does not fit was cut");
}

// --- pictures ----------------------------------------------------------------

// A 24-bit BMP, which anything opens. Rows are stored bottom up.
static int write_bmp(const char *dir, const char *name, const uint16_t *pixels)
{
    char path[512];
    snprintf(path, sizeof(path), "%s/%s.bmp", dir, name);
    FILE *f = fopen(path, "wb");
    if (f == NULL) {
        perror(path);
        return 1;
    }
    uint32_t size = 54 + CS_SCREEN_W * CS_SCREEN_H * 3;
    uint8_t header[54] = {'B', 'M'};
    header[2] = (uint8_t)size, header[3] = (uint8_t)(size >> 8), header[4] = (uint8_t)(size >> 16);
    header[10] = 54, header[14] = 40;
    header[18] = (uint8_t)CS_SCREEN_W, header[19] = (uint8_t)(CS_SCREEN_W >> 8);
    header[22] = (uint8_t)CS_SCREEN_H, header[23] = (uint8_t)(CS_SCREEN_H >> 8);
    header[26] = 1, header[28] = 24;
    fwrite(header, 1, sizeof(header), f);
    for (int y = CS_SCREEN_H - 1; y >= 0; y--) {
        for (int x = 0; x < CS_SCREEN_W; x++) {
            uint16_t p = pixels[y * CS_SCREEN_W + x];
            uint8_t r5 = (p >> 11) & 0x1F, g6 = (p >> 5) & 0x3F, b5 = p & 0x1F;
            uint8_t bgr[3] = {(uint8_t)((b5 << 3) | (b5 >> 2)), (uint8_t)((g6 << 2) | (g6 >> 4)),
                              (uint8_t)((r5 << 3) | (r5 >> 2))};
            fwrite(bgr, 1, 3, f);
        }
    }
    fclose(f);
    printf("wrote %s\n", path);
    return 0;
}

static int pictures(const char *dir)
{
    struct {
        const char *name;
        cs_view_t view;
    } shots[] = {
        {"1-cover", {.cover = true, .link = CS_LINK_UP, .playing = true}},
        {"2-buttons-playing", {.cover = true, .overlay = true, .link = CS_LINK_UP, .playing = true}},
        {"3-buttons-paused-next-pressed",
         {.cover = true, .overlay = true, .link = CS_LINK_UP, .pressed = CS_BTN_NEXT}},
        {"4-no-cover-playing", {.link = CS_LINK_UP, .playing = true}},
        {"5-no-cover-buttons", {.overlay = true, .link = CS_LINK_UP}},
        {"6-no-broker-buttons", {.cover = true, .overlay = true, .link = CS_LINK_NO_BROKER, .playing = true}},
        {"7-no-wifi", {.link = CS_LINK_NO_WIFI}},
    };
    int failed = 0;
    for (size_t i = 0; i < sizeof(shots) / sizeof(shots[0]); i++) {
        render(&shots[i].view, CS_BAND_ROWS, frame);
        failed |= write_bmp(dir, shots[i].name, frame);
    }
    return failed;
}

int main(int argc, char **argv)
{
    if (argc == 3 && strcmp(argv[1], "--pictures") == 0) {
        return pictures(argv[2]);
    }

    struct {
        const char *name;
        void (*fn)(void);
    } tests[] = {
        {"zones cover the whole screen", test_zones_cover_the_whole_screen},
        {"the first touch only brings the buttons up", test_the_first_touch_only_brings_the_buttons_up},
        {"a button acts once when pressed", test_a_button_acts_once_when_pressed},
        {"volume repeats while held", test_volume_repeats_while_held},
        {"a flicker is not a touch", test_a_flicker_is_not_a_touch},
        {"the buttons go away by themselves", test_the_buttons_go_away_by_themselves},
        {"a resting finger keeps the buttons up", test_a_resting_finger_keeps_the_buttons_up},
        {"a pressed button is lit briefly", test_a_pressed_button_is_lit_briefly},
        {"nothing is sent without a broker", test_nothing_is_sent_without_a_broker},
        {"touches survive the clock wrapping", test_touches_survive_the_clock_wrapping},
        {"the view follows what is playing", test_the_view_follows_what_is_playing},
        {"a command is what the contract says", test_a_command_is_what_the_contract_says},
        {"a command that cannot be written is refused", test_a_command_that_cannot_be_written_is_refused},
        {"touch maps corner to corner", test_touch_maps_corner_to_corner},
        {"touch axes can be turned", test_touch_axes_can_be_turned},
        {"the cover is left alone until touched", test_the_cover_is_left_alone_until_touched},
        {"the buttons dim the cover and show", test_the_buttons_dim_the_cover_and_show},
        {"the icons point the right way", test_the_icons_point_the_right_way},
        {"a pressed button is drawn lit", test_a_pressed_button_is_drawn_lit},
        {"the picture does not depend on the bands", test_the_picture_does_not_depend_on_the_bands},
        {"no cover shows a record", test_no_cover_shows_a_record},
        {"a missing broker is marked", test_a_missing_broker_is_marked},
        {"an event is the line the store expects", test_an_event_is_the_line_the_store_expects},
    };

    for (size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int before = failures;
        tests[i].fn();
        printf("%s %s\n", failures == before ? "ok  " : "FAIL", tests[i].name);
    }

    printf("\n%d checks, %d failures\n", checks, failures);
    return failures == 0 ? 0 : 1;
}

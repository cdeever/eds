#include "screen.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "cs_config.h"
#include "display.h"
#include "draw.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "jpeg.h"
#include "touch.h"
#include "touch_map.h"

static const char *TAG = "screen";

// A setting that is off is not defined at all, so each is turned into a 0 or
// a 1 here.
#ifdef CONFIG_CS_TOUCH_SWAP_XY
#define TOUCH_SWAP_XY 1
#else
#define TOUCH_SWAP_XY 0
#endif
#ifdef CONFIG_CS_TOUCH_INVERT_X
#define TOUCH_INVERT_X 1
#else
#define TOUCH_INVERT_X 0
#endif
#ifdef CONFIG_CS_TOUCH_INVERT_Y
#define TOUCH_INVERT_Y 1
#else
#define TOUCH_INVERT_Y 0
#endif
#ifdef CONFIG_CS_PANEL_ROTATE_180
#define PANEL_TURNED 1
#else
#define PANEL_TURNED 0
#endif

// The touch glass is fixed to the panel, so a picture turned upside down
// turns both of its axes with it.
static const cs_touch_cal_t calibration = {
    .x_min = CONFIG_CS_TOUCH_X_MIN,
    .x_max = CONFIG_CS_TOUCH_X_MAX,
    .y_min = CONFIG_CS_TOUCH_Y_MIN,
    .y_max = CONFIG_CS_TOUCH_Y_MAX,
    .swap_xy = TOUCH_SWAP_XY,
    .invert_x = TOUCH_INVERT_X ^ PANEL_TURNED,
    .invert_y = TOUCH_INVERT_Y ^ PANEL_TURNED,
};

// Guards everything below it. The broker client writes; the screen task
// reads, and holds it for the length of a redraw.
static SemaphoreHandle_t lock;

static cs_ui_t ui;
static cs_command_sender send_command;

// The one cover, as the JPEG it arrived as. It is kept because the picture
// has to be drawn again every time the buttons come and go, and there is no
// memory to keep it decoded.
static uint8_t *cover;
static size_t cover_len;      // what is held and whole; 0 for none
static size_t arriving_total; // the size of the one arriving; 0 for none
static size_t arriving_got;
static bool arriving;
static uint32_t arriving_since_ms;

static uint32_t now_ms(void)
{
    return (uint32_t)(esp_timer_get_time() / 1000);
}

// --- what the broker client reports --------------------------------------------

void cs_screen_link(cs_link_t link)
{
    if (xSemaphoreTake(lock, pdMS_TO_TICKS(2000)) != pdTRUE) {
        return;
    }
    cs_ui_set_link(&ui, link);
    xSemaphoreGive(lock);
}

void cs_screen_playing(cs_play_t play, bool has_art, bool controls, bool source)
{
    if (xSemaphoreTake(lock, pdMS_TO_TICKS(2000)) != pdTRUE) {
        ESP_LOGW(TAG, "dropped a track change: the screen was busy");
        return;
    }
    cs_ui_set_playing(&ui, play, has_art);
    cs_ui_set_controls(&ui, controls);
    cs_ui_set_source(&ui, source);
    xSemaphoreGive(lock);
}

bool cs_screen_cover_begin(size_t total)
{
    if (xSemaphoreTake(lock, pdMS_TO_TICKS(2000)) != pdTRUE) {
        ESP_LOGW(TAG, "dropped a cover: the screen was busy");
        return false;
    }

    bool taken = false;
    // Whatever was held is gone from here: the buffer is about to be written
    // over, or the track has no cover that could be made small.
    cover_len = 0;
    arriving_got = 0;

    if (total == 0) {
        arriving = false;
        cs_ui_set_cover(&ui, false);
    } else if (total > CS_MAX_COVER_BYTES || cover == NULL) {
        arriving = false;
        cs_ui_set_cover(&ui, false);
        char extra[48];
        snprintf(extra, sizeof(extra), "\"bytes\":%u,\"limit\":%d", (unsigned)total, CS_MAX_COVER_BYTES);
        cs_event_log_with(CS_EVENT_WARN, "cover.rejected", extra, "Ignored a cover: it is larger than the screen holds");
    } else {
        arriving = true;
        arriving_total = total;
        arriving_since_ms = now_ms();
        taken = true;
    }

    xSemaphoreGive(lock);
    return taken;
}

void cs_screen_cover_data(size_t offset, const uint8_t *data, size_t len)
{
    if (xSemaphoreTake(lock, pdMS_TO_TICKS(2000)) != pdTRUE) {
        return;
    }
    // In order and in bounds, or it is not the cover that was begun.
    if (arriving && offset == arriving_got && offset + len <= arriving_total) {
        memcpy(cover + offset, data, len);
        arriving_got += len;
    } else {
        arriving = false;
    }
    xSemaphoreGive(lock);
}

void cs_screen_cover_end(void)
{
    if (xSemaphoreTake(lock, pdMS_TO_TICKS(2000)) != pdTRUE) {
        return;
    }
    if (arriving && arriving_got == arriving_total) {
        cover_len = arriving_total;
        cs_ui_set_cover(&ui, true);
    } else {
        cs_ui_set_cover(&ui, false);
    }
    arriving = false;
    xSemaphoreGive(lock);
}

// --- drawing -----------------------------------------------------------------

typedef struct {
    const cs_view_t *view;
    uint16_t *band;
    int x_off, y_off;
    int width, height;
    int band_top;  // in the picture's rows
    int band_rows; // 0 before the first block
    bool failed;
} painter_t;

// filler draws rows with no picture in them: above and below a cover that is
// shorter than the panel.
static bool filler(const cs_view_t *view, uint16_t *band, int y, int end)
{
    while (y < end) {
        int rows = end - y > CS_BAND_ROWS ? CS_BAND_ROWS : end - y;
        cs_draw_black(rows, band);
        cs_draw_chrome(view, y, rows, band);
        if (cs_display_push(y, rows) != ESP_OK) {
            return false;
        }
        y += rows;
    }
    return true;
}

static bool flush(painter_t *p)
{
    if (p->band_rows == 0) {
        return true;
    }
    int y = p->y_off + p->band_top;
    cs_draw_chrome(p->view, y, p->band_rows, p->band);
    return cs_display_push(y, p->band_rows) == ESP_OK;
}

static bool on_size(void *ctx, int width, int height)
{
    painter_t *p = ctx;
    if (width <= 0 || height <= 0 || width > CS_SCREEN_W || height > CS_SCREEN_H) {
        ESP_LOGW(TAG, "a %dx%d cover does not fit the panel", width, height);
        return false;
    }
    p->width = width;
    p->height = height;
    p->x_off = (CS_SCREEN_W - width) / 2;
    p->y_off = (CS_SCREEN_H - height) / 2;
    return filler(p->view, p->band, 0, p->y_off);
}

// Blocks come left to right, then down. A block that starts a new row of
// blocks means the band before it is complete.
static bool on_block(void *ctx, int left, int top, int width, int height, const uint8_t *rgb)
{
    painter_t *p = ctx;

    if (p->band_rows == 0 || top != p->band_top) {
        if (!flush(p)) {
            p->failed = true;
            return false;
        }
        if (height > CS_BAND_ROWS) {
            p->failed = true;
            return false;
        }
        p->band_top = top;
        p->band_rows = height;
        cs_draw_black(height, p->band);
    }
    if (top + height > p->height || left + width > p->width || height > p->band_rows) {
        p->failed = true;
        return false;
    }

    for (int row = 0; row < height; row++) {
        uint16_t *out = p->band + (size_t)row * CS_SCREEN_W + p->x_off + left;
        for (int x = 0; x < width; x++) {
            out[x] = cs_rgb565(rgb[0], rgb[1], rgb[2]);
            rgb += 3;
        }
    }
    return true;
}

static bool draw_cover(const cs_view_t *view)
{
    painter_t painter = {.view = view, .band = cs_display_band()};
    cs_jpeg_sink_t sink = {.on_size = on_size, .on_block = on_block, .ctx = &painter};

    if (!cs_jpeg_decode(cover, cover_len, &sink) || painter.failed) {
        return false;
    }
    return flush(&painter) &&
           filler(view, painter.band, painter.y_off + painter.height, CS_SCREEN_H);
}

static void draw_record(const cs_view_t *view)
{
    uint16_t *band = cs_display_band();
    for (int y = 0; y < CS_SCREEN_H; y += CS_BAND_ROWS) {
        int rows = CS_SCREEN_H - y > CS_BAND_ROWS ? CS_BAND_ROWS : CS_SCREEN_H - y;
        cs_draw_backdrop(view, y, rows, band);
        cs_draw_chrome(view, y, rows, band);
        if (cs_display_push(y, rows) != ESP_OK) {
            return;
        }
    }
}

// redraw paints the whole screen. Called with the lock held.
static void redraw(void)
{
    cs_view_t view;
    cs_ui_view(&ui, &view);

    if (view.cover && cover_len > 0) {
        int64_t started = esp_timer_get_time();
        if (draw_cover(&view)) {
            ESP_LOGD(TAG, "cover drawn in %d ms", (int)((esp_timer_get_time() - started) / 1000));
            return;
        }
        // Not a picture the screen can draw. Forget it, so it is not tried
        // again at every touch, and show the record: a screen with half a
        // cover on it looks broken, and one with a record on it does not.
        char extra[32];
        snprintf(extra, sizeof(extra), "\"bytes\":%u", (unsigned)cover_len);
        cs_event_log_with(CS_EVENT_WARN, "cover.rejected", extra, "Ignored a cover: it is not a picture the screen can draw");
        cover_len = 0;
        cs_ui_set_cover(&ui, false);
        cs_ui_take_dirty(&ui);
        cs_ui_view(&ui, &view);
    }
    draw_record(&view);
}

// --- the task ----------------------------------------------------------------

static void screen_task(void *arg)
{
    (void)arg;

    TickType_t last_wake = xTaskGetTickCount();
    bool was_touching = false;
    bool dark = false; // the panel is asleep (ADR-0036)

    for (;;) {
        int raw_x = 0, raw_y = 0, x = 0, y = 0;
        bool touching = cs_touch_read(&raw_x, &raw_y);
        if (touching) {
            cs_touch_map(&calibration, raw_x, raw_y, &x, &y);
            // Serial only, once a press: what the touch settings are tuned from.
            if (!was_touching) {
                ESP_LOGI(TAG, "touch: read %d,%d, taken as %d,%d", raw_x, raw_y, x, y);
            }
        }
        was_touching = touching;

        cs_button_t button = CS_BTN_NONE;
        if (xSemaphoreTake(lock, pdMS_TO_TICKS(10)) == pdTRUE) {
            uint32_t now = now_ms();
            button = cs_ui_touch(&ui, touching, x, y, now);
            cs_ui_tick(&ui, now);

            // A cover that is arriving is worth a moment's wait: drawing now
            // would draw the record, and the cover a blink later.
            bool waiting = arriving && now - arriving_since_ms < CS_COVER_ARRIVAL_MS;
            if (cs_ui_asleep(&ui)) {
                // Nothing is drawn while dark. What changes stays dirty, and
                // waking redraws anyway.
                if (!dark) {
                    cs_display_sleep();
                    dark = true;
                    cs_event_log(CS_EVENT_INFO, "screen.asleep", "Asleep: nobody is there");
                }
            } else if (!waiting && (cs_ui_take_dirty(&ui) || dark)) {
                // Waking, the picture goes on before the light, as at
                // power-on: what shows first is what is playing.
                if (dark) {
                    cs_display_wake();
                }
                redraw();
                if (dark) {
                    cs_display_light();
                    dark = false;
                    cs_event_log(CS_EVENT_INFO, "screen.awake", "Awake");
                }
            }
            xSemaphoreGive(lock);
        }

        // Outside the lock: sending waits on the network.
        if (button != CS_BTN_NONE && send_command != NULL) {
            bool sent = send_command(button);
            char extra[48];
            snprintf(extra, sizeof(extra), "\"command\":\"%s\"", cs_button_command(button));
            if (sent) {
                cs_event_log_with(CS_EVENT_INFO, "command.sent", extra, "Sent %s", cs_button_command(button));
            } else {
                cs_event_log_with(CS_EVENT_WARN, "command.not_sent", extra, "Could not send %s",
                                  cs_button_command(button));
            }
        }

        vTaskDelayUntil(&last_wake, pdMS_TO_TICKS(CS_TOUCH_POLL_MS));
    }
}

esp_err_t cs_screen_start(cs_command_sender send)
{
    send_command = send;
    lock = xSemaphoreCreateMutex();
    if (lock == NULL) {
        return ESP_ERR_NO_MEM;
    }

    // Taken now, before WiFi and TLS have broken the heap into pieces too
    // small for it. Without it the screen still runs, and shows the record.
    cover = malloc(CS_MAX_COVER_BYTES);
    if (cover == NULL) {
        ESP_LOGW(TAG, "no memory for a cover; the screen will show none");
    }

    cs_ui_init(&ui);

    // Something deliberate on the panel before the network exists, so an
    // unconfigured or unreachable screen does not look broken.
    cs_ui_take_dirty(&ui);
    redraw();
    cs_display_light();

    // The largest stack use is a redraw: the decoder's state, and a band of
    // drawing in floating point.
    if (xTaskCreate(screen_task, "screen", 6144, NULL, 5, NULL) != pdPASS) {
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}

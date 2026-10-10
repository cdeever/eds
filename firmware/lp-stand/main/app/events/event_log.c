#include "event_log.h"

#include <stdarg.h>
#include <stdio.h>

#include "esp_log.h"
#include "esp_timer.h"
#include "freertos/queue.h"
#include "freertos/semphr.h"
#include "lp_config.h"

static const char *TAG = "events";

static QueueHandle_t queue;
static SemaphoreHandle_t lock;
static uint32_t dropped;

// Guarded by lock, so no caller needs the stack for them - on_scene runs on
// the MQTT task and the WiFi handler on the event loop, neither with much.
static char message[LP_EVENT_LINE_LEN];
static char line[LP_EVENT_LINE_LEN];

esp_err_t lp_event_log_init(void)
{
    queue = xQueueCreate(LP_EVENT_QUEUE_LEN, LP_EVENT_LINE_LEN);
    lock = xSemaphoreCreateMutex();
    if (queue == NULL || lock == NULL) {
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}

void lp_event_log_with(lp_event_level_t level, const char *event, const char *extra_json,
                       const char *fmt, ...)
{
    if (queue == NULL || lock == NULL || event == NULL || fmt == NULL) {
        return;
    }
    // Bounded, like the scene lock: an event lost to contention is better than
    // a caller held up to record one.
    if (xSemaphoreTake(lock, pdMS_TO_TICKS(100)) != pdTRUE) {
        return;
    }

    va_list args;
    va_start(args, fmt);
    vsnprintf(message, sizeof(message), fmt, args);
    va_end(args);

    size_t len = lp_event_format(line, sizeof(line), level, event, message,
                                 esp_timer_get_time() / 1000, dropped, extra_json);
    if (len == 0) {
        ESP_LOGW(TAG, "%s does not fit in %d bytes; dropped", event, LP_EVENT_LINE_LEN);
        dropped++;
        xSemaphoreGive(lock);
        return;
    }

    // The count this line carries is reported once it is queued, so what is
    // left to report afterwards is only what was lost making room for it.
    uint32_t lost = 0;
    if (xQueueSend(queue, line, 0) != pdTRUE) {
        // Full. message is free again by now, and is the right size to take
        // the one being discarded.
        if (xQueueReceive(queue, message, 0) == pdTRUE) {
            lost++;
        }
        if (xQueueSend(queue, line, 0) != pdTRUE) {
            dropped += lost + 1;
            xSemaphoreGive(lock);
            return;
        }
    }
    dropped = lost;

    xSemaphoreGive(lock);
}

bool lp_event_log_receive(char *out, size_t out_len, TickType_t ticks_to_wait)
{
    if (queue == NULL || out == NULL || out_len < LP_EVENT_LINE_LEN) {
        return false;
    }
    return xQueueReceive(queue, out, ticks_to_wait) == pdTRUE;
}

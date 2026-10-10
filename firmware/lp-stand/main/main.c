// The LP jacket stand.
//
// A stand that props up the LP jacket while vinyl spins, lit by an addressable
// RGB strip that reacts to the record's cover art.
//
// The device subscribes to a scene descriptor and animates it locally. It never
// receives frames: streaming pixels over WiFi turns latency spikes into visible
// stutter, and a broker outage would freeze an animation mid-sweep. Rendering
// here means a stand whose network has gone away keeps doing something
// sensible.

#include <stdio.h>

#include "effects.h"
#include "esp_app_desc.h"
#include "esp_log.h"
#include "esp_system.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "led_strip_out.h"
#include "lp_config.h"
#include "lp_credentials.h"
#include "mqtt.h"
#include "nvs_flash.h"
#include "scene.h"
#include "wifi.h"

static const char *TAG = "lp-stand";

static lp_scene_t current;
static SemaphoreHandle_t current_lock;
static lp_rgb_t frame[LP_LED_COUNT];

// on_scene runs on the MQTT event task, so it only swaps the scene in. The
// render task picks it up on its next frame.
static void on_scene(const lp_scene_t *scene)
{
    if (xSemaphoreTake(current_lock, pdMS_TO_TICKS(100)) != pdTRUE) {
        ESP_LOGW(TAG, "dropped a scene: render task held the lock");
        return;
    }
    current = *scene;
    xSemaphoreGive(current_lock);

    lp_mqtt_publish_state(scene);

    char extra[96];
    snprintf(extra, sizeof(extra), "\"effect\":\"%s\",\"colors\":%u,\"brightness\":%.2f",
             lp_effect_name(scene->effect), (unsigned)scene->palette_len, scene->brightness);
    lp_event_log_with(LP_EVENT_INFO, "scene.applied", extra, "Showing %s in %u colours",
                      lp_effect_name(scene->effect), (unsigned)scene->palette_len);
}

// Why the stand started, in words. A stand that reports "power" once a week
// is being switched off with the lamp; one that reports "panic" is a bug.
static const char *reset_reason(void)
{
    switch (esp_reset_reason()) {
    case ESP_RST_POWERON:
        return "power";
    case ESP_RST_SW:
        return "software";
    case ESP_RST_PANIC:
        return "panic";
    case ESP_RST_INT_WDT:
    case ESP_RST_TASK_WDT:
    case ESP_RST_WDT:
        return "watchdog";
    case ESP_RST_BROWNOUT:
        return "brownout";
    case ESP_RST_DEEPSLEEP:
        return "sleep";
    case ESP_RST_EXT:
        return "reset pin";
    default:
        return "unknown";
    }
}

static void render_task(void *arg)
{
    (void)arg;

    TickType_t last_wake = xTaskGetTickCount();
    for (;;) {
        lp_scene_t scene;
        if (xSemaphoreTake(current_lock, pdMS_TO_TICKS(10)) == pdTRUE) {
            scene = current;
            xSemaphoreGive(current_lock);
        } else {
            // Never stall the strip waiting for a lock; a skipped frame is
            // invisible and a stalled one is not.
            vTaskDelayUntil(&last_wake, pdMS_TO_TICKS(LP_FRAME_INTERVAL_MS));
            continue;
        }

        lp_effects_render(&scene, (uint32_t)(xTaskGetTickCount() * portTICK_PERIOD_MS),
                          frame, LP_LED_COUNT);
        lp_strip_show(frame, LP_LED_COUNT);

        vTaskDelayUntil(&last_wake, pdMS_TO_TICKS(LP_FRAME_INTERVAL_MS));
    }
}

void app_main(void)
{
    esp_err_t err = nvs_flash_init();
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        err = nvs_flash_init();
    }
    ESP_ERROR_CHECK(err);

    // First, so everything after it can be recorded. Failing to create the
    // queue is not fatal: every recorder checks for it, and a stand that
    // cannot report is still a stand.
    if (lp_event_log_init() != ESP_OK) {
        ESP_LOGW(TAG, "no memory for the event log; running without it");
    }

    const esp_app_desc_t *app = esp_app_get_description();
    char extra[128];
    snprintf(extra, sizeof(extra), "\"reason\":\"%s\",\"firmware\":\"%s\",\"idf\":\"%s\"",
             reset_reason(), app->version, app->idf_ver);
    esp_reset_reason_t why = esp_reset_reason();
    bool crashed = why == ESP_RST_PANIC || why == ESP_RST_INT_WDT || why == ESP_RST_TASK_WDT ||
                   why == ESP_RST_WDT || why == ESP_RST_BROWNOUT;
    lp_event_log_with(crashed ? LP_EVENT_ERROR : LP_EVENT_INFO, "system.boot", extra,
                      "Started after %s, firmware %s", reset_reason(), app->version);

    // Before WiFi or MQTT: both read through this, and an unprovisioned stand
    // carries on with no keys rather than refusing to boot.
    ESP_ERROR_CHECK(lp_credentials_load());

    current_lock = xSemaphoreCreateMutex();
    ESP_ERROR_CHECK(current_lock == NULL ? ESP_ERR_NO_MEM : ESP_OK);

    // Something deliberate on the strip before the network exists, so an
    // unconfigured or unreachable stand does not look broken.
    lp_scene_default(&current);
    ESP_ERROR_CHECK(lp_strip_init());

    xTaskCreate(render_task, "render", 4096, NULL, 5, NULL);
    ESP_LOGI(TAG, "rendering at %d ms/frame on %d LEDs",
             LP_FRAME_INTERVAL_MS, LP_LED_COUNT);

    // A failure to associate is not fatal: the strip is already lit, and WiFi
    // keeps retrying underneath.
    lp_wifi_start();

    ESP_ERROR_CHECK(lp_mqtt_start(on_scene));

    // The retained scene arrives moments after subscribing, which is the whole
    // point: a stand that reboots mid-album picks the record back up rather
    // than waiting for the next one.
    ESP_LOGI(TAG, "subscribed; awaiting a retained scene on %s", LP_TOPIC_SCENE);
}

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

#include "effects.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "led_strip_out.h"
#include "lp_config.h"
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

// The cover screen.
//
// A small touch screen that shows the cover of what is playing, and when
// touched, five buttons: play or pause, previous, next, volume up and down.
//
// It reads the current track and a small copy of its cover from the broker,
// and sends each button press there as a command. It knows no player: what
// is current is nowplayd's to say, and which player a command goes to is
// nowplayd's to decide (ADR-0020).

#include <stdio.h>

#include "cs_config.h"
#include "cs_credentials.h"
#include "display.h"
#include "esp_app_desc.h"
#include "esp_log.h"
#include "esp_system.h"
#include "event_log.h"
#include "mqtt.h"
#include "nvs_flash.h"
#include "screen.h"
#include "touch.h"
#include "wifi.h"

static const char *TAG = "cover-screen";

// Why the screen started, in words.
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

void app_main(void)
{
    esp_err_t err = nvs_flash_init();
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        err = nvs_flash_init();
    }
    ESP_ERROR_CHECK(err);

    // First, so everything after it can be recorded.
    if (cs_event_log_init() != ESP_OK) {
        ESP_LOGW(TAG, "no memory for the event log; running without it");
    }

    const esp_app_desc_t *app = esp_app_get_description();
    char extra[128];
    snprintf(extra, sizeof(extra), "\"reason\":\"%s\",\"firmware\":\"%s\",\"idf\":\"%s\"",
             reset_reason(), app->version, app->idf_ver);
    esp_reset_reason_t why = esp_reset_reason();
    bool crashed = why == ESP_RST_PANIC || why == ESP_RST_INT_WDT || why == ESP_RST_TASK_WDT ||
                   why == ESP_RST_WDT || why == ESP_RST_BROWNOUT;
    cs_event_log_with(crashed ? CS_EVENT_ERROR : CS_EVENT_INFO, "system.boot", extra,
                      "Started after %s, firmware %s", reset_reason(), app->version);

    // Before WiFi or MQTT: both read through this, and an unprovisioned
    // screen carries on with no keys rather than refusing to boot.
    ESP_ERROR_CHECK(cs_credentials_load());

    // The panel before the network, and the cover's memory with it: a screen
    // that is lit and showing a record is one that is plainly working, and
    // the heap is never again as whole as it is now.
    ESP_ERROR_CHECK(cs_display_init());
    ESP_ERROR_CHECK(cs_touch_init());
    ESP_ERROR_CHECK(cs_screen_start(cs_mqtt_send_command));

    // A failure to associate is not fatal: WiFi keeps retrying underneath,
    // and the screen says so in its corner.
    cs_wifi_start();

    ESP_ERROR_CHECK(cs_mqtt_start());

    // Both topics are retained, so the track and its cover arrive moments
    // after subscribing: a screen that restarts mid-album picks it back up.
    ESP_LOGI(TAG, "awaiting %s and %s", CS_TOPIC_CURRENT, CS_TOPIC_COVER);
}

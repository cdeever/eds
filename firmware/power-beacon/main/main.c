// The power beacon.
//
// A board with nothing attached, plugged into an outlet that is switched with
// something that cannot say for itself whether it is on. Being connected to
// the broker is the beacon's only message: "online", retained, while it has
// power, and the broker's "offline" on its behalf when it does not (ADR-0023).
//
// It reads nothing, drives nothing and decides nothing. What its presence
// means is for whoever subscribes to say.

#include <stdio.h>

#include "pb_config.h"
#include "pb_credentials.h"
#include "esp_app_desc.h"
#include "esp_log.h"
#include "esp_system.h"
#include "event_log.h"
#include "mqtt.h"
#include "nvs_flash.h"
#include "wifi.h"

static const char *TAG = "power-beacon";

// Why the beacon started, in words. "power" is the ordinary one here: every
// time the switch goes on is a start.
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
    if (pb_event_log_init() != ESP_OK) {
        ESP_LOGW(TAG, "no memory for the event log; running without it");
    }

    const esp_app_desc_t *app = esp_app_get_description();
    char extra[128];
    snprintf(extra, sizeof(extra), "\"reason\":\"%s\",\"firmware\":\"%s\",\"idf\":\"%s\"",
             reset_reason(), app->version, app->idf_ver);
    esp_reset_reason_t why = esp_reset_reason();
    bool crashed = why == ESP_RST_PANIC || why == ESP_RST_INT_WDT || why == ESP_RST_TASK_WDT ||
                   why == ESP_RST_WDT || why == ESP_RST_BROWNOUT;
    pb_event_log_with(crashed ? PB_EVENT_ERROR : PB_EVENT_INFO, "system.boot", extra,
                      "Started after %s, firmware %s", reset_reason(), app->version);

    // Before WiFi or MQTT: both read through this, and an unprovisioned
    // beacon carries on with no keys rather than refusing to boot.
    ESP_ERROR_CHECK(pb_credentials_load());

    // Straight to the network: the time from here to the broker is the time
    // the room waits to learn the switch went on. A failure to associate is
    // not fatal; WiFi keeps retrying underneath.
    pb_wifi_start();

    ESP_ERROR_CHECK(pb_mqtt_start());
}

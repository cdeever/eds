// Ears: the reSpeaker Lite (ADR-0043).
//
// An XMOS chip cleans up what two microphones hear and gives one voice over
// I2S to the XIAO ESP32-S3 this runs on. While the XIAO's button is held,
// what is said is kept, and on release sent as one WAV to the broker, where
// voiced makes it a request for an album (ADR-0042). What voiced made of it
// comes back, and the light says.

#include <stdio.h>

#include "ea_config.h"
#include "ea_credentials.h"
#include "esp_app_desc.h"
#include "esp_heap_caps.h"
#include "esp_log.h"
#include "esp_system.h"
#include "event_log.h"
#include "mic.h"
#include "mqtt.h"
#include "nvs_flash.h"
#include "panel.h"
#include "wifi.h"

static const char *TAG = "ears";

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
    case ESP_RST_USB:
        return "usb";
    case ESP_RST_EXT:
        return "reset pin";
    default:
        return "unknown";
    }
}

static void send_request(const uint8_t *wav, size_t len)
{
    ea_mqtt_send_request(wav, len);
}

void app_main(void)
{
    esp_err_t err = nvs_flash_init();
    if (err == ESP_ERR_NVS_NO_FREE_PAGES || err == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_ERROR_CHECK(nvs_flash_erase());
        err = nvs_flash_init();
    }
    ESP_ERROR_CHECK(err);

    if (ea_event_log_init() != ESP_OK) {
        ESP_LOGW(TAG, "no memory for the event log; running without it");
    }

    const esp_app_desc_t *app = esp_app_get_description();
    char extra[160];
    snprintf(extra, sizeof(extra), "\"reason\":\"%s\",\"firmware\":\"%s\",\"idf\":\"%s\",\"psram\":%u",
             reset_reason(), app->version, app->idf_ver, (unsigned)heap_caps_get_total_size(MALLOC_CAP_SPIRAM));
    esp_reset_reason_t why = esp_reset_reason();
    bool crashed = why == ESP_RST_PANIC || why == ESP_RST_INT_WDT || why == ESP_RST_TASK_WDT ||
                   why == ESP_RST_WDT || why == ESP_RST_BROWNOUT;
    ea_event_log_with(crashed ? EA_EVENT_ERROR : EA_EVENT_INFO, "system.boot", extra,
                      "Started after %s, firmware %s", reset_reason(), app->version);

    ESP_ERROR_CHECK(ea_credentials_load());
    ESP_ERROR_CHECK(ea_panel_init());

    // The microphones first: what they are given is printed on the console
    // whether or not there is a network, which is what a bench needs.
    if (ea_mic_start(send_request, ea_mqtt_send_woken) != ESP_OK) {
        ea_event_log(EA_EVENT_ERROR, "audio.failed", "The microphones could not be started");
    }

    ea_wifi_start();
    ESP_ERROR_CHECK(ea_mqtt_start());
    ESP_LOGI(TAG, "hold the button (GPIO%d) and speak", CONFIG_EA_BUTTON_GPIO);
}

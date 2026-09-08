#include "wifi.h"

#include <string.h>

#include "esp_event.h"
#include "esp_log.h"
#include "esp_wifi.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include "lwip/ip4_addr.h"
#include "nvs_flash.h"

static const char *TAG = "wifi";

#define LP_WIFI_CONNECTED_BIT BIT0

static EventGroupHandle_t wifi_events;
static int retries;

static void on_wifi_event(void *arg, esp_event_base_t base, int32_t id, void *data)
{
    (void)arg;

    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_START) {
        esp_wifi_connect();
        return;
    }

    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) {
        xEventGroupClearBits(wifi_events, LP_WIFI_CONNECTED_BIT);
        // Backing off would be tidier, but an unattended stand that stops
        // trying is one that needs a person. Keep reconnecting.
        retries++;
        if (retries % 10 == 1) {
            ESP_LOGW(TAG, "disconnected, retrying (attempt %d)", retries);
        }
        vTaskDelay(pdMS_TO_TICKS(2000));
        esp_wifi_connect();
        return;
    }

    if (base == IP_EVENT && id == IP_EVENT_STA_GOT_IP) {
        const ip_event_got_ip_t *event = (const ip_event_got_ip_t *)data;
        ESP_LOGI(TAG, "got " IPSTR, IP2STR(&event->ip_info.ip));
        retries = 0;
        xEventGroupSetBits(wifi_events, LP_WIFI_CONNECTED_BIT);
    }
}

esp_err_t lp_wifi_start(void)
{
    wifi_events = xEventGroupCreate();
    if (wifi_events == NULL) {
        return ESP_ERR_NO_MEM;
    }

    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());
    esp_netif_create_default_wifi_sta();

    wifi_init_config_t init = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&init));

    ESP_ERROR_CHECK(esp_event_handler_instance_register(
        WIFI_EVENT, ESP_EVENT_ANY_ID, &on_wifi_event, NULL, NULL));
    ESP_ERROR_CHECK(esp_event_handler_instance_register(
        IP_EVENT, IP_EVENT_STA_GOT_IP, &on_wifi_event, NULL, NULL));

    wifi_config_t config = {0};
    strlcpy((char *)config.sta.ssid, CONFIG_LP_WIFI_SSID, sizeof(config.sta.ssid));
    strlcpy((char *)config.sta.password, CONFIG_LP_WIFI_PASSWORD, sizeof(config.sta.password));
    config.sta.threshold.authmode = WIFI_AUTH_WPA2_PSK;

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &config));
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "connecting to %s", CONFIG_LP_WIFI_SSID);

    // Wait, but not forever: the render task should start showing the default
    // scene even if the network never comes up.
    EventBits_t bits = xEventGroupWaitBits(wifi_events, LP_WIFI_CONNECTED_BIT,
                                           pdFALSE, pdTRUE, pdMS_TO_TICKS(30000));
    if ((bits & LP_WIFI_CONNECTED_BIT) == 0) {
        ESP_LOGW(TAG, "no address yet; continuing and retrying in the background");
        return ESP_ERR_TIMEOUT;
    }
    return ESP_OK;
}

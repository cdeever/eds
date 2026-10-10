#include "wifi.h"

#include <stdio.h>
#include <string.h>

#include "esp_event.h"
#include "esp_log.h"
#include "esp_wifi.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include "lp_credentials.h"
#include "lwip/ip4_addr.h"
#include "nvs_flash.h"

static const char *TAG = "wifi";

#define LP_WIFI_CONNECTED_BIT BIT0

static EventGroupHandle_t wifi_events;
static int retries;
static char ip_text[16];

int lp_wifi_rssi(void)
{
    wifi_ap_record_t ap;
    return esp_wifi_sta_get_ap_info(&ap) == ESP_OK ? ap.rssi : 0;
}

const char *lp_wifi_ip(void)
{
    return ip_text;
}

static void on_wifi_event(void *arg, esp_event_base_t base, int32_t id, void *data)
{
    (void)arg;

    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_START) {
        esp_wifi_connect();
        return;
    }

    if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) {
        // Recorded once, when an association is lost, and not for every retry
        // after it: a stand out of range would otherwise fill its event queue
        // with the same line and push out whatever happened before.
        if (xEventGroupGetBits(wifi_events) & LP_WIFI_CONNECTED_BIT) {
            const wifi_event_sta_disconnected_t *lost = (const wifi_event_sta_disconnected_t *)data;
            char extra[32];
            snprintf(extra, sizeof(extra), "\"reason\":%d", (int)lost->reason);
            lp_event_log_with(LP_EVENT_WARN, "wifi.disconnected", extra,
                              "Lost the WiFi network (reason %d)", (int)lost->reason);
        }
        ip_text[0] = '\0';
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
        snprintf(ip_text, sizeof(ip_text), IPSTR, IP2STR(&event->ip_info.ip));

        char ssid[2 * 33];
        char extra[192];
        lp_event_escape(lp_cred_wifi_ssid(), ssid, sizeof(ssid));
        snprintf(extra, sizeof(extra), "\"ssid\":\"%s\",\"ip\":\"%s\",\"rssi\":%d,\"attempts\":%d",
                 ssid, ip_text, lp_wifi_rssi(), retries + 1);
        lp_event_log_with(LP_EVENT_INFO, "wifi.connected", extra,
                          "Joined %s as %s", lp_cred_wifi_ssid(), ip_text);
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
    strlcpy((char *)config.sta.ssid, lp_cred_wifi_ssid(), sizeof(config.sta.ssid));
    strlcpy((char *)config.sta.password, lp_cred_wifi_password(), sizeof(config.sta.password));

    // Demanding WPA2 of an open network is a connection that can never
    // succeed, and the driver only says so as a warning. Match the threshold
    // to whether a key was actually provisioned.
    config.sta.threshold.authmode =
        config.sta.password[0] != '\0' ? WIFI_AUTH_WPA2_PSK : WIFI_AUTH_OPEN;

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &config));
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "connecting to %s", lp_cred_wifi_ssid());

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

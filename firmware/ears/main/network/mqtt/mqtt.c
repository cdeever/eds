#include "mqtt.h"

#include <stdio.h>
#include <string.h>

#include "ea_config.h"
#include "ea_credentials.h"
#include "esp_heap_caps.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "mic.h"
#include "mqtt_client.h"
#include "panel.h"
#include "wifi.h"

static const char *TAG = "mqtt";

static esp_mqtt_client_handle_t client;
static volatile bool connected;

// What was heard is a few hundred bytes. Anything larger is not that.
#define MAX_HEARD 1024
static char heard[MAX_HEARD];

bool ea_mqtt_connected(void)
{
    return connected;
}

static void on_connected(void)
{
    ESP_LOGI(TAG, "connected; listening on %s", EA_TOPIC_HEARD);
    esp_mqtt_client_subscribe(client, EA_TOPIC_HEARD, 0);
    esp_mqtt_client_publish(client, EA_TOPIC_STATUS, EA_STATUS_ONLINE, 0, 1, 1);

    char extra[64];
    snprintf(extra, sizeof(extra), "\"ip\":\"%s\",\"rssi\":%d", ea_wifi_ip(), ea_wifi_rssi());
    ea_event_log_with(EA_EVENT_INFO, "mqtt.connected", extra, "Connected to the broker");
    connected = true;
    ea_panel_flash(2, 80, 120);
}

// What voiced made of a request: one long flash for an album asked for, three
// short for anything else. The words are not kept or logged here: they are
// voiced's, in its journal, and the room heard them already.
static void on_heard(const char *json, size_t len)
{
    bool understood = memmem(json, len, "\"understood\":true", 17) != NULL;
    ESP_LOGI(TAG, "heard: %.*s", (int)len, json);
    if (understood) {
        ea_panel_flash(1, 900, 100);
    } else {
        ea_panel_flash(3, 120, 120);
    }
    ea_event_log_with(understood ? EA_EVENT_INFO : EA_EVENT_WARN, "voice.heard",
                      understood ? "\"understood\":true" : "\"understood\":false",
                      understood ? "voiced asked for an album" : "voiced did not understand");
}

static void on_data(const esp_mqtt_event_handle_t event)
{
    static bool assembling;
    size_t offset = (size_t)event->current_data_offset;
    size_t len = (size_t)event->data_len;
    size_t total = (size_t)event->total_data_len;

    if (offset == 0) {
        size_t want = strlen(EA_TOPIC_HEARD);
        assembling = (size_t)event->topic_len == want && memcmp(event->topic, EA_TOPIC_HEARD, want) == 0 &&
                     total < sizeof(heard);
    }
    if (!assembling || offset + len > sizeof(heard)) {
        assembling = false;
        return;
    }
    memcpy(heard + offset, event->data, len);
    if (offset + len >= total) {
        assembling = false;
        on_heard(heard, total);
    }
}

static void on_mqtt_event(void *handler_args, esp_event_base_t base, int32_t id, void *data)
{
    (void)handler_args;
    (void)base;
    const esp_mqtt_event_handle_t event = (esp_mqtt_event_handle_t)data;

    switch ((esp_mqtt_event_id_t)id) {
    case MQTT_EVENT_CONNECTED:
        on_connected();
        break;
    case MQTT_EVENT_DATA:
        on_data(event);
        break;
    case MQTT_EVENT_DISCONNECTED:
        ESP_LOGW(TAG, "disconnected; the client will keep retrying");
        if (connected) {
            connected = false;
            ea_event_log(EA_EVENT_WARN, "mqtt.disconnected", "Lost the broker");
        }
        break;
    case MQTT_EVENT_ERROR:
        if (event->error_handle->error_type == MQTT_ERROR_TYPE_CONNECTION_REFUSED) {
            ESP_LOGE(TAG, "the broker refused the connection (code %d)",
                     event->error_handle->connect_return_code);
        } else {
            ESP_LOGE(TAG, "transport error: esp_err 0x%x, tls 0x%x, errno %d",
                     (unsigned)event->error_handle->esp_tls_last_esp_err,
                     (unsigned)event->error_handle->esp_tls_stack_err,
                     event->error_handle->esp_transport_sock_errno);
        }
        break;
    default:
        break;
    }
}

static void log_health(void)
{
    char extra[192];
    snprintf(extra, sizeof(extra),
             "\"rssi\":%d,\"heap_free\":%u,\"psram_free\":%u,\"rate\":%u,\"left_db\":%.1f,\"right_db\":%.1f",
             ea_wifi_rssi(), (unsigned)heap_caps_get_free_size(MALLOC_CAP_INTERNAL),
             (unsigned)heap_caps_get_free_size(MALLOC_CAP_SPIRAM), (unsigned)ea_mic_rate(),
             (double)ea_mic_level(0), (double)ea_mic_level(1));
    ea_event_log_with(EA_EVENT_INFO, "device.health", extra, "Health report");
}

// Carries queued events to the log topic while the broker is there, and says
// the device is online again every thirty seconds (ADR-0034).
static void log_task(void *arg)
{
    (void)arg;
    static char line[EA_EVENT_LINE_LEN];
    TickType_t last_health = xTaskGetTickCount();
    TickType_t last_presence = xTaskGetTickCount();

    for (;;) {
        if (!connected) {
            vTaskDelay(pdMS_TO_TICKS(250));
            continue;
        }
        if (xTaskGetTickCount() - last_health >= pdMS_TO_TICKS(EA_HEALTH_INTERVAL_MS)) {
            last_health = xTaskGetTickCount();
            log_health();
        }
        if (xTaskGetTickCount() - last_presence >= pdMS_TO_TICKS(EA_PRESENCE_INTERVAL_MS)) {
            last_presence = xTaskGetTickCount();
            esp_mqtt_client_publish(client, EA_TOPIC_STATUS, EA_STATUS_ONLINE, 0, 0, 1);
        }
        if (!ea_event_log_receive(line, sizeof(line), pdMS_TO_TICKS(250))) {
            continue;
        }
        if (esp_mqtt_client_publish(client, EA_TOPIC_LOG, line, 0, 1, 0) < 0) {
            ESP_LOGW(TAG, "could not publish an event: %s", line);
        }
    }
}

esp_err_t ea_mqtt_start(void)
{
    esp_mqtt_client_config_t config = {
        .broker.address.uri = ea_cred_mqtt_uri(),
        .broker.verification.certificate = ea_cred_mqtt_ca(),
        .credentials = {
            .username = ea_cred_mqtt_username(),
            .client_id = EA_DEVICE_ID,
            .authentication.password = ea_cred_mqtt_password(),
        },
        .session.last_will = {
            .topic = EA_TOPIC_STATUS,
            .msg = EA_STATUS_OFFLINE,
            .qos = 1,
            .retain = 1,
        },
        .network.reconnect_timeout_ms = 5000,
        // A request is written in one go, half a megabyte at most; on a
        // slow moment the default would give up part-way.
        .network.timeout_ms = 20000,
        // And in pieces of this: with the default 1 KB, a five-second
        // request was 160 writes, each waiting on the one before's
        // acknowledgment, and took 16 to 22 seconds (CHG-0036).
        .buffer.out_size = 16384,
    };

    client = esp_mqtt_client_init(&config);
    if (client == NULL) {
        return ESP_FAIL;
    }
    ESP_ERROR_CHECK(esp_mqtt_client_register_event(client, ESP_EVENT_ANY_ID, &on_mqtt_event, NULL));
    if (xTaskCreate(log_task, "events", 4096, NULL, 3, NULL) != pdPASS) {
        ESP_LOGW(TAG, "no event task; the device will run without reporting events");
    }
    ESP_LOGI(TAG, "dialing %s as %s; requests on %s", ea_cred_mqtt_uri(), ea_cred_mqtt_username(),
             EA_TOPIC_UTTERANCE);
    return esp_mqtt_client_start(client);
}

bool ea_mqtt_send_request(const uint8_t *wav, size_t len)
{
    if (client == NULL || !connected) {
        ESP_LOGW(TAG, "no broker; the request was not sent");
        ea_panel_flash(3, 120, 120);
        return false;
    }
    int64_t started = esp_timer_get_time();
    // QoS 0 and not retained, as a button press is: a request lost is asked
    // again; one that arrives late plays an album nobody is waiting for.
    int id = esp_mqtt_client_publish(client, EA_TOPIC_UTTERANCE, (const char *)wav, (int)len, 0, 0);
    unsigned took = (unsigned)((esp_timer_get_time() - started) / 1000);
    char extra[64];
    snprintf(extra, sizeof(extra), "\"bytes\":%u,\"took_ms\":%u", (unsigned)len, took);
    if (id < 0) {
        ea_event_log_with(EA_EVENT_WARN, "voice.unsent", extra, "A request could not be sent");
        ea_panel_flash(3, 120, 120);
        return false;
    }
    ea_event_log_with(EA_EVENT_INFO, "voice.sent", extra, "Sent a request of %u ms",
                      (unsigned)((len - 44) / 2 * 1000 / 16000));
    return true;
}

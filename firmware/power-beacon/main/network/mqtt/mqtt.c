#include "mqtt.h"

#include <stdio.h>

#include "esp_heap_caps.h"
#include "esp_log.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "mqtt_client.h"
#include "pb_config.h"
#include "pb_credentials.h"
#include "wifi.h"

static const char *TAG = "mqtt";

static esp_mqtt_client_handle_t client;
static volatile bool connected;

static void on_connected(void)
{
    // Presence, retained, replacing the will the broker would have published.
    // Said on every connection and not only the first: after a broker
    // restart, or a will sent for a connection that was only slow, this is
    // what puts the topic right again.
    esp_mqtt_client_publish(client, PB_TOPIC_STATUS, PB_STATUS_ONLINE, 0, 1, 1);
    ESP_LOGI(TAG, "connected; %s is %s", PB_TOPIC_STATUS, PB_STATUS_ONLINE);

    // More of these than of system.boot is how a flapping connection shows
    // up, and for a beacon each one beyond the first was a moment of saying
    // the switch was off when it was not. The uptime on the first is how
    // long the room waited to learn it was on.
    char extra[64];
    snprintf(extra, sizeof(extra), "\"ip\":\"%s\",\"rssi\":%d", pb_wifi_ip(), pb_wifi_rssi());
    pb_event_log_with(PB_EVENT_INFO, "mqtt.connected", extra, "Connected to the broker");
    connected = true;
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
    case MQTT_EVENT_DISCONNECTED:
        ESP_LOGW(TAG, "disconnected; the client will keep retrying");
        // This fires for every failed attempt as well as for a lost
        // connection. Only the loss is an event; it waits in the queue and is
        // delivered when the broker is back.
        if (connected) {
            connected = false;
            pb_event_log(PB_EVENT_WARN, "mqtt.disconnected", "Lost the broker");
        }
        break;
    case MQTT_EVENT_ERROR:
        // Serial only: none of this can be published, by definition. A
        // refusal names the account; anything else is the network or TLS, and
        // the handshake wants tens of KB at once and fails oddly without it.
        if (event->error_handle->error_type == MQTT_ERROR_TYPE_CONNECTION_REFUSED) {
            ESP_LOGE(TAG, "the broker refused the connection (code %d)",
                     event->error_handle->connect_return_code);
        } else {
            ESP_LOGE(TAG, "transport error: esp_err 0x%x, tls 0x%x, errno %d (heap free %u)",
                     (unsigned)event->error_handle->esp_tls_last_esp_err,
                     (unsigned)event->error_handle->esp_tls_stack_err,
                     event->error_handle->esp_transport_sock_errno,
                     (unsigned)heap_caps_get_free_size(MALLOC_CAP_DEFAULT));
        }
        break;
    default:
        break;
    }
}

static void log_health(void)
{
    char extra[96];
    snprintf(extra, sizeof(extra), "\"rssi\":%d,\"heap_free\":%u,\"heap_min\":%u",
             pb_wifi_rssi(),
             (unsigned)heap_caps_get_free_size(MALLOC_CAP_DEFAULT),
             (unsigned)heap_caps_get_minimum_free_size(MALLOC_CAP_DEFAULT));
    pb_event_log_with(PB_EVENT_INFO, "beacon.health", extra, "Health report");
}

// log_task carries queued events to the beacon's log topic. It takes one only
// while the broker is there, so events recorded offline wait in their queue
// rather than being handed to a client that would discard them.
static void log_task(void *arg)
{
    (void)arg;

    static char line[PB_EVENT_LINE_LEN];
    TickType_t last_health = xTaskGetTickCount();

    for (;;) {
        if (!connected) {
            vTaskDelay(pdMS_TO_TICKS(250));
            continue;
        }

        if (xTaskGetTickCount() - last_health >= pdMS_TO_TICKS(PB_HEALTH_INTERVAL_MS)) {
            last_health = xTaskGetTickCount();
            log_health();
        }

        if (!pb_event_log_receive(line, sizeof(line), pdMS_TO_TICKS(250))) {
            continue;
        }

        // QoS 1 and not retained: each event should arrive once, and a log
        // line replayed to every new subscriber would be an event that never
        // happened again.
        if (esp_mqtt_client_publish(client, PB_TOPIC_LOG, line, 0, 1, 0) < 0) {
            ESP_LOGW(TAG, "could not publish an event: %s", line);
        }
    }
}

esp_err_t pb_mqtt_start(void)
{
    esp_mqtt_client_config_t config = {
        .broker.address.uri = pb_cred_mqtt_uri(),
        .credentials = {
            .username = pb_cred_mqtt_username(),
            .client_id = PB_BEACON_ID,
            .authentication.password = pb_cred_mqtt_password(),
        },
        .session = {
            // The will is the message the beacon exists to have sent for it:
            // nothing here runs when the power goes, so the broker says it.
            .last_will = {
                .topic = PB_TOPIC_STATUS,
                .msg = PB_STATUS_OFFLINE,
                .qos = 1,
                .retain = 1,
            },
            // And this is how soon it is said (pb_config.h).
            .keepalive = PB_MQTT_KEEPALIVE_S,
        },
        .network.reconnect_timeout_ms = 5000,
    };

    // Verify the broker against the provisioned CA. With no CA an mqtts://
    // connection fails its handshake, which is the right answer to "who am I
    // talking to?".
    config.broker.verification.certificate = pb_cred_mqtt_ca();

    client = esp_mqtt_client_init(&config);
    if (client == NULL) {
        return ESP_FAIL;
    }

    ESP_ERROR_CHECK(esp_mqtt_client_register_event(client, ESP_EVENT_ANY_ID,
                                                   &on_mqtt_event, NULL));

    if (xTaskCreate(log_task, "events", 4096, NULL, 3, NULL) != pdPASS) {
        ESP_LOGW(TAG, "no event task; the beacon will run without reporting events");
    }

    ESP_LOGI(TAG, "dialling %s as %s; presence on %s, events on %s",
             pb_cred_mqtt_uri(), pb_cred_mqtt_username(), PB_TOPIC_STATUS, PB_TOPIC_LOG);
    return esp_mqtt_client_start(client);
}

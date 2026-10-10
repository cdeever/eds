#include "mqtt.h"

#include <stdio.h>

#include "esp_heap_caps.h"
#include "esp_log.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "lp_config.h"
#include "lp_credentials.h"
#include "mqtt_client.h"
#include "scene_json.h"
#include "wifi.h"

static const char *TAG = "mqtt";

static esp_mqtt_client_handle_t client;
static lp_scene_handler scene_handler;
static volatile bool connected;

static void on_connected(void)
{
    ESP_LOGI(TAG, "connected; subscribing to %s", LP_TOPIC_SCENE);

    // QoS 1. A dropped scene means the stand shows the previous record's
    // colours until the next one, which is exactly the sort of quiet wrongness
    // that is hard to notice and annoying to debug.
    esp_mqtt_client_subscribe(client, LP_TOPIC_SCENE, 1);

    // Presence, retained, replacing the will the broker would have published.
    esp_mqtt_client_publish(client, LP_TOPIC_STATUS, LP_STATUS_ONLINE, 0, 1, 1);

    // More of these than of system.boot is how a flapping connection shows up.
    char extra[64];
    snprintf(extra, sizeof(extra), "\"ip\":\"%s\",\"rssi\":%d", lp_wifi_ip(), lp_wifi_rssi());
    lp_event_log_with(LP_EVENT_INFO, "mqtt.connected", extra, "Connected to the broker");
    connected = true;
}

// A scene that was refused is the event most worth keeping: the stand's
// answer to one is to carry on showing the last good scene, which from the
// room looks exactly like nothing having been sent.
static void scene_rejected(const char *why, int bytes)
{
    char extra[32];
    snprintf(extra, sizeof(extra), "\"bytes\":%d", bytes);
    lp_event_log_with(LP_EVENT_WARN, "scene.rejected", extra, "Ignored a scene: %s", why);
}

static void on_data(const esp_mqtt_event_handle_t event)
{
    // Payloads arrive whole for a scene-sized message, but the API permits
    // fragmentation, and a truncated JSON document parses as garbage rather
    // than failing loudly.
    if (event->data_len != event->total_data_len) {
        ESP_LOGW(TAG, "ignoring fragmented payload (%d of %d bytes)",
                 event->data_len, event->total_data_len);
        // Only for the first piece, or one oversized scene is several events.
        if (event->current_data_offset == 0) {
            scene_rejected("it arrived in pieces", event->total_data_len);
        }
        return;
    }
    if (event->total_data_len > LP_MAX_SCENE_BYTES) {
        ESP_LOGW(TAG, "ignoring oversized payload (%d bytes)", event->total_data_len);
        scene_rejected("it is larger than a scene can be", event->total_data_len);
        return;
    }

    lp_scene_t scene;
    if (!lp_scene_parse(event->data, (size_t)event->data_len, &scene)) {
        // Deliberately not clearing the strip: a stand that goes dark on a bad
        // payload is worse than one that keeps showing the last good scene.
        scene_rejected("it is not a scene this firmware can render", event->data_len);
        return;
    }

    ESP_LOGI(TAG, "scene: %s, %u colours, brightness %.2f",
             lp_effect_name(scene.effect), (unsigned)scene.palette_len, scene.brightness);

    if (scene_handler != NULL) {
        scene_handler(&scene);
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
        // This fires for every failed attempt as well as for a lost
        // connection. Only the loss is an event; it waits in the queue and is
        // delivered when the broker is back.
        if (connected) {
            connected = false;
            lp_event_log(LP_EVENT_WARN, "mqtt.disconnected", "Lost the broker");
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
             lp_wifi_rssi(),
             (unsigned)heap_caps_get_free_size(MALLOC_CAP_DEFAULT),
             (unsigned)heap_caps_get_minimum_free_size(MALLOC_CAP_DEFAULT));
    lp_event_log_with(LP_EVENT_INFO, "stand.health", extra, "Health report");
}

// log_task carries queued events to the stand's log topic. It takes one only
// while the broker is there, so events recorded offline wait in their queue
// rather than being handed to a client that would discard them - and it is a
// task of its own because the render task must never wait on the network.
static void log_task(void *arg)
{
    (void)arg;

    static char line[LP_EVENT_LINE_LEN];
    TickType_t last_health = xTaskGetTickCount();

    for (;;) {
        if (!connected) {
            vTaskDelay(pdMS_TO_TICKS(250));
            continue;
        }

        if (xTaskGetTickCount() - last_health >= pdMS_TO_TICKS(LP_HEALTH_INTERVAL_MS)) {
            last_health = xTaskGetTickCount();
            log_health();
        }

        if (!lp_event_log_receive(line, sizeof(line), pdMS_TO_TICKS(250))) {
            continue;
        }

        // QoS 1 and not retained: each event should arrive once, and a log
        // line replayed to every new subscriber would be an event that never
        // happened again.
        if (esp_mqtt_client_publish(client, LP_TOPIC_LOG, line, 0, 1, 0) < 0) {
            ESP_LOGW(TAG, "could not publish an event: %s", line);
        }
    }
}

esp_err_t lp_mqtt_start(lp_scene_handler on_scene)
{
    scene_handler = on_scene;

    esp_mqtt_client_config_t config = {
        .broker.address.uri = lp_cred_mqtt_uri(),
        .credentials = {
            .username = lp_cred_mqtt_username(),
            .client_id = LP_STAND_ID,
            .authentication.password = lp_cred_mqtt_password(),
        },
        // The will is how the room learns this stand died rather than simply
        // went quiet.
        .session.last_will = {
            .topic = LP_TOPIC_STATUS,
            .msg = LP_STATUS_OFFLINE,
            .qos = 1,
            .retain = 1,
        },
        .network.reconnect_timeout_ms = 5000,
    };

    // Verify the broker against the provisioned CA. Per-device client
    // certificates are the intended end state (see the roadmap); this is the
    // simpler first step. With no CA an mqtts:// connection fails its
    // handshake, which is the right answer to "who am I talking to?".
    config.broker.verification.certificate = lp_cred_mqtt_ca();

    client = esp_mqtt_client_init(&config);
    if (client == NULL) {
        return ESP_FAIL;
    }

    ESP_ERROR_CHECK(esp_mqtt_client_register_event(client, ESP_EVENT_ANY_ID,
                                                   &on_mqtt_event, NULL));

    // Below the render task: a late log line is nothing, a late frame shows.
    if (xTaskCreate(log_task, "events", 4096, NULL, 3, NULL) != pdPASS) {
        ESP_LOGW(TAG, "no event task; the stand will run without reporting events");
    }

    ESP_LOGI(TAG, "dialling %s as %s; events on %s",
             lp_cred_mqtt_uri(), lp_cred_mqtt_username(), LP_TOPIC_LOG);
    return esp_mqtt_client_start(client);
}

void lp_mqtt_publish_state(const lp_scene_t *scene)
{
    if (client == NULL || scene == NULL) {
        return;
    }

    char payload[128];
    int written = snprintf(payload, sizeof(payload),
                           "{\"v\":%d,\"effect\":\"%s\",\"colors\":%u,\"brightness\":%.2f}",
                           LP_SCENE_CONTRACT_VERSION, lp_effect_name(scene->effect),
                           (unsigned)scene->palette_len, scene->brightness);
    if (written <= 0 || written >= (int)sizeof(payload)) {
        return;
    }
    esp_mqtt_client_publish(client, LP_TOPIC_STATE, payload, written, 1, 1);
}

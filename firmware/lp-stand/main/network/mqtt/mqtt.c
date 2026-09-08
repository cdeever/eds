#include "mqtt.h"

#include <stdio.h>

#include "esp_log.h"
#include "lp_config.h"
#include "mqtt_client.h"
#include "scene_json.h"

static const char *TAG = "mqtt";

static esp_mqtt_client_handle_t client;
static lp_scene_handler scene_handler;

static void on_connected(void)
{
    ESP_LOGI(TAG, "connected; subscribing to %s", LP_TOPIC_SCENE);

    // QoS 1. A dropped scene means the stand shows the previous record's
    // colours until the next one, which is exactly the sort of quiet wrongness
    // that is hard to notice and annoying to debug.
    esp_mqtt_client_subscribe(client, LP_TOPIC_SCENE, 1);

    // Presence, retained, replacing the will the broker would have published.
    esp_mqtt_client_publish(client, LP_TOPIC_STATUS, LP_STATUS_ONLINE, 0, 1, 1);
}

static void on_data(const esp_mqtt_event_handle_t event)
{
    // Payloads arrive whole for a scene-sized message, but the API permits
    // fragmentation, and a truncated JSON document parses as garbage rather
    // than failing loudly.
    if (event->data_len != event->total_data_len) {
        ESP_LOGW(TAG, "ignoring fragmented payload (%d of %d bytes)",
                 event->data_len, event->total_data_len);
        return;
    }
    if (event->total_data_len > LP_MAX_SCENE_BYTES) {
        ESP_LOGW(TAG, "ignoring oversized payload (%d bytes)", event->total_data_len);
        return;
    }

    lp_scene_t scene;
    if (!lp_scene_parse(event->data, (size_t)event->data_len, &scene)) {
        // Deliberately not clearing the strip: a stand that goes dark on a bad
        // payload is worse than one that keeps showing the last good scene.
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
        break;
    case MQTT_EVENT_ERROR:
        ESP_LOGE(TAG, "transport error");
        break;
    default:
        break;
    }
}

esp_err_t lp_mqtt_start(lp_scene_handler on_scene)
{
    scene_handler = on_scene;

    esp_mqtt_client_config_t config = {
        .broker.address.uri = CONFIG_LP_MQTT_URI,
        .credentials = {
            .username = CONFIG_LP_MQTT_USERNAME,
            .client_id = LP_STAND_ID,
            .authentication.password = CONFIG_LP_MQTT_PASSWORD,
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

#if CONFIG_LP_MQTT_USE_TLS
    // The CA is embedded at build time. Per-device client certificates are the
    // intended end state (see the roadmap); this is the simpler first step.
    extern const uint8_t mqtt_ca_pem_start[] asm("_binary_mqtt_ca_pem_start");
    config.broker.verification.certificate = (const char *)mqtt_ca_pem_start;
#endif

    client = esp_mqtt_client_init(&config);
    if (client == NULL) {
        return ESP_FAIL;
    }

    ESP_ERROR_CHECK(esp_mqtt_client_register_event(client, ESP_EVENT_ANY_ID,
                                                   &on_mqtt_event, NULL));
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

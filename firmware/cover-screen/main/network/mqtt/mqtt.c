#include "mqtt.h"

#include <stdio.h>
#include <string.h>

#include "cs_config.h"
#include "cs_credentials.h"
#include "esp_heap_caps.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "mqtt_client.h"
#include "now_json.h"
#include "screen.h"
#include "wifi.h"

static const char *TAG = "mqtt";

static esp_mqtt_client_handle_t client;
static volatile bool connected;

// When the broker last answered anything, and the question a command asks
// before it is sent: see cs_mqtt_send_command.
static volatile int64_t heard_us;
static volatile int probe_id = -1;
static SemaphoreHandle_t probe_answered;

// A broker heard from this recently is taken to be there.
#define FRESH_US (3 * 1000 * 1000)
// How long a command waits to find out. Longer than a round trip on a bad
// day, shorter than a finger's patience.
#define PROBE_WAIT_MS 800

// A message longer than the client's buffer arrives in pieces, and only the
// first says which topic it is on. This remembers.
static enum { NOTHING, CURRENT, COVER, IGNORED } assembling;
static char current[CS_MAX_CURRENT_BYTES];

bool cs_mqtt_connected(void)
{
    return connected;
}

static void on_connected(void)
{
    ESP_LOGI(TAG, "connected; subscribing to %s and %s", CS_TOPIC_CURRENT, CS_TOPIC_COVER);

    // QoS 1. A dropped track change leaves the last album's cover on the
    // screen until the next one, with nothing to say it is wrong.
    esp_mqtt_client_subscribe(client, CS_TOPIC_CURRENT, 1);
    esp_mqtt_client_subscribe(client, CS_TOPIC_COVER, 1);

    // Presence, retained, replacing the will the broker would have published.
    esp_mqtt_client_publish(client, CS_TOPIC_STATUS, CS_STATUS_ONLINE, 0, 1, 1);

    char extra[64];
    snprintf(extra, sizeof(extra), "\"ip\":\"%s\",\"rssi\":%d", cs_wifi_ip(), cs_wifi_rssi());
    cs_event_log_with(CS_EVENT_INFO, "mqtt.connected", extra, "Connected to the broker");
    heard_us = esp_timer_get_time();
    connected = true;
    cs_screen_link(CS_LINK_UP);
}

static void on_current(const char *json, size_t len)
{
    cs_now_t now;
    if (!cs_now_parse(json, len, &now)) {
        // The screen keeps showing what it was showing, which from the room
        // looks like nothing having been sent. So it is said here.
        char extra[32];
        snprintf(extra, sizeof(extra), "\"bytes\":%u", (unsigned)len);
        cs_event_log_with(CS_EVENT_WARN, "current.rejected", extra,
                          "Ignored a track: it is not one this firmware reads");
        return;
    }
    ESP_LOGI(TAG, "current: %s, %s%s", now.play == CS_PLAY_PLAYING ? "playing" : now.play == CS_PLAY_PAUSED ? "paused" : "stopped",
             now.has_art ? "with a cover" : "no cover", now.no_controls ? ", no controls" : "");
    cs_screen_playing(now.play, now.has_art, !now.no_controls);
}

static bool topic_is(const esp_mqtt_event_handle_t event, const char *topic)
{
    size_t len = strlen(topic);
    return (size_t)event->topic_len == len && memcmp(event->topic, topic, len) == 0;
}

static void on_data(const esp_mqtt_event_handle_t event)
{
    size_t offset = (size_t)event->current_data_offset;
    size_t len = (size_t)event->data_len;
    size_t total = (size_t)event->total_data_len;

    if (offset == 0) {
        if (topic_is(event, CS_TOPIC_COVER)) {
            assembling = cs_screen_cover_begin(total) ? COVER : IGNORED;
            if (assembling == COVER) {
                ESP_LOGI(TAG, "cover: %u bytes", (unsigned)total);
            }
        } else if (topic_is(event, CS_TOPIC_CURRENT)) {
            if (total > sizeof(current)) {
                assembling = IGNORED;
                char extra[32];
                snprintf(extra, sizeof(extra), "\"bytes\":%u", (unsigned)total);
                cs_event_log_with(CS_EVENT_WARN, "current.rejected", extra,
                                  "Ignored a track: it is larger than one can be");
            } else {
                assembling = CURRENT;
            }
        } else {
            assembling = IGNORED;
        }
    }

    switch (assembling) {
    case COVER:
        cs_screen_cover_data(offset, (const uint8_t *)event->data, len);
        if (offset + len >= total) {
            cs_screen_cover_end();
            assembling = NOTHING;
        }
        break;
    case CURRENT:
        if (offset + len <= sizeof(current)) {
            memcpy(current + offset, event->data, len);
            if (offset + len >= total) {
                on_current(current, total);
                assembling = NOTHING;
            }
        } else {
            assembling = IGNORED;
        }
        break;
    default:
        break;
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
        heard_us = esp_timer_get_time();
        on_data(event);
        break;
    case MQTT_EVENT_PUBLISHED:
        // The broker acknowledged something: it is there, now.
        heard_us = esp_timer_get_time();
        if (event->msg_id == probe_id && probe_answered != NULL) {
            xSemaphoreGive(probe_answered);
        }
        break;
    case MQTT_EVENT_DISCONNECTED:
        ESP_LOGW(TAG, "disconnected; the client will keep retrying");
        // Half a cover is no cover, and the rest of it is not coming.
        if (assembling == COVER) {
            cs_screen_cover_end();
        }
        assembling = NOTHING;
        cs_screen_link(cs_wifi_connected() ? CS_LINK_NO_BROKER : CS_LINK_NO_WIFI);
        // This fires for every failed attempt as well as for a lost
        // connection. Only the loss is an event.
        if (connected) {
            connected = false;
            cs_event_log(CS_EVENT_WARN, "mqtt.disconnected", "Lost the broker");
        }
        break;
    case MQTT_EVENT_ERROR:
        // Serial only: none of this can be published, by definition.
        if (event->error_handle->error_type == MQTT_ERROR_TYPE_CONNECTION_REFUSED) {
            ESP_LOGE(TAG, "the broker refused the connection (code %d)",
                     event->error_handle->connect_return_code);
        } else {
            ESP_LOGE(TAG, "transport error: esp_err 0x%x, tls 0x%x, errno %d (heap free %u, largest %u)",
                     (unsigned)event->error_handle->esp_tls_last_esp_err,
                     (unsigned)event->error_handle->esp_tls_stack_err,
                     event->error_handle->esp_transport_sock_errno,
                     (unsigned)heap_caps_get_free_size(MALLOC_CAP_DEFAULT),
                     (unsigned)heap_caps_get_largest_free_block(MALLOC_CAP_DEFAULT));
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
             cs_wifi_rssi(),
             (unsigned)heap_caps_get_free_size(MALLOC_CAP_DEFAULT),
             (unsigned)heap_caps_get_minimum_free_size(MALLOC_CAP_DEFAULT));
    cs_event_log_with(CS_EVENT_INFO, "screen.health", extra, "Health report");
}

// log_task carries queued events to the screen's log topic, only while the
// broker is there, so events recorded offline wait in their queue. It is
// also what notices the network going: the broker client only says that it
// lost the broker, not why.
static void log_task(void *arg)
{
    (void)arg;

    static char line[CS_EVENT_LINE_LEN];
    TickType_t last_health = xTaskGetTickCount();
    TickType_t last_presence = xTaskGetTickCount();

    for (;;) {
        if (!connected) {
            cs_screen_link(cs_wifi_connected() ? CS_LINK_NO_BROKER : CS_LINK_NO_WIFI);
            vTaskDelay(pdMS_TO_TICKS(250));
            continue;
        }

        if (xTaskGetTickCount() - last_health >= pdMS_TO_TICKS(CS_HEALTH_INTERVAL_MS)) {
            last_health = xTaskGetTickCount();
            log_health();
        }

        // QoS 0: a repeat that is lost is followed by another in thirty
        // seconds, and nothing should queue up behind one.
        if (xTaskGetTickCount() - last_presence >= pdMS_TO_TICKS(CS_PRESENCE_INTERVAL_MS)) {
            last_presence = xTaskGetTickCount();
            esp_mqtt_client_publish(client, CS_TOPIC_STATUS, CS_STATUS_ONLINE, 0, 0, 1);
        }

        if (!cs_event_log_receive(line, sizeof(line), pdMS_TO_TICKS(250))) {
            continue;
        }

        // QoS 1 and not retained: each event should arrive once.
        if (esp_mqtt_client_publish(client, CS_TOPIC_LOG, line, 0, 1, 0) < 0) {
            ESP_LOGW(TAG, "could not publish an event: %s", line);
        }
    }
}

esp_err_t cs_mqtt_start(void)
{
    esp_mqtt_client_config_t config = {
        .broker.address.uri = cs_cred_mqtt_uri(),
        .credentials = {
            .username = cs_cred_mqtt_username(),
            .client_id = CS_SCREEN_ID,
            .authentication.password = cs_cred_mqtt_password(),
        },
        .session.last_will = {
            .topic = CS_TOPIC_STATUS,
            .msg = CS_STATUS_OFFLINE,
            .qos = 1,
            .retain = 1,
        },
        .network.reconnect_timeout_ms = 5000,
    };

    // Verify the broker against the provisioned CA. With none, an mqtts://
    // connection fails its handshake, which is the right answer to "who am
    // I talking to?".
    config.broker.verification.certificate = cs_cred_mqtt_ca();

    client = esp_mqtt_client_init(&config);
    if (client == NULL) {
        return ESP_FAIL;
    }
    probe_answered = xSemaphoreCreateBinary();

    ESP_ERROR_CHECK(esp_mqtt_client_register_event(client, ESP_EVENT_ANY_ID,
                                                   &on_mqtt_event, NULL));

    if (xTaskCreate(log_task, "events", 4096, NULL, 3, NULL) != pdPASS) {
        ESP_LOGW(TAG, "no event task; the screen will run without reporting events");
    }

    ESP_LOGI(TAG, "dialing %s as %s; events on %s",
             cs_cred_mqtt_uri(), cs_cred_mqtt_username(), CS_TOPIC_LOG);
    return esp_mqtt_client_start(client);
}

// broker_is_there finds out whether the broker answers, now.
//
// "Connected" is not that. A connection that has stopped carrying anything
// still takes what is written to it, holds it, and delivers it when it
// recovers - which the first evening this screen ran was ten seconds and
// eight button presses later, all at once (CHG-0012). So before a command
// is written, something harmless is: the screen's own presence, again, with
// an acknowledgment asked for. If that comes back, the command goes. If it
// does not, the command is never written, and there is nothing to arrive
// late but the word "online".
static bool broker_is_there(void)
{
    if (esp_timer_get_time() - heard_us < FRESH_US) {
        return true;
    }
    if (probe_answered == NULL) {
        return false;
    }

    xSemaphoreTake(probe_answered, 0);
    probe_id = esp_mqtt_client_publish(client, CS_TOPIC_STATUS, CS_STATUS_ONLINE, 0, 1, 1);
    bool answered = probe_id >= 0 && xSemaphoreTake(probe_answered, pdMS_TO_TICKS(PROBE_WAIT_MS)) == pdTRUE;
    probe_id = -1;
    return answered;
}

bool cs_mqtt_send_command(cs_button_t button)
{
    if (client == NULL || !connected) {
        return false;
    }

    char payload[96];
    size_t len = cs_command_format(payload, sizeof(payload), button, CS_SCREEN_ID);
    if (len == 0) {
        return false;
    }

    if (!broker_is_there()) {
        // Say so on the screen, and start again with a connection that
        // works: this one is holding whatever was written to it.
        ESP_LOGW(TAG, "the broker did not answer; %s was not sent", cs_button_command(button));
        connected = false;
        cs_screen_link(cs_wifi_connected() ? CS_LINK_NO_BROKER : CS_LINK_NO_WIFI);
        cs_event_log(CS_EVENT_WARN, "mqtt.stalled", "The broker stopped answering");
        esp_mqtt_client_disconnect(client);
        esp_mqtt_client_reconnect(client);
        return false;
    }

    // QoS 0 and not retained, both on purpose. At QoS 1 the client keeps an
    // unacknowledged message and sends it again when the broker comes back,
    // which is a "next" arriving a minute after the finger left. A press
    // that is lost is pressed again; one that arrives late cannot be undone.
    return esp_mqtt_client_publish(client, CS_TOPIC_COMMAND, payload, (int)len, 0, 0) >= 0;
}

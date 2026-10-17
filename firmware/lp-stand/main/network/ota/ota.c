#include "ota.h"

#include <stdio.h>
#include <string.h>

#include "cJSON.h"
#include "esp_app_desc.h"
#include "esp_log.h"
#include "esp_ota_ops.h"
#include "esp_system.h"
#include "esp_timer.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "freertos/task.h"
#include "lp_config.h"
#include "ota_receive.h"
#include "sdkconfig.h"

// Without rollback a build that cannot reach the broker stays, and the stand
// needs its cable again: the one thing this is for. An sdkconfig made before
// ADR-0035 has it off, and sdkconfig.defaults does not override it.
#if !CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE
#error "ADR-0035 needs CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE. This sdkconfig predates it: delete sdkconfig and build again."
#endif

static const char *TAG = "ota";

static SemaphoreHandle_t lock;
static lp_ota_t rx;
static esp_ota_handle_t handle;
static const esp_partition_t *slot;
static TickType_t last_heard;

// A message longer than the client's buffer arrives in pieces, and only the
// first says which topic it is on. This remembers.
static enum { NOTHING, OFFER, CHUNK } assembling;

static volatile bool on_probation;
static esp_timer_handle_t probation_timer;

static void elf_of(const uint8_t sha[32], char out[17])
{
    for (int i = 0; i < 8; i++) {
        snprintf(out + 2 * i, 3, "%02x", sha[i]);
    }
}

static void say(esp_mqtt_client_handle_t client, const char *state, const uint32_t *next,
                const char *version, const char *elf, const char *why)
{
    char line[256];
    size_t n = lp_ota_state_format(line, sizeof(line), state, next, version, elf, why);
    if (n > 0) {
        // QoS 1 and never retained: an answer is for the sender listening
        // now, and a stale one would answer a question nobody asked.
        esp_mqtt_client_publish(client, LP_TOPIC_OTA_STATE, line, (int)n, 1, 0);
    }
}

// give_up abandons the build being received, and says why. Holds the lock.
static void give_up(esp_mqtt_client_handle_t client, const char *why)
{
    if (rx.receiving) {
        esp_ota_abort(handle);
    }
    lp_ota_abandon(&rx);
    ESP_LOGW(TAG, "refused a build: %s", why);
    say(client, "refused", NULL, NULL, NULL, why);
    char extra[160];
    char safe[128];
    lp_event_escape(why, safe, sizeof(safe));
    snprintf(extra, sizeof(extra), "\"why\":\"%s\"", safe);
    lp_event_log_with(LP_EVENT_WARN, "ota.refused", extra, "Refused a build");
}

static void reboot_task(void *arg)
{
    esp_mqtt_client_handle_t client = (esp_mqtt_client_handle_t)arg;
    // Long enough for the answer and the event to go out.
    vTaskDelay(pdMS_TO_TICKS(1500));
    // Cleanly, so the broker sends no will: the stand is gone for seconds,
    // not offline, and its new build says online again (ADR-0035). Stopped,
    // not merely disconnected: esp_mqtt_client_disconnect only asks the
    // client's task to, later, and the restart came first, so the broker
    // sent the will when the new build took the client id over (CHG-0025).
    // Stopping from this task sends the DISCONNECT before it returns.
    esp_mqtt_client_stop(client);
    vTaskDelay(pdMS_TO_TICKS(100));
    esp_restart();
}

// finish checks the build whole and boots it. Holds the lock.
static void finish(esp_mqtt_client_handle_t client)
{
    lp_ota_abandon(&rx); // the handle is closed below, either way

    // esp_ota_end checks the image: its header, its segments and the hash
    // the build appended. A build cut short or corrupted stops here.
    esp_err_t err = esp_ota_end(handle);
    if (err != ESP_OK) {
        char why[96];
        snprintf(why, sizeof(why), "the image is not whole (%s)", esp_err_to_name(err));
        give_up(client, why);
        return;
    }

    esp_app_desc_t desc;
    const esp_app_desc_t *own = esp_app_get_description();
    if (esp_ota_get_partition_description(slot, &desc) != ESP_OK ||
        strncmp(desc.project_name, own->project_name, sizeof(desc.project_name)) != 0) {
        give_up(client, "the image is not this firmware's");
        return;
    }

    err = esp_ota_set_boot_partition(slot);
    if (err != ESP_OK) {
        char why[96];
        snprintf(why, sizeof(why), "could not boot it (%s)", esp_err_to_name(err));
        give_up(client, why);
        return;
    }

    char elf[17];
    elf_of(desc.app_elf_sha256, elf);
    ESP_LOGI(TAG, "booting %s (%s) from %s", desc.version, elf, slot->label);
    say(client, "rebooting", NULL, desc.version, elf, NULL);
    char extra[128];
    snprintf(extra, sizeof(extra), "\"version\":\"%.32s\",\"elf\":\"%s\"", desc.version, elf);
    lp_event_log_with(LP_EVENT_INFO, "ota.applied", extra, "Restarting into firmware %.32s",
                      desc.version);

    if (xTaskCreate(reboot_task, "reboot", 3072, client, 4, NULL) != pdPASS) {
        esp_restart();
    }
}

static void on_offer(esp_mqtt_client_handle_t client, const char *json, size_t len)
{
    cJSON *offer = cJSON_ParseWithLength(json, len);
    const cJSON *v = cJSON_GetObjectItemCaseSensitive(offer, "v");
    const cJSON *project = cJSON_GetObjectItemCaseSensitive(offer, "project");
    const cJSON *size = cJSON_GetObjectItemCaseSensitive(offer, "size");
    const cJSON *elf = cJSON_GetObjectItemCaseSensitive(offer, "elf");

    const esp_app_desc_t *own = esp_app_get_description();
    char why[96] = "";

    if (offer == NULL || !cJSON_IsNumber(v) || v->valueint != LP_OTA_CONTRACT_VERSION ||
        !cJSON_IsString(project) || !cJSON_IsNumber(size) || size->valuedouble < 1 ||
        size->valuedouble > (double)UINT32_MAX) {
        snprintf(why, sizeof(why), "it is not an offer this firmware reads");
    } else if (strncmp(project->valuestring, own->project_name, sizeof(own->project_name)) != 0) {
        snprintf(why, sizeof(why), "it is %.32s's, and this is %.32s", project->valuestring,
                 own->project_name);
    }

    if (why[0] == '\0') {
        if (rx.receiving) {
            // A new offer abandons the one before: a sender that started
            // again is the only one there is.
            esp_ota_abort(handle);
            lp_ota_abandon(&rx);
        }
        slot = esp_ota_get_next_update_partition(NULL);
        const uint32_t bytes = (uint32_t)size->valuedouble;
        if (slot == NULL) {
            snprintf(why, sizeof(why), "there is no slot to write it to");
        } else if (!lp_ota_begin(&rx, bytes, (uint32_t)slot->size)) {
            snprintf(why, sizeof(why), "it is larger than the slot");
        } else {
            // Erased as it is written: the whole slot at once would hold the
            // client for seconds, and the broker would give up on it.
            esp_err_t err = esp_ota_begin(slot, OTA_WITH_SEQUENTIAL_WRITES, &handle);
            if (err != ESP_OK) {
                lp_ota_abandon(&rx);
                snprintf(why, sizeof(why), "could not open the slot (%s)", esp_err_to_name(err));
            }
        }
    }

    if (why[0] != '\0') {
        give_up(client, why);
    } else {
        last_heard = xTaskGetTickCount();
        ESP_LOGI(TAG, "receiving %lu bytes into %s", (unsigned long)rx.size, slot->label);
        // The hash is the sender's word, and goes into a line of JSON.
        char safe[24];
        lp_event_escape(cJSON_IsString(elf) ? elf->valuestring : "", safe, sizeof(safe));
        char extra[96];
        snprintf(extra, sizeof(extra), "\"bytes\":%lu,\"elf\":\"%s\"", (unsigned long)rx.size, safe);
        lp_event_log_with(LP_EVENT_INFO, "ota.offered", extra, "Receiving a build");
        const uint32_t zero = 0;
        say(client, "ready", &zero, NULL, NULL, NULL);
    }
    cJSON_Delete(offer);
}

static void on_chunk(esp_mqtt_client_handle_t client, const esp_mqtt_event_handle_t event)
{
    lp_ota_piece_t piece = lp_ota_piece(&rx, (const uint8_t *)event->data,
                                        (size_t)event->data_len,
                                        (size_t)event->current_data_offset,
                                        (size_t)event->total_data_len);
    if (!rx.receiving) {
        return;
    }
    last_heard = xTaskGetTickCount();

    if (piece.bad) {
        give_up(client, "it is longer than was offered");
        return;
    }
    if (piece.len > 0) {
        esp_err_t err = esp_ota_write(handle, piece.data, piece.len);
        if (err != ESP_OK) {
            char why[96];
            snprintf(why, sizeof(why), "could not write it (%s)", esp_err_to_name(err));
            give_up(client, why);
            return;
        }
        lp_ota_wrote(&rx, piece.len);
    }
    if (piece.message_done) {
        if (lp_ota_complete(&rx)) {
            finish(client);
        } else {
            const uint32_t next = rx.next;
            say(client, "receiving", &next, NULL, NULL, NULL);
        }
    }
}

static bool topic_is(const esp_mqtt_event_handle_t event, const char *topic)
{
    size_t len = strlen(topic);
    return (size_t)event->topic_len == len && memcmp(event->topic, topic, len) == 0;
}

bool lp_ota_on_data(esp_mqtt_client_handle_t client, esp_mqtt_event_handle_t event)
{
    if (event->current_data_offset == 0) {
        assembling = topic_is(event, LP_TOPIC_OTA_CHUNK) ? CHUNK
                     : topic_is(event, LP_TOPIC_OTA)     ? OFFER
                                                         : NOTHING;
    }
    if (assembling == NOTHING || lock == NULL) {
        return false;
    }

    xSemaphoreTake(lock, portMAX_DELAY);
    if (assembling == CHUNK) {
        on_chunk(client, event);
    } else if (event->current_data_offset == 0) {
        // An offer is a line of JSON; one that does not arrive whole is not
        // one.
        if (event->data_len == event->total_data_len) {
            on_offer(client, event->data, (size_t)event->data_len);
        } else {
            give_up(client, "the offer arrived in pieces");
        }
    }
    xSemaphoreGive(lock);

    if (event->current_data_offset + event->data_len >= event->total_data_len) {
        assembling = NOTHING;
    }
    return true;
}

void lp_ota_tick(esp_mqtt_client_handle_t client)
{
    if (lock == NULL || !rx.receiving) {
        return;
    }
    xSemaphoreTake(lock, portMAX_DELAY);
    if (rx.receiving && xTaskGetTickCount() - last_heard >= pdMS_TO_TICKS(LP_OTA_STALL_MS)) {
        give_up(client, "nothing arrived for a minute");
    }
    xSemaphoreGive(lock);
}

static void probation_over(void *arg)
{
    (void)arg;
    if (!on_probation) {
        return;
    }
    // Serial only: not having reached the broker is the reason.
    ESP_LOGE(TAG, "this build has not reached the broker; going back to the one before it");
    esp_ota_mark_app_invalid_rollback_and_reboot();
}

void lp_ota_probation(void)
{
    lock = xSemaphoreCreateMutex();

    esp_ota_img_states_t state;
    const esp_partition_t *running = esp_ota_get_running_partition();
    if (esp_ota_get_state_partition(running, &state) != ESP_OK ||
        state != ESP_OTA_IMG_PENDING_VERIFY) {
        return;
    }
    on_probation = true;
    const esp_timer_create_args_t args = {.callback = probation_over, .name = "probation"};
    if (esp_timer_create(&args, &probation_timer) == ESP_OK) {
        esp_timer_start_once(probation_timer, (uint64_t)LP_OTA_CONFIRM_WITHIN_MS * 1000);
    }
    ESP_LOGW(TAG, "new build on %s: kept once it reaches the broker", running->label);
}

void lp_ota_on_connect(esp_mqtt_client_handle_t client)
{
    esp_mqtt_client_subscribe(client, LP_TOPIC_OTA, 1);
    esp_mqtt_client_subscribe(client, LP_TOPIC_OTA_CHUNK, 1);

    const esp_app_desc_t *own = esp_app_get_description();
    char elf[17];
    elf_of(own->app_elf_sha256, elf);

    if (on_probation) {
        on_probation = false;
        if (probation_timer != NULL) {
            esp_timer_stop(probation_timer);
        }
        // Reached the broker, its login accepted: everything the next update
        // will need. Kept.
        esp_ota_mark_app_valid_cancel_rollback();
        char extra[128];
        snprintf(extra, sizeof(extra), "\"version\":\"%.32s\",\"elf\":\"%s\"", own->version, elf);
        lp_event_log_with(LP_EVENT_INFO, "ota.kept", extra, "Kept firmware %.32s", own->version);
    }

    say(client, "running", NULL, own->version, elf, NULL);
}

// Updates through the broker, and the probation of a new build (ADR-0035).
//
// A build is offered on the beacon's ota topic and arrives in pieces on
// ota/chunk; the beacon writes it to its spare slot as it comes, answers on
// ota/state, and boots it. The new build is kept once it has reached the
// broker, and given up by itself if it has not within PB_OTA_CONFIRM_WITHIN_MS.
// Which bytes go where is app/ota (host tested); this is the flash and the
// broker.
#pragma once

#include <stdbool.h>

#include "mqtt_client.h"

// pb_ota_probation starts the clock on a build that has not yet reached the
// broker. Call it once at boot, before the broker is dialed.
void pb_ota_probation(void);

// pb_ota_on_connect subscribes to the beacon's ota topics, keeps a build on
// probation, and says what is running. Call it on every connection.
void pb_ota_on_connect(esp_mqtt_client_handle_t client);

// pb_ota_on_data takes one MQTT_EVENT_DATA. It returns true when the message
// was the ota topics', which nobody else need look at.
bool pb_ota_on_data(esp_mqtt_client_handle_t client, esp_mqtt_event_handle_t event);

// pb_ota_tick gives up a build that has stopped arriving. Call it often from
// a task that is not the MQTT client's.
void pb_ota_tick(esp_mqtt_client_handle_t client);

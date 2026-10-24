// MQTT: where a request goes, and what was made of it comes back.
//
// The device dials the broker on Deevnet IoTaaS's IoT Backend segment, as
// the stand and the screen do.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

// ea_mqtt_start connects, subscribes to what was heard, announces presence
// and starts forwarding the event log. Connecting happens in the background
// and is retried indefinitely.
esp_err_t ea_mqtt_start(void);

bool ea_mqtt_connected(void);

// ea_mqtt_send_request publishes a request: one WAV, at QoS 0 and never
// retained. It returns false, having sent nothing, when the broker is not
// there: a request is for now, and one delivered after a reconnect would
// play an album minutes after it was asked for. It blocks while the WAV is
// written, a second or so for a few seconds of speech.
bool ea_mqtt_send_request(const uint8_t *wav, size_t len);

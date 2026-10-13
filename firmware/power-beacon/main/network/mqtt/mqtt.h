// MQTT: the beacon's whole purpose.
//
// The beacon dials the broker on Deevnet IoTaaS's IoT Backend segment, as the
// stand does, says "online" and keeps the connection. It subscribes to
// nothing. What it is for is what the broker says when the connection goes:
// the will, "offline", which is how the room learns that the switch the
// beacon shares was turned off (ADR-0023).
#pragma once

#include "esp_err.h"

// pb_mqtt_start connects, announces presence and starts forwarding the event
// log to this beacon's log topic. It returns as soon as the client is
// started; connection happens in the background and is retried indefinitely.
esp_err_t pb_mqtt_start(void);

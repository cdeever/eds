// MQTT: where the track and its cover come from, and where a button press
// goes.
//
// The screen dials the broker on the substrate's IoT Backend segment, as the
// stand does and for the same reason: clients always initiate, and that is
// the segment built to accept them.
#pragma once

#include <stdbool.h>

#include "esp_err.h"
#include "ui.h"

// cs_mqtt_start connects, subscribes to the current track and its small
// cover, announces presence and starts forwarding the event log. It returns
// as soon as the client is started; connection happens in the background and
// is retried indefinitely.
esp_err_t cs_mqtt_start(void);

bool cs_mqtt_connected(void);

// cs_mqtt_send_command publishes a button press. It returns false, having
// sent nothing, when the broker is not there: a command is for now, and one
// delivered after a reconnect would skip a track nobody is asking to skip.
//
// "There" is checked, not assumed: if the broker has not been heard from in
// the last few seconds it is asked first, and the command waits up to most
// of a second for the answer. Call it from a task that can wait that long.
bool cs_mqtt_send_command(cs_button_t button);

// Station-mode WiFi.
//
// The screen lives on the substrate's IoT segment (VLAN 30), which is where the
// segmentation model puts "custom-developed embedded devices with controlled
// firmware". It takes a DHCP lease there and reaches the broker on IoT Backend.
#pragma once

#include <stdbool.h>

#include "esp_err.h"

// cs_wifi_start connects and returns once an IP has been obtained, or after
// the connection has failed enough times to be worth reporting. It keeps
// retrying in the background either way - a screen that gives up needs someone
// to walk over and power-cycle it.
esp_err_t cs_wifi_start(void);

// cs_wifi_rssi is the signal from the access point in dBm, or 0 when the
// screen is not associated.
int cs_wifi_rssi(void);

// cs_wifi_ip is the screen's address as text, or "" before it has one.
const char *cs_wifi_ip(void);

// cs_wifi_connected is whether the screen holds an address right now.
bool cs_wifi_connected(void);

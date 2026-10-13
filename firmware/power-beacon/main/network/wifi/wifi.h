// Station-mode WiFi.
//
// The beacon lives on Deevnet IoTaaS's IoT segment (VLAN 30), which is where the
// segmentation model puts "custom-developed embedded devices with controlled
// firmware". It takes a DHCP lease there and reaches the broker on IoT Backend.
#pragma once

#include <stdbool.h>

#include "esp_err.h"

// pb_wifi_start connects and returns once an IP has been obtained, or after
// the connection has failed enough times to be worth reporting. It keeps
// retrying in the background either way - a beacon that gives up needs someone
// to walk over and power-cycle it.
esp_err_t pb_wifi_start(void);

// pb_wifi_rssi is the signal from the access point in dBm, or 0 when the
// beacon is not associated.
int pb_wifi_rssi(void);

// pb_wifi_ip is the beacon's address as text, or "" before it has one.
const char *pb_wifi_ip(void);

// pb_wifi_connected is whether the beacon holds an address right now.
bool pb_wifi_connected(void);

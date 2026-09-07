// Station-mode WiFi.
//
// The stand lives on the substrate's IoT segment (VLAN 30), which is where the
// segmentation model puts "custom-developed embedded devices with controlled
// firmware". It takes a DHCP lease there and reaches the broker on IoT Backend.
#pragma once

#include "esp_err.h"

// lp_wifi_start connects and returns once an IP has been obtained, or after
// the connection has failed enough times to be worth reporting. It keeps
// retrying in the background either way - a stand that gives up needs someone
// to walk over and power-cycle it.
esp_err_t lp_wifi_start(void);

// Copied from firmware/cover-screen: change one, change the other.
//
// Station-mode WiFi.
//
// The device lives on Deevnet IoTaaS's IoT segment (VLAN 30), which is where the
// segmentation model puts "custom-developed embedded devices with controlled
// firmware". It takes a DHCP lease there and reaches the broker on IoT Backend.
#pragma once

#include <stdbool.h>

#include "esp_err.h"

// ea_wifi_start connects and returns once an IP has been obtained, or after
// the connection has failed enough times to be worth reporting. It keeps
// retrying in the background either way - a device that gives up needs someone
// to walk over and power-cycle it.
esp_err_t ea_wifi_start(void);

// ea_wifi_rssi is the signal from the access point in dBm, or 0 when the
// device is not associated.
int ea_wifi_rssi(void);

// ea_wifi_ip is the device's address as text, or "" before it has one.
const char *ea_wifi_ip(void);

// ea_wifi_connected is whether the device holds an address right now.
bool ea_wifi_connected(void);

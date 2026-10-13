// Compile-time defaults. Anything an owner would plausibly change lives in
// Kconfig (idf.py menuconfig) rather than here; this file holds the values
// that are properties of the program rather than of an installation.
#pragma once

#include "sdkconfig.h"

// --- identity and topics ---------------------------------------------------

#define PB_BEACON_ID    CONFIG_PB_BEACON_ID
#define PB_TOPIC_PREFIX CONFIG_PB_TOPIC_PREFIX

// The beacon's presence, with a last will. This topic is the whole of what
// the beacon is for: it shares a switch with something that cannot speak for
// itself, so "online" here means that thing has power, and the will means it
// has lost it (ADR-0023). What shares the switch is not said here, and the
// beacon does not know.
#define PB_TOPIC_STATUS PB_TOPIC_PREFIX "/beacon/" PB_BEACON_ID "/status"

// Where the beacon's key events go: `<tenant>/log/<device name>`,
// Deevnet IoTaaS's shape, as the stand's are.
#define PB_TOPIC_LOG    PB_TOPIC_PREFIX "/log/" PB_BEACON_ID

#define PB_STATUS_ONLINE  "online"
#define PB_STATUS_OFFLINE "offline"

// --- timing ----------------------------------------------------------------

// How long the broker goes without hearing from the beacon before it asks
// whether it is there. A broker publishes a will after one and a half of
// these, so this is what decides how soon the room learns the switch went
// off: fifteen seconds. The stand and the screen leave it at the library's
// two minutes, because nothing waits on their absence; everything here does.
//
// Shorter is not better without end. Each of these is a round trip on WiFi,
// and one missed is a beacon that says the turntable was turned off.
#define PB_MQTT_KEEPALIVE_S 10

// --- events ----------------------------------------------------------------

#define PB_EVENT_QUEUE_LEN 16

// How often the beacon reports its signal and free heap.
#define PB_HEALTH_INTERVAL_MS (5 * 60 * 1000)

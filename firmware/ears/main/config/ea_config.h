// Compile-time defaults. Anything an owner would plausibly change lives in
// Kconfig (idf.py menuconfig); this file holds what belongs to the program.
#pragma once

#include "sdkconfig.h"

// --- identity and topics ---------------------------------------------------

#define EA_DEVICE_ID    CONFIG_EA_DEVICE_ID
#define EA_TOPIC_PREFIX CONFIG_EA_TOPIC_PREFIX

// What the device sends: what followed the wake word, or what was said while
// its button was held, as one WAV (ADR-0042). Never retained: a request kept
// by the broker would be asked again by every reconnect.
#define EA_TOPIC_UTTERANCE EA_TOPIC_PREFIX "/voice/" EA_DEVICE_ID "/utterance"

// What voiced made of it, for a light.
#define EA_TOPIC_HEARD EA_TOPIC_PREFIX "/voice/" EA_DEVICE_ID "/heard"

// Its own presence, with a last will.
#define EA_TOPIC_STATUS EA_TOPIC_PREFIX "/voice/" EA_DEVICE_ID "/status"

// Its key events: `<tenant>/log/<device name>`, Deevnet IoTaaS's shape.
#define EA_TOPIC_LOG EA_TOPIC_PREFIX "/log/" EA_DEVICE_ID

#define EA_STATUS_ONLINE  "online"
#define EA_STATUS_OFFLINE "offline"

// --- a request ---------------------------------------------------------------

// The longest request kept, and the shortest sent: scribe takes 0.3 to 15
// seconds (ADR-0042). A press too short to be a sentence is a slip.
#define EA_REQUEST_MAX_MS 15000
#define EA_REQUEST_MIN_MS 400

// --- timing ----------------------------------------------------------------

// How often what the microphones are given is printed on the serial console,
// and how often the device reports its signal, heap and sound to the store.
#define EA_LEVEL_INTERVAL_MS  2000
#define EA_HEALTH_INTERVAL_MS (5 * 60 * 1000)

// How often the device says again that it is online (ADR-0034).
#define EA_PRESENCE_INTERVAL_MS (30 * 1000)

// --- events ----------------------------------------------------------------

#define EA_EVENT_QUEUE_LEN 16

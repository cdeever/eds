// Compile-time defaults. Anything an owner would plausibly change lives in
// Kconfig (idf.py menuconfig) rather than here; this file holds the values
// that are properties of the program rather than of an installation.
#pragma once

#include "sdkconfig.h"

// --- strip -----------------------------------------------------------------

#define LP_LED_COUNT       CONFIG_LP_LED_COUNT
#define LP_LED_GPIO        CONFIG_LP_LED_GPIO

// 60 FPS is smoother than the eye needs for these effects and still leaves the
// RMT peripheral idle most of the time.
#define LP_FRAME_INTERVAL_MS 16

// --- identity and topics ---------------------------------------------------

#define LP_STAND_ID     CONFIG_LP_STAND_ID
#define LP_TOPIC_PREFIX CONFIG_LP_TOPIC_PREFIX

// Topic layout mirrors lightd's. The stand owns its own status topic; lightd
// publishes its presence separately, because a dark stand and a dead publisher
// look identical from the room.
#define LP_TOPIC_SCENE  LP_TOPIC_PREFIX "/lightstand/" LP_STAND_ID "/scene"
#define LP_TOPIC_STATUS LP_TOPIC_PREFIX "/lightstand/" LP_STAND_ID "/status"
#define LP_TOPIC_STATE  LP_TOPIC_PREFIX "/lightstand/" LP_STAND_ID "/state"

// Where the stand's key events go. The shape is the substrate's, not EdS's:
// `<tenant>/log/<device name>`, the one topic under log/ this stand's broker
// account may publish to, which the substrate's bridge carries into the
// tenant's device log partition. So the prefix here has to be the tenant's
// name and the id the device's name in the registry - which they are.
#define LP_TOPIC_LOG    LP_TOPIC_PREFIX "/log/" LP_STAND_ID

#define LP_STATUS_ONLINE  "online"
#define LP_STATUS_OFFLINE "offline"

// A scene is small; anything larger is not one. Bounding this keeps a hostile
// or broken publisher from exhausting heap.
#define LP_MAX_SCENE_BYTES 2048

// --- events ----------------------------------------------------------------

// Events held while the broker is away. Sixteen covers a boot and a reconnect
// with room to spare; beyond that the oldest go, and the loss is counted.
#define LP_EVENT_QUEUE_LEN 16

// How often the stand reports its signal and free heap. These are the two
// numbers that drift on an unattended device, and the store has no metrics
// service - a number in a log line is how it gets graphed.
#define LP_HEALTH_INTERVAL_MS (5 * 60 * 1000)

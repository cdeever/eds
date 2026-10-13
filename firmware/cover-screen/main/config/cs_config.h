// Compile-time defaults. Anything an owner would plausibly change lives in
// Kconfig (idf.py menuconfig) rather than here; this file holds the values
// that are properties of the program rather than of an installation.
#pragma once

#include "sdkconfig.h"

// --- identity and topics ---------------------------------------------------

#define CS_SCREEN_ID    CONFIG_CS_SCREEN_ID
#define CS_TOPIC_PREFIX CONFIG_CS_TOPIC_PREFIX

// What the screen reads. Both are nowplayd's, and both are retained, so a
// screen that restarts mid-album finds the track and its cover waiting.
//
// The screen follows the current track and nothing else: it knows no player
// and no source, which is what lets another player take over without the
// screen being told (ADR-0006, ADR-0020).
#define CS_TOPIC_CURRENT CS_TOPIC_PREFIX "/nowplaying/current"
#define CS_TOPIC_COVER   CS_TOPIC_PREFIX "/nowplaying/current/art/small"

// What a button press is sent to. Never retained: a command the broker kept
// would skip a track for every program that connected afterward.
#define CS_TOPIC_COMMAND CS_TOPIC_PREFIX "/nowplaying/command"

// The screen's own presence, with a last will.
#define CS_TOPIC_STATUS  CS_TOPIC_PREFIX "/screen/" CS_SCREEN_ID "/status"

// Where the screen's key events go: `<tenant>/log/<device name>`,
// Deevnet IoTaaS's shape, as the stand's are.
#define CS_TOPIC_LOG     CS_TOPIC_PREFIX "/log/" CS_SCREEN_ID

#define CS_STATUS_ONLINE  "online"
#define CS_STATUS_OFFLINE "offline"

// --- sizes -----------------------------------------------------------------

// The largest cover the screen will hold. nowplayd aims for no more than
// this; one that is larger is refused, and the record is shown instead.
// Bounding it is also what keeps a broken publisher from exhausting the heap.
#define CS_MAX_COVER_BYTES 32768

// The current track is a few hundred bytes. Anything larger is not one.
#define CS_MAX_CURRENT_BYTES 2048

// --- timing ----------------------------------------------------------------

// How often the panel is asked whether it is being touched.
#define CS_TOUCH_POLL_MS 20

// How long a cover may take to arrive before the screen stops waiting for
// the rest of it and draws what it has.
#define CS_COVER_ARRIVAL_MS 3000

// --- events ----------------------------------------------------------------

#define CS_EVENT_QUEUE_LEN 16

// How often the screen reports its signal and free heap.
#define CS_HEALTH_INTERVAL_MS (5 * 60 * 1000)

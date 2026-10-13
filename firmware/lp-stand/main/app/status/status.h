// Status: what is wrong with the stand, said on the strip.
//
// A stand starts dark, and a dark stand that cannot reach the network would
// be no different from one that is switched off (ADR-0029). So when it
// cannot get as far as the broker it says how far it got, in red blinks on
// its first LED: the number of blinks is the step that failed.
//
// The rest of the strip is left alone. A stand that loses its network in the
// middle of a record goes on showing the record.
//
// Nothing here depends on ESP-IDF: it is given what the network says and the
// time, and is compiled and tested on the host.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "scene.h"

// The steps from power to a scene, in order. The value is the number of
// blinks, and is written down for people in the README: changing one changes
// what a person counting blinks is told.
typedef enum {
    LP_FAULT_NONE = 0,
    LP_FAULT_UNPROVISIONED = 1, // nothing in the creds partition to try with
    LP_FAULT_NO_WIFI = 2,       // provisioned, and not on the WiFi network
    LP_FAULT_NO_BROKER = 3,     // on the network, and the broker is not answering
    LP_FAULT_REFUSED = 4,       // the broker answered, and refused this stand's login
} lp_fault_t;

// What the network says of itself, as main reads it each frame.
typedef struct {
    bool provisioned; // credentials were read from flash
    bool wifi;        // associated, with an address
    bool broker;      // connected to the broker
    bool refused;     // the broker's last answer was a refusal
} lp_link_t;

// How long something must have been wrong before it is said. Joining the
// network and the broker takes several seconds from power on, and a
// connection that drops and comes back is not worth a red light.
#define LP_FAULT_GRACE_MS 20000u

// One blink, the gap after it, and the pause after the last of a count.
#define LP_FAULT_BLINK_ON_MS  200u
#define LP_FAULT_BLINK_OFF_MS 300u
#define LP_FAULT_PAUSE_MS     2000u

typedef struct {
    bool troubled;     // something is wrong now
    uint32_t since_ms; // and has been since then
} lp_status_t;

// lp_fault_of is the first step that has not been reached.
lp_fault_t lp_fault_of(const lp_link_t *link);

// lp_status_step takes what is wrong at t_ms and returns what to show: the
// fault, once something has been wrong for LP_FAULT_GRACE_MS without a
// break, and LP_FAULT_NONE before that. One fault turning into another is
// not a break; only everything working is.
lp_fault_t lp_status_step(lp_status_t *status, lp_fault_t fault, uint32_t t_ms);

// lp_status_lit reports whether the blink is on at t_ms for a fault: `fault`
// blinks, a pause, and again.
bool lp_status_lit(lp_fault_t fault, uint32_t t_ms);

// lp_status_overlay draws a fault over a rendered frame: the first LED is
// red while the blink is on and black while it is off, so the count reads
// the same over a lit scene as over a dark one. No fault, no change.
void lp_status_overlay(lp_fault_t fault, uint32_t t_ms, lp_rgb_t *frame, size_t count);

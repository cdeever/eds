#include "endpoint.h"

#include <stdbool.h>

// A quiet room through the XMOS chip, measured: -45 dB or so.
#define START_FLOOR_DB -50.0f
// The floor is never taken lower or higher than these: below, a burst of
// digital silence (MUTE) would make every breath speech; above, nothing could
// ever be heard over it.
#define FLOOR_MIN_DB -70.0f
#define FLOOR_MAX_DB -25.0f
// How far up the floor follows the room each block: about a tenth of the way
// over a second of 32 ms blocks.
#define FLOOR_RISE 0.003f

static float clampf(float v, float lo, float hi)
{
    return v < lo ? lo : v > hi ? hi : v;
}

// How much of each new block goes into the smoothed level: about a tenth of
// a second of 32 ms blocks.
#define SMOOTHING 0.4f

void ea_endpoint_init(ea_endpoint_t *e)
{
    e->floor_db = START_FLOOR_DB;
    ea_endpoint_start(e);
}

void ea_endpoint_idle(ea_endpoint_t *e, float level_db)
{
    if (level_db < e->floor_db) {
        e->floor_db = level_db;
    } else {
        e->floor_db += (level_db - e->floor_db) * FLOOR_RISE;
    }
    e->floor_db = clampf(e->floor_db, FLOOR_MIN_DB, FLOOR_MAX_DB);
}

void ea_endpoint_start(ea_endpoint_t *e)
{
    e->heard_ms = e->speech_ms = e->quiet_ms = e->last_speech_ms = 0;
    e->smooth_db = e->floor_db;
    e->loud_db = e->floor_db;
}

ea_endpoint_result_t ea_endpoint_feed(ea_endpoint_t *e, float level_db, uint32_t ms)
{
    e->heard_ms += ms;
    e->smooth_db += (level_db - e->smooth_db) * SMOOTHING;

    float speech_from = e->floor_db + EA_ENDPOINT_MARGIN_DB;
    float quiet_below = speech_from;
    if (e->speech_ms > 0 && e->loud_db - EA_ENDPOINT_BELOW_SPEECH_DB > quiet_below) {
        quiet_below = e->loud_db - EA_ENDPOINT_BELOW_SPEECH_DB;
    }

    bool speaking = e->speech_ms == 0 ? e->smooth_db >= speech_from : e->smooth_db >= quiet_below;
    if (speaking) {
        e->speech_ms += ms;
        e->quiet_ms = 0;
        e->last_speech_ms = e->heard_ms;
        if (e->smooth_db > e->loud_db) {
            e->loud_db = e->smooth_db;
        }
    } else if (e->speech_ms > 0) {
        e->quiet_ms += ms;
    }

    if (e->heard_ms >= EA_ENDPOINT_MAX_MS) {
        return e->speech_ms > 0 ? EA_ENDPOINT_DONE : EA_ENDPOINT_NOTHING;
    }
    if (e->speech_ms == 0) {
        return e->heard_ms >= EA_ENDPOINT_WAIT_MS ? EA_ENDPOINT_NOTHING : EA_ENDPOINT_LISTENING;
    }
    return e->quiet_ms >= EA_ENDPOINT_QUIET_MS ? EA_ENDPOINT_DONE : EA_ENDPOINT_LISTENING;
}

#include "endpoint.h"

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

void ea_endpoint_init(ea_endpoint_t *e)
{
    e->floor_db = START_FLOOR_DB;
    e->heard_ms = e->speech_ms = e->quiet_ms = 0;
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
    e->heard_ms = e->speech_ms = e->quiet_ms = 0;
}

ea_endpoint_result_t ea_endpoint_feed(ea_endpoint_t *e, float level_db, uint32_t ms)
{
    e->heard_ms += ms;
    if (level_db >= e->floor_db + EA_ENDPOINT_MARGIN_DB) {
        e->speech_ms += ms;
        e->quiet_ms = 0;
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

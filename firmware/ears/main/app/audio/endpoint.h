// When a request has been said.
//
// After the wake word the device hears what follows until the speaker stops:
// a request is "play X's first album" and a breath, not a fixed five seconds.
// Pure C on levels, no ESP-IDF, like pcm.c: a request cut off mid-name, or
// one that never ends because the room is loud, shows up only as "not
// understood", and is worth testing with a compiler.
//
// The level is each block's, in dB below full scale (ea_pcm_level). Speech is
// a margin above the room's floor, which is learned while nobody is asking.
#pragma once

#include <stdint.h>

// Speech is this far above the floor.
#define EA_ENDPOINT_MARGIN_DB 10.0f
// The quiet that ends a request, once it has begun.
#define EA_ENDPOINT_QUIET_MS 700
// How long to wait for a request to begin, after the wake word.
#define EA_ENDPOINT_WAIT_MS 3000
// The longest a request is heard, whatever the room does. scribe takes
// fifteen seconds; a request is never near that, and in a room loud enough
// that it never seems to end, six is plenty (2026-10-24: eight, all of it).
#define EA_ENDPOINT_MAX_MS 6000

typedef enum {
    EA_ENDPOINT_LISTENING, // keep hearing
    EA_ENDPOINT_DONE,      // said, and finished
    EA_ENDPOINT_NOTHING,   // nobody spoke after the wake word
} ea_endpoint_result_t;

typedef struct {
    float floor_db;     // the room, while nobody asks
    uint32_t heard_ms;  // since the request began to be heard
    uint32_t speech_ms; // of it, speech
    uint32_t quiet_ms;  // quiet since the last speech
} ea_endpoint_t;

// ea_endpoint_init starts with a quiet room's floor until one is learned.
void ea_endpoint_init(ea_endpoint_t *e);

// ea_endpoint_idle learns the floor from a block heard while nobody asks. It
// follows the room down at once and up slowly, so a word or a slammed door
// does not raise it but music that keeps playing does.
void ea_endpoint_idle(ea_endpoint_t *e, float level_db);

// ea_endpoint_start begins hearing a request.
void ea_endpoint_start(ea_endpoint_t *e);

// ea_endpoint_feed takes a block of ms milliseconds at level_db, and says
// whether the request is still being said.
ea_endpoint_result_t ea_endpoint_feed(ea_endpoint_t *e, float level_db, uint32_t ms);

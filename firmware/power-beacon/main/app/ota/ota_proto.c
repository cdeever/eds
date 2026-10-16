#include "ota_proto.h"

#include <stdio.h>
#include <string.h>

#include "event_format.h"

bool pb_ota_begin(pb_ota_t *o, uint32_t size, uint32_t capacity)
{
    memset(o, 0, sizeof(*o));
    if (size == 0 || size > capacity) {
        return false;
    }
    o->receiving = true;
    o->size = size;
    return true;
}

pb_ota_piece_t pb_ota_piece(pb_ota_t *o, const uint8_t *data, size_t len, size_t offset,
                            size_t total)
{
    pb_ota_piece_t piece = {0};
    if (!o->receiving) {
        return piece;
    }
    piece.message_done = offset + len >= total;

    if (offset == 0) {
        o->in_message = true;
        o->taking = false;
        if (len < PB_OTA_CHUNK_HEADER || total <= PB_OTA_CHUNK_HEADER) {
            // Too short to say where it goes. Answered, so the sender asks
            // again, but nothing is written.
            if (piece.message_done) {
                o->in_message = false;
            }
            return piece;
        }
        const uint32_t at = ((uint32_t)data[0] << 24) | ((uint32_t)data[1] << 16) |
                            ((uint32_t)data[2] << 8) | (uint32_t)data[3];
        const size_t body = total - PB_OTA_CHUNK_HEADER;
        if (at == o->next) {
            if ((uint64_t)o->next + body > o->size) {
                // More than the image that was offered: whatever this is, it
                // is not that image.
                piece.bad = true;
                o->in_message = false;
                return piece;
            }
            o->taking = true;
        }
        // Anything else is a piece sent twice, or out of turn: ignored, and
        // answered with the byte that is wanted.
        data += PB_OTA_CHUNK_HEADER;
        len -= PB_OTA_CHUNK_HEADER;
    } else if (!o->in_message) {
        // The rest of a message whose start was not seen: before this image
        // was offered, or across a reconnection.
        return piece;
    }

    if (o->taking) {
        piece.data = data;
        piece.len = len;
    }
    if (piece.message_done) {
        o->in_message = false;
        o->taking = false;
    }
    return piece;
}

void pb_ota_wrote(pb_ota_t *o, size_t n)
{
    o->next += (uint32_t)n;
}

bool pb_ota_complete(const pb_ota_t *o)
{
    return o->receiving && o->next == o->size;
}

void pb_ota_abandon(pb_ota_t *o)
{
    memset(o, 0, sizeof(*o));
}

size_t pb_ota_state_format(char *out, size_t cap, const char *state, const uint32_t *next,
                           const char *version, const char *elf, const char *why)
{
    char safe[96];
    size_t used = 0;
    int n;

#define APPEND(...)                                                                    \
    do {                                                                               \
        n = snprintf(out + used, cap - used, __VA_ARGS__);                             \
        if (n < 0 || (size_t)n >= cap - used) {                                        \
            return 0;                                                                  \
        }                                                                              \
        used += (size_t)n;                                                             \
    } while (0)

    if (out == NULL || cap == 0 || state == NULL) {
        return 0;
    }
    pb_event_escape(state, safe, sizeof(safe));
    APPEND("{\"v\":%d,\"state\":\"%s\"", PB_OTA_CONTRACT_VERSION, safe);
    if (next != NULL) {
        APPEND(",\"next\":%lu", (unsigned long)*next);
    }
    if (version != NULL) {
        pb_event_escape(version, safe, sizeof(safe));
        APPEND(",\"version\":\"%s\"", safe);
    }
    if (elf != NULL) {
        pb_event_escape(elf, safe, sizeof(safe));
        APPEND(",\"elf\":\"%s\"", safe);
    }
    if (why != NULL) {
        pb_event_escape(why, safe, sizeof(safe));
        APPEND(",\"why\":\"%s\"", safe);
    }
    APPEND("}");
#undef APPEND
    return used;
}

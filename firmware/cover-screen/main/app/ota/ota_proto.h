// Receiving a build through the broker, without the flash (ADR-0035).
//
// Pure C, like the event format: which bytes of which message are written
// where, and what the screen answers, decide whether an update works at all,
// and a mistake here is a screen that will not boot or will not take the next
// build. So it is tested with a compiler. The ESP-IDF half (network/ota)
// does the writing and the publishing. A copy of the stand's.
//
// A build arrives as many messages on the screen's ota/chunk topic, each four
// bytes of offset, big-endian, and then a piece of the image. A message
// longer than the MQTT client's buffer is itself handed over in pieces, of
// which only the first carries those four bytes. The screen takes only the
// piece that starts at the next byte it wants, and answers every message with
// that byte, so a sender that sends what it is asked for needs no other flow
// control and no other recovery.
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define CS_OTA_CONTRACT_VERSION 1

// The offset at the head of each chunk message.
#define CS_OTA_CHUNK_HEADER 4

typedef struct {
    bool receiving;
    uint32_t size; // the whole image, as offered
    uint32_t next; // the next byte wanted

    // The chunk message being handed over now.
    bool in_message;
    bool taking; // its bytes are the next ones, and are written
} cs_ota_t;

// What to do with one piece of a chunk message.
typedef struct {
    const uint8_t *data; // bytes to write at o->next, then cs_ota_wrote
    size_t len;          // 0 when there is nothing to write
    bool message_done;   // the last piece of a message: answer with o->next
    bool bad;            // it cannot be part of this image: give it up
} cs_ota_piece_t;

// cs_ota_begin starts receiving an image of size bytes into a slot of
// capacity bytes. It refuses an empty image and one that does not fit. A
// begin while receiving starts again.
bool cs_ota_begin(cs_ota_t *o, uint32_t size, uint32_t capacity);

// cs_ota_piece takes one piece of a chunk message: data and len as handed
// over, offset where the piece starts in the message, and total the message's
// whole length.
cs_ota_piece_t cs_ota_piece(cs_ota_t *o, const uint8_t *data, size_t len, size_t offset,
                            size_t total);

// cs_ota_wrote records that n bytes were written at o->next.
void cs_ota_wrote(cs_ota_t *o, size_t n);

// cs_ota_complete reports whether the whole image has been written.
bool cs_ota_complete(const cs_ota_t *o);

// cs_ota_abandon stops receiving.
void cs_ota_abandon(cs_ota_t *o);

// cs_ota_state_format writes the screen's answer on its ota/state topic and
// returns its length, or 0 if it does not fit in cap bytes. state is one of
// "ready", "receiving", "rebooting", "refused" or "running". next is given
// for the first three; version and elf for "running" and "rebooting", why
// for "refused". A NULL string leaves its field out.
size_t cs_ota_state_format(char *out, size_t cap, const char *state, const uint32_t *next,
                           const char *version, const char *elf, const char *why);

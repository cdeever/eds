// Host tests for the hardware-independent half of the firmware.
//
// The beacon does one thing a compiler can check: it writes event lines the
// tenant's log store has to be able to file. Those tests are the stand's,
// copied with the code they test (firmware/lp-stand). Everything else here is
// WiFi and MQTT, and needs a device.

#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

#include "event_format.h"
#include "ota_proto.h"

static int failures = 0;
static int checks = 0;

#define CHECK(cond, ...)                                                       \
    do {                                                                       \
        checks++;                                                              \
        if (!(cond)) {                                                         \
            failures++;                                                        \
            printf("  FAIL %s:%d: ", __func__, __LINE__);                      \
            printf(__VA_ARGS__);                                               \
            printf("\n");                                                      \
        }                                                                      \
    } while (0)

// well_formed checks what the log store needs of a line: one flat JSON object
// whose strings hold no raw control character, no bad escape and no broken
// UTF-8. Not a JSON parser - just enough to catch a line that would be filed
// as text with none of its fields.
static bool well_formed(const char *line)
{
    size_t n = strlen(line);
    if (n < 2 || line[0] != '{' || line[n - 1] != '}') {
        return false;
    }

    bool in_string = false;
    for (size_t i = 1; i + 1 < n; i++) {
        unsigned char c = (unsigned char)line[i];
        if (!in_string) {
            if (c == '"') {
                in_string = true;
            } else if (c == '{' || c == '}') {
                return false;
            }
            continue;
        }
        if (c < 0x20) {
            return false;
        }
        if (c == '\\') {
            char e = line[++i];
            if (e == 'u') {
                for (int k = 0; k < 4; k++) {
                    char h = line[++i];
                    if (!((h >= '0' && h <= '9') || (h >= 'a' && h <= 'f'))) {
                        return false;
                    }
                }
            } else if (e != '"' && e != '\\') {
                return false;
            }
        } else if (c == '"') {
            in_string = false;
        } else if (c >= 0x80) {
            int follow = (c & 0xE0) == 0xC0 ? 1 : (c & 0xF0) == 0xE0 ? 2 : (c & 0xF8) == 0xF0 ? 3 : -1;
            if (follow < 0) {
                return false;
            }
            for (int k = 0; k < follow; k++) {
                if (((unsigned char)line[++i] & 0xC0) != 0x80) {
                    return false;
                }
            }
        }
    }
    return !in_string;
}

static void test_event_is_the_line_the_store_expects(void)
{
    char line[PB_EVENT_LINE_LEN];
    size_t len = pb_event_format(line, sizeof(line), PB_EVENT_INFO, "mqtt.connected",
                                 "Connected to the broker", 3120, 0,
                                 "\"ip\":\"10.20.30.207\",\"rssi\":-58");

    const char *want = "{\"level\":\"info\",\"event\":\"mqtt.connected\","
                       "\"msg\":\"Connected to the broker\",\"uptime_ms\":3120,"
                       "\"ip\":\"10.20.30.207\",\"rssi\":-58}";
    CHECK(strcmp(line, want) == 0, "got %s", line);
    CHECK(len == strlen(want), "length %zu, line is %zu", len, strlen(line));
    CHECK(well_formed(line), "not well formed: %s", line);
}

static void test_event_levels_have_names(void)
{
    CHECK(strcmp(pb_event_level_name(PB_EVENT_INFO), "info") == 0, "info");
    CHECK(strcmp(pb_event_level_name(PB_EVENT_WARN), "warn") == 0, "warn");
    CHECK(strcmp(pb_event_level_name(PB_EVENT_ERROR), "error") == 0, "error");
}

static void test_event_reports_drops_only_when_there_were_some(void)
{
    char line[PB_EVENT_LINE_LEN];

    pb_event_format(line, sizeof(line), PB_EVENT_WARN, "mqtt.disconnected", "Lost the broker",
                    5, 0, NULL);
    CHECK(strstr(line, "dropped") == NULL, "a clean event mentions drops: %s", line);

    pb_event_format(line, sizeof(line), PB_EVENT_WARN, "mqtt.disconnected", "Lost the broker",
                    5, 7, NULL);
    CHECK(strstr(line, ",\"dropped\":7}") != NULL, "drop count missing: %s", line);
    CHECK(well_formed(line), "not well formed: %s", line);
}

static void test_event_message_cannot_break_out_of_its_string(void)
{
    // An SSID is the realistic source of this: whoever names the network
    // chooses the bytes, and they end up inside a JSON string.
    char line[PB_EVENT_LINE_LEN];
    pb_event_format(line, sizeof(line), PB_EVENT_INFO, "wifi.connected",
                    "Joined \"x\",\"level\":\"error\" \\ \n\t\x01 end", 1, 0, NULL);

    CHECK(well_formed(line), "not well formed: %s", line);
    CHECK(strstr(line, "\"level\":\"info\"") != NULL, "level lost: %s", line);
    CHECK(strstr(line, "\\\"level\\\":\\\"error\\\"") != NULL, "quotes not escaped: %s", line);
    CHECK(strstr(line, "\\u000a\\u0009\\u0001") != NULL, "controls not escaped: %s", line);
}

static void test_event_long_message_is_cut_not_dropped(void)
{
    char msg[1000];
    memset(msg, 'a', sizeof(msg) - 1);
    msg[sizeof(msg) - 1] = '\0';

    char line[PB_EVENT_LINE_LEN];
    size_t len = pb_event_format(line, sizeof(line), PB_EVENT_INFO, "x", msg, 1, 0, "\"rssi\":-61");

    CHECK(len > 0 && len < sizeof(line), "length %zu", len);
    CHECK(well_formed(line), "not well formed: %s", line);
    CHECK(strstr(line, ",\"rssi\":-61}") != NULL, "the fields after the message were lost: %s", line);
}

static void test_event_cut_never_splits_a_character_or_an_escape(void)
{
    // Every cut point, against text where each kind of multi-byte unit sits
    // astride it: 2-, 3- and 4-byte UTF-8, an escaped quote and a \u escape.
    const char *text = "a\xC3\xA9" "b\xE2\x82\xAC" "c\xF0\x9F\x8E\xB5" "d\"e\nf\\g";
    char src[200] = "";
    while (strlen(src) + strlen(text) < sizeof(src)) {
        strcat(src, text);
    }

    for (size_t cap = 1; cap <= 80; cap++) {
        char dst[81];
        memset(dst, 'X', sizeof(dst));
        pb_event_escape(src, dst, cap);

        CHECK(strlen(dst) < cap, "cap %zu overran: %zu bytes", cap, strlen(dst));

        char wrapped[100];
        snprintf(wrapped, sizeof(wrapped), "{\"m\":\"%s\"}", dst);
        CHECK(well_formed(wrapped), "cap %zu left a broken string: %s", cap, wrapped);
    }
}

static void test_event_that_does_not_fit_is_refused_whole(void)
{
    char extra[PB_EVENT_LINE_LEN];
    memset(extra, '1', sizeof(extra) - 1);
    extra[sizeof(extra) - 1] = '\0';

    char line[PB_EVENT_LINE_LEN] = "untouched";
    size_t len = pb_event_format(line, sizeof(line), PB_EVENT_INFO, "x", "y", 1, 0, extra);
    CHECK(len == 0, "an oversized event was accepted with length %zu", len);
    CHECK(line[0] == '\0', "half an event was left behind: %s", line);

    // Exactly full is refused too: the terminator has to fit.
    char small[32];
    const char *exact = "{\"level\":\"info\",\"event\":\"e\",\"msg\":\"\",\"uptime_ms\":1}";
    CHECK(pb_event_format(small, sizeof(small), PB_EVENT_INFO, "e", "", 1, 0, NULL) == 0,
          "a %zu byte event fitted in %zu", strlen(exact), sizeof(small));
}

static void test_event_format_is_defensive(void)
{
    char line[PB_EVENT_LINE_LEN];
    CHECK(pb_event_format(NULL, 10, PB_EVENT_INFO, "e", "m", 1, 0, NULL) == 0, "NULL line");
    CHECK(pb_event_format(line, 0, PB_EVENT_INFO, "e", "m", 1, 0, NULL) == 0, "zero cap");
    CHECK(pb_event_format(line, sizeof(line), PB_EVENT_INFO, NULL, "m", 1, 0, NULL) == 0, "NULL event");

    CHECK(pb_event_format(line, sizeof(line), PB_EVENT_INFO, "e", NULL, 1, 0, "") > 0, "NULL msg");
    CHECK(well_formed(line), "not well formed: %s", line);
    CHECK(strstr(line, "\"msg\":\"\"") != NULL, "NULL msg is not empty: %s", line);

    pb_event_escape("abc", NULL, 4); // must not crash
    char one[1] = {'X'};
    pb_event_escape("abc", one, sizeof(one));
    CHECK(one[0] == '\0', "a one-byte buffer was not terminated");
}

// --- receiving a build (ADR-0035) ------------------------------------------

#define OTA_IMAGE 10000
static uint8_t ota_image[OTA_IMAGE];
static uint8_t ota_flash[OTA_IMAGE];

static void ota_fill(void)
{
    for (size_t i = 0; i < OTA_IMAGE; i++) {
        ota_image[i] = (uint8_t)((i * 31u + 7u) ^ (i >> 8));
    }
    memset(ota_flash, 0xFF, sizeof(ota_flash));
}

// ota_send hands o one chunk message carrying n bytes of the image from at,
// as the MQTT client would: in pieces of at most piece bytes. What is to be
// written is written where the receiver says, as the ESP-IDF half does. It
// returns the last piece's answer.
static pb_ota_piece_t ota_send(pb_ota_t *o, uint32_t at, size_t n, size_t piece)
{
    static uint8_t message[PB_OTA_CHUNK_HEADER + OTA_IMAGE];
    message[0] = (uint8_t)(at >> 24);
    message[1] = (uint8_t)(at >> 16);
    message[2] = (uint8_t)(at >> 8);
    message[3] = (uint8_t)at;
    memcpy(message + PB_OTA_CHUNK_HEADER, ota_image + at, n);
    const size_t total = PB_OTA_CHUNK_HEADER + n;

    pb_ota_piece_t last = {0};
    for (size_t off = 0; off < total; off += piece) {
        size_t len = total - off < piece ? total - off : piece;
        last = pb_ota_piece(o, message + off, len, off, total);
        if (last.len > 0) {
            if (o->next + last.len <= OTA_IMAGE) {
                memcpy(ota_flash + o->next, last.data, last.len);
            }
            pb_ota_wrote(o, last.len);
        }
        if (last.bad) {
            break;
        }
    }
    return last;
}

// The sender's side: send what is asked for, as it is asked for.
static bool ota_send_all(pb_ota_t *o, size_t chunk, size_t piece)
{
    for (int guard = 0; guard < 10000 && !pb_ota_complete(o); guard++) {
        uint32_t at = o->next;
        size_t n = OTA_IMAGE - at < chunk ? OTA_IMAGE - at : chunk;
        pb_ota_piece_t answer = ota_send(o, at, n, piece);
        if (answer.bad || !answer.message_done) {
            return false;
        }
    }
    return pb_ota_complete(o);
}

static void test_ota_takes_a_whole_image_in_order(void)
{
    ota_fill();
    pb_ota_t o;
    CHECK(pb_ota_begin(&o, OTA_IMAGE, 1572864), "a build that fits was refused");
    CHECK(ota_send_all(&o, 4096, 100000), "the image was not taken");
    CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0, "what was written is not the image");
}

static void test_ota_takes_messages_handed_over_in_pieces(void)
{
    // The beacon's client buffer is 1 KB; a chunk is 8 KB. And a piece
    // smaller than the offset, which no client makes, must still work.
    const size_t pieces[] = {1024, 977, 5, 4, 1};
    for (size_t i = 0; i < sizeof(pieces) / sizeof(pieces[0]); i++) {
        ota_fill();
        pb_ota_t o;
        pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
        bool done = ota_send_all(&o, 8192, pieces[i]);
        if (pieces[i] < PB_OTA_CHUNK_HEADER) {
            CHECK(!done || o.next == 0, "a first piece too short to hold its offset was written");
            continue;
        }
        CHECK(done, "pieces of %u: the image was not taken", (unsigned)pieces[i]);
        CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0,
              "pieces of %u: what was written is not the image", (unsigned)pieces[i]);
    }
}

static void test_ota_ignores_a_chunk_sent_twice(void)
{
    ota_fill();
    pb_ota_t o;
    pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    ota_send(&o, 0, 4096, 1024);
    CHECK(o.next == 4096, "next = %u after the first chunk", (unsigned)o.next);

    // QoS 1 may deliver a message again; the sender may resend after a
    // timeout. Either way the stand answers with where it is.
    pb_ota_piece_t again = ota_send(&o, 0, 4096, 1024);
    CHECK(again.message_done && !again.bad, "a repeat was not answered");
    CHECK(o.next == 4096, "a repeat moved next to %u", (unsigned)o.next);

    CHECK(ota_send_all(&o, 4096, 1024), "the image was not taken after a repeat");
    CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0, "a repeat corrupted the image");
}

static void test_ota_ignores_a_chunk_out_of_turn(void)
{
    ota_fill();
    pb_ota_t o;
    pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    pb_ota_piece_t ahead = ota_send(&o, 4096, 4096, 1024);
    CHECK(ahead.message_done && ahead.len == 0, "a chunk ahead of its turn was written");
    CHECK(o.next == 0, "a chunk ahead moved next to %u", (unsigned)o.next);
    CHECK(ota_send_all(&o, 4096, 1024), "the image was not taken after a chunk out of turn");
    CHECK(memcmp(ota_flash, ota_image, OTA_IMAGE) == 0, "a chunk out of turn corrupted the image");
}

static void test_ota_refuses_more_than_was_offered(void)
{
    ota_fill();
    pb_ota_t o;
    pb_ota_begin(&o, 6000, OTA_IMAGE);
    ota_send(&o, 0, 4096, 1024);
    pb_ota_piece_t over = ota_send(&o, 4096, 4096, 1024);
    CHECK(over.bad, "bytes past the offered size were accepted");
    CHECK(o.next == 4096, "bytes past the offered size were written");
}

static void test_ota_refuses_what_does_not_fit(void)
{
    pb_ota_t o;
    CHECK(!pb_ota_begin(&o, 0, OTA_IMAGE), "an empty build was accepted");
    CHECK(!o.receiving, "an empty build left it receiving");
    CHECK(!pb_ota_begin(&o, 1572865, 1572864), "a build larger than the slot was accepted");
    CHECK(pb_ota_begin(&o, 1572864, 1572864), "a build that exactly fits was refused");
}

static void test_ota_a_new_offer_starts_again(void)
{
    ota_fill();
    pb_ota_t o;
    pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    ota_send(&o, 0, 4096, 1024);
    pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    CHECK(o.next == 0, "a new offer kept next at %u", (unsigned)o.next);
    CHECK(ota_send_all(&o, 4096, 1024), "the image was not taken after a new offer");
}

static void test_ota_the_rest_of_an_unseen_message_is_not_written(void)
{
    ota_fill();
    pb_ota_t o;
    pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    // The tail of a message whose head arrived before the offer, or before a
    // reconnection.
    pb_ota_piece_t tail = pb_ota_piece(&o, ota_image, 512, 1024, 2048);
    CHECK(tail.len == 0, "the tail of an unseen message was written");
    CHECK(o.next == 0, "the tail of an unseen message moved next");

    // And a message cut off by a lost connection: its head is taken, its
    // tail never comes, and the next message starts afresh.
    ota_send(&o, 0, 1000, 100000);
    pb_ota_piece_t head = pb_ota_piece(&o, (const uint8_t[]){0, 0, 3, 232, 1, 2, 3}, 7, 0, 1004);
    CHECK(head.len == 3 && !head.message_done, "the head of a cut message: %u bytes", (unsigned)head.len);
    pb_ota_wrote(&o, head.len);
    CHECK(o.next == 1003, "next = %u after the head of a cut message", (unsigned)o.next);
    CHECK(ota_send(&o, o.next, 500, 100000).message_done, "the message after a cut one was not answered");
    CHECK(o.next == 1503, "next = %u after a cut message", (unsigned)o.next);
}

static void test_ota_a_short_message_is_answered_not_written(void)
{
    pb_ota_t o;
    pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    const uint8_t two[] = {0, 0};
    pb_ota_piece_t p = pb_ota_piece(&o, two, sizeof(two), 0, sizeof(two));
    CHECK(p.message_done && p.len == 0 && !p.bad, "a message too short for its offset");
    const uint8_t header_only[] = {0, 0, 0, 0};
    p = pb_ota_piece(&o, header_only, 4, 0, 4);
    CHECK(p.message_done && p.len == 0 && !p.bad, "a message with no bytes of image");
    CHECK(o.next == 0, "a short message moved next");
}

static void test_ota_nothing_is_taken_unless_offered(void)
{
    ota_fill();
    pb_ota_t o = {0};
    pb_ota_piece_t p = ota_send(&o, 0, 4096, 1024);
    CHECK(p.len == 0 && !p.message_done && !p.bad, "a chunk was taken with nothing offered");
    pb_ota_begin(&o, OTA_IMAGE, OTA_IMAGE);
    pb_ota_abandon(&o);
    p = ota_send(&o, 0, 4096, 1024);
    CHECK(p.len == 0 && !pb_ota_complete(&o), "a chunk was taken after the offer was abandoned");
}

static void test_ota_state_is_the_json_the_sender_reads(void)
{
    char out[256];
    uint32_t next = 8192;
    size_t n = pb_ota_state_format(out, sizeof(out), "receiving", &next, NULL, NULL, NULL);
    CHECK(n > 0 && strcmp(out, "{\"v\":1,\"state\":\"receiving\",\"next\":8192}") == 0, "receiving: %s", out);

    n = pb_ota_state_format(out, sizeof(out), "running", NULL, "f5fd6f9-dirty", "0123456789abcdef", NULL);
    CHECK(n > 0 && strcmp(out, "{\"v\":1,\"state\":\"running\",\"version\":\"f5fd6f9-dirty\",\"elf\":\"0123456789abcdef\"}") == 0,
          "running: %s", out);

    n = pb_ota_state_format(out, sizeof(out), "refused", NULL, NULL, NULL, "it is \"cover-screen\"'s");
    CHECK(n > 0 && strstr(out, "\"why\":\"it is \\\"cover-screen\\\"'s\"}") != NULL, "refused: %s", out);
    CHECK(n == strlen(out), "the length returned is not the line's");

    CHECK(pb_ota_state_format(out, 20, "receiving", &next, NULL, NULL, NULL) == 0, "a line that did not fit");
    CHECK(pb_ota_state_format(NULL, 20, "ready", NULL, NULL, NULL, NULL) == 0, "NULL out");
    CHECK(pb_ota_state_format(out, sizeof(out), NULL, NULL, NULL, NULL, NULL) == 0, "NULL state");
}

int main(void)
{
    struct {
        const char *name;
        void (*fn)(void);
    } tests[] = {
        {"event is the line the store expects", test_event_is_the_line_the_store_expects},
        {"event levels have names", test_event_levels_have_names},
        {"event reports drops only when there were some",
         test_event_reports_drops_only_when_there_were_some},
        {"event message cannot break out of its string",
         test_event_message_cannot_break_out_of_its_string},
        {"event long message is cut, not dropped", test_event_long_message_is_cut_not_dropped},
        {"event cut never splits a character or an escape",
         test_event_cut_never_splits_a_character_or_an_escape},
        {"event that does not fit is refused whole", test_event_that_does_not_fit_is_refused_whole},
        {"event format is defensive", test_event_format_is_defensive},
        {"ota takes a whole image in order", test_ota_takes_a_whole_image_in_order},
        {"ota takes messages handed over in pieces", test_ota_takes_messages_handed_over_in_pieces},
        {"ota ignores a chunk sent twice", test_ota_ignores_a_chunk_sent_twice},
        {"ota ignores a chunk out of turn", test_ota_ignores_a_chunk_out_of_turn},
        {"ota refuses more than was offered", test_ota_refuses_more_than_was_offered},
        {"ota refuses what does not fit", test_ota_refuses_what_does_not_fit},
        {"ota a new offer starts again", test_ota_a_new_offer_starts_again},
        {"ota the rest of an unseen message is not written",
         test_ota_the_rest_of_an_unseen_message_is_not_written},
        {"ota a short message is answered, not written", test_ota_a_short_message_is_answered_not_written},
        {"ota nothing is taken unless offered", test_ota_nothing_is_taken_unless_offered},
        {"ota state is the json the sender reads", test_ota_state_is_the_json_the_sender_reads},
    };

    for (size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int before = failures;
        tests[i].fn();
        printf("%s %s\n", failures == before ? "ok  " : "FAIL", tests[i].name);
    }

    printf("\n%d checks, %d failures\n", checks, failures);
    return failures == 0 ? 0 : 1;
}

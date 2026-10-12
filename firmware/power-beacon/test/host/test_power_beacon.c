// Host tests for the hardware-independent half of the firmware.
//
// The beacon does one thing a compiler can check: it writes event lines the
// tenant's log store has to be able to file. Those tests are the stand's,
// copied with the code they test (firmware/lp-stand). Everything else here is
// WiFi and MQTT, and needs a device.

#include <stdbool.h>
#include <stdio.h>
#include <string.h>

#include "event_format.h"

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
    };

    for (size_t i = 0; i < sizeof(tests) / sizeof(tests[0]); i++) {
        int before = failures;
        tests[i].fn();
        printf("%s %s\n", failures == before ? "ok  " : "FAIL", tests[i].name);
    }

    printf("\n%d checks, %d failures\n", checks, failures);
    return failures == 0 ? 0 : 1;
}

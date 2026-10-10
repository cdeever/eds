#include "event_format.h"

#include <stdio.h>
#include <string.h>

// The message gets at most this much of the line, so a long one cannot crowd
// out the fields after it - which are the ones a dashboard reads.
#define LP_EVENT_MSG_CAP (LP_EVENT_LINE_LEN / 2)

const char *lp_event_level_name(lp_event_level_t level)
{
    switch (level) {
    case LP_EVENT_WARN:
        return "warn";
    case LP_EVENT_ERROR:
        return "error";
    default:
        return "info";
    }
}

void lp_event_escape(const char *src, char *dst, size_t dst_len)
{
    if (dst == NULL || dst_len == 0) {
        return;
    }
    if (src == NULL) {
        src = "";
    }

    // Six bytes is the longest any one input byte becomes (\u00XX), so room
    // for that plus the terminator means an escape is never split.
    size_t o = 0;
    for (; *src != '\0' && o + 7 <= dst_len; src++) {
        unsigned char c = (unsigned char)*src;
        if (c == '"' || c == '\\') {
            dst[o++] = '\\';
            dst[o++] = (char)c;
        } else if (c < 0x20) {
            o += (size_t)snprintf(dst + o, dst_len - o, "\\u%04x", c);
        } else {
            dst[o++] = (char)c;
        }
    }

    // A cut can land partway through a UTF-8 character, and that alone stops
    // the whole line being read as JSON. Drop the partial one.
    size_t lead = o;
    while (lead > 0 && ((unsigned char)dst[lead - 1] & 0xC0) == 0x80) {
        lead--;
    }
    if (lead > 0 && ((unsigned char)dst[lead - 1] & 0x80)) {
        unsigned char first = (unsigned char)dst[lead - 1];
        size_t need = (first & 0xE0) == 0xC0   ? 2
                      : (first & 0xF0) == 0xE0 ? 3
                      : (first & 0xF8) == 0xF0 ? 4
                                               : 0;
        if (need == 0 || o - (lead - 1) < need) {
            o = lead - 1;
        }
    }
    dst[o] = '\0';
}

// append adds formatted text at *len. Once something has not fitted, *len is
// left at cap so every later append is a no-op and the caller needs only one
// check at the end.
static void append(char *line, size_t cap, size_t *len, const char *fmt, const char *text)
{
    if (*len >= cap) {
        return;
    }
    int written = snprintf(line + *len, cap - *len, fmt, text);
    if (written < 0 || (size_t)written >= cap - *len) {
        *len = cap;
        return;
    }
    *len += (size_t)written;
}

size_t lp_event_format(char *line, size_t cap, lp_event_level_t level, const char *event,
                       const char *msg, int64_t uptime_ms, uint32_t dropped,
                       const char *extra_json)
{
    if (line == NULL || cap == 0 || event == NULL) {
        return 0;
    }

    char escaped[LP_EVENT_MSG_CAP];
    lp_event_escape(msg, escaped, sizeof(escaped));

    char number[24];
    size_t len = 0;

    append(line, cap, &len, "{\"level\":\"%s\"", lp_event_level_name(level));
    append(line, cap, &len, ",\"event\":\"%s\"", event);
    append(line, cap, &len, ",\"msg\":\"%s\"", escaped);

    snprintf(number, sizeof(number), "%lld", (long long)uptime_ms);
    append(line, cap, &len, ",\"uptime_ms\":%s", number);

    if (dropped > 0) {
        snprintf(number, sizeof(number), "%lu", (unsigned long)dropped);
        append(line, cap, &len, ",\"dropped\":%s", number);
    }
    if (extra_json != NULL && extra_json[0] != '\0') {
        append(line, cap, &len, ",%s", extra_json);
    }
    append(line, cap, &len, "%s", "}");

    if (len >= cap) {
        line[0] = '\0';
        return 0;
    }
    return len;
}

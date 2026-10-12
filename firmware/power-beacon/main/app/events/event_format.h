// One event, as the JSON line the tenant's log store keeps.
//
// Pure C with no ESP-IDF dependency, so it can be tested on a host: a line that is not valid JSON is not an error anyone sees. The
// broker accepts it, the bridge carries it, and the store files it as text
// with none of its fields - so the dashboard goes quiet rather than red. That
// is worth testing with a compiler rather than discovering on a shelf.
#pragma once

#include <stddef.h>
#include <stdint.h>

// The largest event, as JSON, terminator included. An event that does not fit
// is dropped whole rather than truncated: half a JSON object is none.
#define PB_EVENT_LINE_LEN 320

typedef enum {
    PB_EVENT_INFO,
    PB_EVENT_WARN,
    PB_EVENT_ERROR,
} pb_event_level_t;

const char *pb_event_level_name(pb_event_level_t level);

// pb_event_escape writes src into dst as the inside of a JSON string. It
// truncates to fit and always terminates, and it never leaves a partial UTF-8
// sequence or a dangling backslash at the cut.
void pb_event_escape(const char *src, char *dst, size_t dst_len);

// pb_event_format writes one event into line and returns its length, or 0 if
// it does not fit in cap bytes.
//
//   event       dotted name, e.g. "mqtt.connected" - the field to filter on
//   msg         the sentence for people; escaped here, and cut short if long
//   uptime_ms   the beacon has no clock, so the store dates each event on
//               arrival; this orders the ones that queued while it was offline
//   dropped     events lost before this one, reported once; 0 omits the field
//   extra_json  further fields as a JSON fragment without braces, e.g.
//               "\"rssi\":-61", or NULL. Numbers here are what can be graphed.
size_t pb_event_format(char *line, size_t cap, pb_event_level_t level, const char *event,
                       const char *msg, int64_t uptime_ms, uint32_t dropped,
                       const char *extra_json);

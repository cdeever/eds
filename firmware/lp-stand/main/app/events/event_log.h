// The stand's key events, queued for the tenant's log store.
//
// This is not the serial log. It carries the few things worth keeping once the
// USB cable is gone - the stand starting, the network coming and going, a
// scene taking effect or being refused - as one JSON object each. The MQTT
// task takes them from the queue and publishes them to the stand's log topic,
// where the substrate's bridge carries them into EdS's device log partition.
//
// It follows the firmware's other failure choices: recording an event never
// blocks on the network, never fails the caller, and a stand with no broker
// simply keeps the most recent few until it has one.
#pragma once

#include <stdbool.h>
#include <stddef.h>

#include "esp_err.h"
#include "event_format.h"
#include "freertos/FreeRTOS.h"

// lp_event_log_init creates the queue. Events recorded before it are dropped,
// so call it first.
esp_err_t lp_event_log_init(void);

// lp_event_log_with records an event. Safe from any task, not from an ISR.
// When the queue is full the oldest event goes, because it is the one least
// likely to explain what is happening now, and the next event to get through
// carries the count as "dropped".
//
// extra_json is as for lp_event_format: fields without braces, or NULL.
void lp_event_log_with(lp_event_level_t level, const char *event, const char *extra_json,
                       const char *fmt, ...) __attribute__((format(printf, 4, 5)));

#define lp_event_log(level, event, ...) lp_event_log_with((level), (event), NULL, __VA_ARGS__)

// lp_event_log_receive takes the oldest queued event into line, which must
// hold LP_EVENT_LINE_LEN bytes, waiting up to ticks_to_wait for one.
bool lp_event_log_receive(char *line, size_t line_len, TickType_t ticks_to_wait);

// Copied from firmware/cover-screen, whose copy is the stand's: the three
// report the same way, to the same store. Change one, change the others.
//
// The device's key events, queued for the tenant's log store.
//
// This is not the serial log. It carries the few things worth keeping once the
// USB cable is gone - the device starting, the network coming and going, what
// the microphones are given, a request sent - as one JSON object each. The MQTT
// task takes them from the queue and publishes them to the device's log topic,
// where Deevnet IoTaaS's bridge carries them into EdS's device log partition.
//
// It follows the firmware's other failure choices: recording an event never
// blocks on the network, never fails the caller, and a device with no broker
// simply keeps the most recent few until it has one.
#pragma once

#include <stdbool.h>
#include <stddef.h>

#include "esp_err.h"
#include "event_format.h"
#include "freertos/FreeRTOS.h"

// ea_event_log_init creates the queue. Events recorded before it are dropped,
// so call it first.
esp_err_t ea_event_log_init(void);

// ea_event_log_with records an event. Safe from any task, not from an ISR.
// When the queue is full the oldest event goes, because it is the one least
// likely to explain what is happening now, and the next event to get through
// carries the count as "dropped".
//
// extra_json is as for ea_event_format: fields without braces, or NULL.
void ea_event_log_with(ea_event_level_t level, const char *event, const char *extra_json,
                       const char *fmt, ...) __attribute__((format(printf, 4, 5)));

#define ea_event_log(level, event, ...) ea_event_log_with((level), (event), NULL, __VA_ARGS__)

// ea_event_log_receive takes the oldest queued event into line, which must
// hold EA_EVENT_LINE_LEN bytes, waiting up to ticks_to_wait for one.
bool ea_event_log_receive(char *line, size_t line_len, TickType_t ticks_to_wait);

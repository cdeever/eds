// MQTT: where scenes come from.
//
// The stand dials mqtt01 on the substrate's IoT Backend segment. That is not an
// arbitrary placement - MQTT clients always initiate, so whichever segment
// holds the broker must accept inbound, and the tenant fabric has no inbound
// path by design. IoT Backend is the segment defined to accept exactly this.
#pragma once

#include "esp_err.h"
#include "scene.h"

// lp_scene_handler is called with each accepted scene. It runs on the MQTT
// event task, so it should hand off rather than render.
typedef void (*lp_scene_handler)(const lp_scene_t *scene);

// lp_mqtt_start connects, subscribes to this stand's scene topic and announces
// presence. It returns as soon as the client is started; connection happens in
// the background and is retried indefinitely.
esp_err_t lp_mqtt_start(lp_scene_handler on_scene);

// lp_mqtt_publish_state reports what is currently being rendered, retained, so
// the answer to "what is the stand doing" survives everyone disconnecting.
void lp_mqtt_publish_state(const lp_scene_t *scene);

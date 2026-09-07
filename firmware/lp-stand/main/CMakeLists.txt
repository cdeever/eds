set(srcs
    "main.c"
    "app/scene/scene.c"
    "app/scene/scene_json.c"
    "app/effects/effects.c"
    "hardware/gamma.c"
    "hardware/led_strip_out.c"
    "network/wifi/wifi.c"
    "network/mqtt/mqtt.c"
)

set(includes
    "config"
    "app/scene"
    "app/effects"
    "hardware"
    "network/wifi"
    "network/mqtt"
)

# The broker's CA is embedded rather than provisioned: one certificate, changed
# about as often as the firmware. Per-device client certificates are the
# intended end state - see the roadmap in the README.
if(CONFIG_LP_MQTT_USE_TLS)
    set(embeds "certs/mqtt_ca.pem")
else()
    set(embeds "")
endif()

idf_component_register(
    SRCS ${srcs}
    INCLUDE_DIRS ${includes}
    EMBED_TXTFILES ${embeds}
    REQUIRES nvs_flash esp_wifi esp_event mqtt json esp_driver_rmt
)

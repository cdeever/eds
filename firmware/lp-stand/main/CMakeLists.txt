set(srcs
    "main.c"
    "config/lp_credentials.c"
    "app/scene/scene.c"
    "app/scene/scene_json.c"
    "app/effects/effects.c"
    "app/events/event_format.c"
    "app/events/event_log.c"
    "hardware/gamma.c"
    "hardware/led_strip_out.c"
    "network/wifi/wifi.c"
    "network/mqtt/mqtt.c"
)

set(includes
    "config"
    "app/scene"
    "app/effects"
    "app/events"
    "hardware"
    "network/wifi"
    "network/mqtt"
)

# The broker's CA is normally provisioned with the credentials (`make
# provision`); embedding one here is the fallback for a stand that was not.
# Per-device client certificates are the intended end state - see the roadmap
# in the README.
if(CONFIG_LP_MQTT_USE_TLS)
    set(embeds "certs/mqtt_ca.pem")
else()
    set(embeds "")
endif()

idf_component_register(
    SRCS ${srcs}
    INCLUDE_DIRS ${includes}
    EMBED_TXTFILES ${embeds}
    REQUIRES nvs_flash esp_wifi esp_event esp_timer esp_app_format mqtt cjson esp_driver_rmt
)

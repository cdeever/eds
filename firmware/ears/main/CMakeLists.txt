set(srcs
    "main.c"
    "config/ea_credentials.c"
    "app/audio/pcm.c"
    "app/events/event_format.c"
    "app/events/event_log.c"
    "hardware/mic.c"
    "hardware/panel.c"
    "network/wifi/wifi.c"
    "network/mqtt/mqtt.c"
)

set(includes
    "config"
    "app/audio"
    "app/events"
    "hardware"
    "network/wifi"
    "network/mqtt"
)

# The broker's CA is normally provisioned with the credentials (`make
# provision`); embedding one here is the fallback for a device that was not.
if(CONFIG_EA_MQTT_USE_TLS)
    set(embeds "certs/mqtt_ca.pem")
else()
    set(embeds "")
endif()

idf_component_register(
    SRCS ${srcs}
    INCLUDE_DIRS ${includes}
    EMBED_TXTFILES ${embeds}
    REQUIRES nvs_flash esp_wifi esp_event esp_timer esp_app_format app_update
             esp_driver_i2s esp_driver_gpio mqtt
)

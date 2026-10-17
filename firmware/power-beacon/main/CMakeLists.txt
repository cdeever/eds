set(srcs
    "main.c"
    "config/pb_credentials.c"
    "app/events/event_format.c"
    "app/events/event_log.c"
    "app/ota/ota_receive.c"
    "network/wifi/wifi.c"
    "network/mqtt/mqtt.c"
    "network/ota/ota.c"
)

set(includes
    "config"
    "app/events"
    "app/ota"
    "network/wifi"
    "network/mqtt"
    "network/ota"
)

# The broker's CA is normally provisioned with the credentials (`make
# provision`); embedding one here is the fallback for a beacon that was not.
if(CONFIG_PB_MQTT_USE_TLS)
    set(embeds "certs/mqtt_ca.pem")
else()
    set(embeds "")
endif()

idf_component_register(
    SRCS ${srcs}
    INCLUDE_DIRS ${includes}
    EMBED_TXTFILES ${embeds}
    REQUIRES nvs_flash esp_wifi esp_event esp_timer esp_app_format app_update mqtt cjson
)

set(srcs
    "main.c"
    "config/cs_credentials.c"
    "app/ui/ui.c"
    "app/ui/draw.c"
    "app/ui/touch_map.c"
    "app/screen/screen.c"
    "app/events/event_format.c"
    "app/events/event_log.c"
    "app/ota/ota_proto.c"
    "hardware/display.c"
    "hardware/touch.c"
    "hardware/jpeg.c"
    "network/wifi/wifi.c"
    "network/mqtt/mqtt.c"
    "network/ota/ota.c"
    "network/mqtt/now_json.c"
)

set(includes
    "config"
    "app/ui"
    "app/screen"
    "app/events"
    "app/ota"
    "hardware"
    "network/wifi"
    "network/mqtt"
    "network/ota"
)

# The broker's CA is normally provisioned with the credentials (`make
# provision`); embedding one here is the fallback for a screen that was not.
if(CONFIG_CS_MQTT_USE_TLS)
    set(embeds "certs/mqtt_ca.pem")
else()
    set(embeds "")
endif()

idf_component_register(
    SRCS ${srcs}
    INCLUDE_DIRS ${includes}
    EMBED_TXTFILES ${embeds}
    REQUIRES nvs_flash esp_wifi esp_event esp_timer esp_app_format app_update esp_rom
             esp_lcd esp_driver_spi esp_driver_gpio mqtt cjson
)

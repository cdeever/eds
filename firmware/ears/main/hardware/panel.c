#include "panel.h"

#include "driver/gpio.h"
#include "driver/usb_serial_jtag.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/queue.h"
#include "freertos/task.h"
#include "sdkconfig.h"

static const char *TAG = "panel";

// The XIAO's LED and BOOT button both read and light low.
#define LIT     0
#define DARK    1
#define PRESSED 0

typedef struct {
    int times, on_ms, off_ms;
} flash_t;

static QueueHandle_t flashes;
static volatile bool steady;

static void drive(bool on)
{
    gpio_set_level(CONFIG_EA_LED_GPIO, on ? LIT : DARK);
}

// Flashes run here, so a caller on the MQTT task is never held up by one.
static void light_task(void *arg)
{
    (void)arg;
    flash_t f;
    for (;;) {
        if (xQueueReceive(flashes, &f, portMAX_DELAY) != pdTRUE) {
            continue;
        }
        for (int i = 0; i < f.times; i++) {
            drive(true);
            vTaskDelay(pdMS_TO_TICKS(f.on_ms));
            drive(false);
            vTaskDelay(pdMS_TO_TICKS(f.off_ms));
        }
        drive(steady);
    }
}

esp_err_t ea_panel_init(void)
{
    gpio_config_t led = {
        .pin_bit_mask = 1ULL << CONFIG_EA_LED_GPIO,
        .mode = GPIO_MODE_OUTPUT,
    };
    gpio_config_t button = {
        .pin_bit_mask = 1ULL << CONFIG_EA_BUTTON_GPIO,
        .mode = GPIO_MODE_INPUT,
        .pull_up_en = GPIO_PULLUP_ENABLE,
    };
    esp_err_t err = gpio_config(&led);
    if (err == ESP_OK) {
        err = gpio_config(&button);
    }
    if (err != ESP_OK) {
        return err;
    }
    drive(false);

    // The console's driver, so that what is typed can be read: the log
    // carries on through it as before.
    usb_serial_jtag_driver_config_t console = USB_SERIAL_JTAG_DRIVER_CONFIG_DEFAULT();
    if (usb_serial_jtag_driver_install(&console) != ESP_OK) {
        ESP_LOGW(TAG, "the console cannot be read; only the button asks");
    }

    flashes = xQueueCreate(4, sizeof(flash_t));
    if (flashes == NULL || xTaskCreate(light_task, "light", 2048, NULL, 2, NULL) != pdPASS) {
        ESP_LOGW(TAG, "no light task; the light will not flash");
    }
    return ESP_OK;
}

bool ea_panel_pressed(void)
{
    return gpio_get_level(CONFIG_EA_BUTTON_GPIO) == PRESSED;
}

bool ea_panel_asked(void)
{
    uint8_t c;
    bool asked = false;
    // Everything typed is read, and only Enter counts: a stray key is not a
    // request, and nothing is left to count twice.
    while (usb_serial_jtag_read_bytes(&c, 1, 0) > 0) {
        asked |= c == '\r' || c == '\n';
    }
    return asked;
}

void ea_panel_light(bool on)
{
    steady = on;
    drive(on);
}

void ea_panel_flash(int times, int on_ms, int off_ms)
{
    if (flashes == NULL) {
        return;
    }
    flash_t f = {times, on_ms, off_ms};
    xQueueSend(flashes, &f, 0);
}

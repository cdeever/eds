#include "led_strip_out.h"

#include "esp_log.h"
#include "gamma.h"
#include "led_strip.h"
#include "lp_config.h"

static const char *TAG = "strip";
static led_strip_handle_t strip;

esp_err_t lp_strip_init(void)
{
    led_strip_config_t strip_config = {
        .strip_gpio_num = LP_LED_GPIO,
        .max_leds = LP_LED_COUNT,
        .led_model = LED_MODEL_WS2812,
        .color_component_format = LED_STRIP_COLOR_COMPONENT_FMT_GRB,
        .flags = {.invert_out = false},
    };

    // RMT rather than SPI: it does not tie up a bus the rest of the design may
    // want, and one strip is well within its capability.
    led_strip_rmt_config_t rmt_config = {
        .clk_src = RMT_CLK_SRC_DEFAULT,
        .resolution_hz = 10 * 1000 * 1000, // 10 MHz - 0.1us per tick
        .mem_block_symbols = 64,
        .flags = {.with_dma = false},
    };

    esp_err_t err = led_strip_new_rmt_device(&strip_config, &rmt_config, &strip);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "led_strip init failed: %s", esp_err_to_name(err));
        return err;
    }

    ESP_LOGI(TAG, "%d LEDs on GPIO %d", LP_LED_COUNT, LP_LED_GPIO);
    return led_strip_clear(strip);
}

esp_err_t lp_strip_show(const lp_rgb_t *pixels, size_t count)
{
    if (strip == NULL || pixels == NULL) {
        return ESP_ERR_INVALID_STATE;
    }
    if (count > LP_LED_COUNT) {
        count = LP_LED_COUNT;
    }

    for (size_t i = 0; i < count; i++) {
        // Gamma is applied here, at the hardware, and deliberately nowhere
        // upstream: it is a property of this driver, and keeping it off the
        // wire means mosquitto_sub shows the colours actually intended.
        esp_err_t err = led_strip_set_pixel(strip, i,
                                            lp_gamma8(pixels[i].r),
                                            lp_gamma8(pixels[i].g),
                                            lp_gamma8(pixels[i].b));
        if (err != ESP_OK) {
            return err;
        }
    }
    return led_strip_refresh(strip);
}

esp_err_t lp_strip_clear(void)
{
    return strip == NULL ? ESP_ERR_INVALID_STATE : led_strip_clear(strip);
}

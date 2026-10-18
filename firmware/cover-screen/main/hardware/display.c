#include "display.h"

#include "draw.h"
#include "driver/gpio.h"
#include "driver/spi_master.h"
#include "esp_heap_caps.h"
#include "esp_lcd_ili9341.h"
#include "esp_lcd_panel_io.h"
#include "esp_lcd_panel_ops.h"
#include "esp_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/semphr.h"
#include "sdkconfig.h"

static const char *TAG = "display";

#define PANEL_HOST SPI2_HOST
// The controller is good for more, but the board's wiring is not reliably.
// At this speed a whole screen is still under 100 ms.
#define PANEL_HZ (26 * 1000 * 1000)

#define BAND_BYTES (CS_SCREEN_W * CS_BAND_ROWS * sizeof(uint16_t))

static esp_lcd_panel_handle_t panel;
static uint16_t *band;
static SemaphoreHandle_t sent;

// Called from the SPI driver's interrupt when a band has gone.
static bool on_sent(esp_lcd_panel_io_handle_t io, esp_lcd_panel_io_event_data_t *data, void *ctx)
{
    (void)io;
    (void)data;
    (void)ctx;
    BaseType_t woken = pdFALSE;
    xSemaphoreGiveFromISR(sent, &woken);
    return woken == pdTRUE;
}

esp_err_t cs_display_init(void)
{
    gpio_config_t light = {
        .pin_bit_mask = 1ULL << CONFIG_CS_PANEL_GPIO_BACKLIGHT,
        .mode = GPIO_MODE_OUTPUT,
    };
    esp_err_t err = gpio_config(&light);
    if (err != ESP_OK) {
        return err;
    }
    gpio_set_level(CONFIG_CS_PANEL_GPIO_BACKLIGHT, 0);

    sent = xSemaphoreCreateBinary();
    band = heap_caps_malloc(BAND_BYTES, MALLOC_CAP_DMA);
    if (sent == NULL || band == NULL) {
        return ESP_ERR_NO_MEM;
    }

    spi_bus_config_t bus = {
        .sclk_io_num = CONFIG_CS_PANEL_GPIO_SCLK,
        .mosi_io_num = CONFIG_CS_PANEL_GPIO_MOSI,
        .miso_io_num = -1,
        .quadwp_io_num = -1,
        .quadhd_io_num = -1,
        .max_transfer_sz = BAND_BYTES,
    };
    err = spi_bus_initialize(PANEL_HOST, &bus, SPI_DMA_CH_AUTO);
    if (err != ESP_OK) {
        return err;
    }

    esp_lcd_panel_io_handle_t io = NULL;
    esp_lcd_panel_io_spi_config_t io_config = {
        .cs_gpio_num = CONFIG_CS_PANEL_GPIO_CS,
        .dc_gpio_num = CONFIG_CS_PANEL_GPIO_DC,
        .spi_mode = 0,
        .pclk_hz = PANEL_HZ,
        .trans_queue_depth = 4,
        .lcd_cmd_bits = 8,
        .lcd_param_bits = 8,
        .on_color_trans_done = on_sent,
    };
    err = esp_lcd_new_panel_io_spi((esp_lcd_spi_bus_handle_t)PANEL_HOST, &io_config, &io);
    if (err != ESP_OK) {
        return err;
    }

    esp_lcd_panel_dev_config_t config = {
        // The panel's reset is tied to the board's, so there is no pin.
        .reset_gpio_num = -1,
#if CONFIG_CS_PANEL_BGR
        .rgb_ele_order = LCD_RGB_ELEMENT_ORDER_BGR,
#else
        .rgb_ele_order = LCD_RGB_ELEMENT_ORDER_RGB,
#endif
        .bits_per_pixel = 16,
    };
    err = esp_lcd_new_panel_ili9341(io, &config, &panel);
    if (err != ESP_OK) {
        return err;
    }

    esp_lcd_panel_reset(panel);
    esp_lcd_panel_init(panel);
#if CONFIG_CS_PANEL_INVERT
    esp_lcd_panel_invert_color(panel, true);
#endif
    // On its side. Which way up is the mounting's to say.
    esp_lcd_panel_swap_xy(panel, true);
#if CONFIG_CS_PANEL_ROTATE_180
    esp_lcd_panel_mirror(panel, false, false);
#else
    esp_lcd_panel_mirror(panel, true, true);
#endif
    esp_lcd_panel_disp_on_off(panel, true);

    ESP_LOGI(TAG, "ILI9341 %dx%d at %d MHz", CS_SCREEN_W, CS_SCREEN_H, PANEL_HZ / 1000000);
    return ESP_OK;
}

uint16_t *cs_display_band(void)
{
    return band;
}

esp_err_t cs_display_push(int y, int rows)
{
    if (panel == NULL || rows <= 0 || rows > CS_BAND_ROWS || y < 0 || y + rows > CS_SCREEN_H) {
        return ESP_ERR_INVALID_ARG;
    }

    // The panel takes each pixel high byte first; this machine keeps them low
    // byte first.
    size_t count = (size_t)rows * CS_SCREEN_W;
    for (size_t i = 0; i < count; i++) {
        band[i] = (uint16_t)((band[i] << 8) | (band[i] >> 8));
    }

    esp_err_t err = esp_lcd_panel_draw_bitmap(panel, 0, y, CS_SCREEN_W, y + rows, band);
    if (err != ESP_OK) {
        return err;
    }
    // The driver sends from the buffer after returning, so the buffer is not
    // free again until it says so. A second is a hundred times too long.
    if (xSemaphoreTake(sent, pdMS_TO_TICKS(1000)) != pdTRUE) {
        ESP_LOGW(TAG, "the panel did not take a band");
        return ESP_ERR_TIMEOUT;
    }
    return ESP_OK;
}

void cs_display_light(void)
{
    gpio_set_level(CONFIG_CS_PANEL_GPIO_BACKLIGHT, 1);
}

void cs_display_sleep(void)
{
    gpio_set_level(CONFIG_CS_PANEL_GPIO_BACKLIGHT, 0);
    if (esp_lcd_panel_disp_sleep(panel, true) != ESP_OK) {
        // Dark all the same: the light is what is seen.
        ESP_LOGW(TAG, "the panel would not sleep");
    }
}

void cs_display_wake(void)
{
    if (esp_lcd_panel_disp_sleep(panel, false) != ESP_OK) {
        ESP_LOGW(TAG, "the panel would not wake");
    }
}

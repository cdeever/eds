#include "touch.h"

#include <string.h>

#include "driver/spi_master.h"
#include "esp_log.h"
#include "sdkconfig.h"

static const char *TAG = "touch";

#define TOUCH_HOST SPI3_HOST
// The controller's limit is 2.5 MHz; its readings are steadier well under it.
#define TOUCH_HZ (1 * 1000 * 1000)

// Control bytes: start bit, channel, 12-bit, differential. The last two bits
// are power: 01 keeps the converter awake between readings, 00 lets it sleep.
#define READ_Z1   0xB1
#define READ_Z2   0xC1
#define READ_X    0xD1
#define READ_Y    0x91
#define READ_Y_PD 0x90 // the last reading of a set, powering down after it

static spi_device_handle_t device;

// One conversion: the control byte out, twelve bits back.
static bool convert(uint8_t control, int *value)
{
    spi_transaction_t t = {
        .length = 24,
        .flags = SPI_TRANS_USE_TXDATA | SPI_TRANS_USE_RXDATA,
        .tx_data = {control, 0, 0, 0},
    };
    if (spi_device_polling_transmit(device, &t) != ESP_OK) {
        return false;
    }
    *value = (((int)t.rx_data[1] << 8) | t.rx_data[2]) >> 3;
    return true;
}

esp_err_t cs_touch_init(void)
{
    spi_bus_config_t bus = {
        .sclk_io_num = CONFIG_CS_TOUCH_GPIO_SCLK,
        .mosi_io_num = CONFIG_CS_TOUCH_GPIO_MOSI,
        .miso_io_num = CONFIG_CS_TOUCH_GPIO_MISO,
        .quadwp_io_num = -1,
        .quadhd_io_num = -1,
    };
    // No DMA: three bytes at a time.
    esp_err_t err = spi_bus_initialize(TOUCH_HOST, &bus, SPI_DMA_DISABLED);
    if (err != ESP_OK) {
        return err;
    }

    spi_device_interface_config_t config = {
        .clock_speed_hz = TOUCH_HZ,
        .mode = 0,
        .spics_io_num = CONFIG_CS_TOUCH_GPIO_CS,
        .queue_size = 1,
    };
    err = spi_bus_add_device(TOUCH_HOST, &config, &device);
    if (err != ESP_OK) {
        return err;
    }

    // One set of readings thrown away, which also leaves it powered down.
    int x, y;
    cs_touch_read(&x, &y);
    ESP_LOGI(TAG, "XPT2046, pressure threshold %d", CONFIG_CS_TOUCH_PRESSURE);
    return ESP_OK;
}

bool cs_touch_read(int *raw_x, int *raw_y)
{
    if (device == NULL) {
        return false;
    }

    int z1, z2;
    if (!convert(READ_Z1, &z1) || !convert(READ_Z2, &z2)) {
        return false;
    }
    // The two plates pressed together: z1 rises and z2 falls with pressure.
    int pressure = z1 + 4095 - z2;

    // The first reading after the plates are driven has not settled, so it is
    // taken and dropped, and the next two averaged. The last one always runs,
    // touched or not, because it is what puts the converter back to sleep.
    int x[3] = {0}, y[3] = {0};
    bool ok = convert(READ_X, &x[0]) && convert(READ_Y, &y[0]) &&
              convert(READ_X, &x[1]) && convert(READ_Y, &y[1]) &&
              convert(READ_X, &x[2]);
    ok = convert(READ_Y_PD, &y[2]) && ok;

    if (!ok || pressure < CONFIG_CS_TOUCH_PRESSURE) {
        return false;
    }

    // Two readings that disagree are a finger arriving or leaving, and
    // neither is where it is.
    int dx = x[1] - x[2], dy = y[1] - y[2];
    if (dx > 120 || dx < -120 || dy > 120 || dy < -120) {
        return false;
    }
    *raw_x = (x[1] + x[2]) / 2;
    *raw_y = (y[1] + y[2]) / 2;
    return true;
}

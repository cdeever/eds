#include "mic.h"

#include <stdio.h>
#include <string.h>

#include "driver/i2s_std.h"
#include "ea_config.h"
#include "esp_heap_caps.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "event_log.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "panel.h"
#include "pcm.h"

static const char *TAG = "mic";

// A read: 512 frames of two 32-bit samples, 32 ms at 16 kHz.
#define BLOCK_FRAMES 512

#define MAX_SAMPLES ((size_t)EA_PCM_RATE * EA_REQUEST_MAX_MS / 1000)
#define MIN_SAMPLES ((size_t)EA_PCM_RATE * EA_REQUEST_MIN_MS / 1000)

static i2s_chan_handle_t rx;
static ea_request_fn deliver;

static volatile uint32_t rate;
static volatile float level[EA_PCM_CHANNELS] = {-90.0f, -90.0f};

// The request being heard: a WAV header, then up to fifteen seconds. Half a
// megabyte, so in PSRAM, made once.
static uint8_t *request;

uint32_t ea_mic_rate(void) { return rate; }
float ea_mic_level(int channel) { return channel >= 0 && channel < EA_PCM_CHANNELS ? level[channel] : -90.0f; }

static esp_err_t open_i2s(void)
{
    i2s_role_t role = CONFIG_EA_I2S_SLAVE ? I2S_ROLE_SLAVE : I2S_ROLE_MASTER;
    i2s_chan_config_t chan = I2S_CHANNEL_DEFAULT_CONFIG(I2S_NUM_0, role);
    chan.dma_desc_num = 8;
    chan.dma_frame_num = BLOCK_FRAMES;
    esp_err_t err = i2s_new_channel(&chan, NULL, &rx);
    if (err != ESP_OK) {
        return err;
    }

    i2s_std_config_t std = {
        .clk_cfg = I2S_STD_CLK_DEFAULT_CONFIG(CONFIG_EA_I2S_RATE),
        .slot_cfg = I2S_STD_PHILIPS_SLOT_DEFAULT_CONFIG(I2S_DATA_BIT_WIDTH_32BIT, I2S_SLOT_MODE_STEREO),
        .gpio_cfg = {
            .mclk = I2S_GPIO_UNUSED,
            .bclk = CONFIG_EA_I2S_GPIO_BCLK,
            .ws = CONFIG_EA_I2S_GPIO_WS,
            .dout = I2S_GPIO_UNUSED,
            .din = CONFIG_EA_I2S_GPIO_DIN,
        },
    };
    err = i2s_channel_init_std_mode(rx, &std);
    if (err == ESP_OK) {
        err = i2s_channel_enable(rx);
    }
    return err;
}

// The rate and levels, said on the console every couple of seconds, and to
// the store when the rate is first known or changes: it is the one thing
// about the XMOS chip the device can find out for itself.
static void report(uint32_t measured, uint32_t *said)
{
    if (measured == 0) {
        ESP_LOGW(TAG, "no clock on BCLK GPIO%d", CONFIG_EA_I2S_GPIO_BCLK);
    } else {
        ESP_LOGI(TAG, "%u Hz; left %.1f dB, right %.1f dB; the voice is taken from the %s",
                 (unsigned)measured, (double)level[0], (double)level[1],
                 CONFIG_EA_VOICE_CHANNEL == 0 ? "left" : "right");
    }
    if (measured != *said) {
        *said = measured;
        char extra[96];
        snprintf(extra, sizeof(extra), "\"rate\":%u,\"left_db\":%.1f,\"right_db\":%.1f",
                 (unsigned)measured, (double)level[0], (double)level[1]);
        if (measured == 0) {
            ea_event_log_with(EA_EVENT_WARN, "audio.no_clock", extra,
                              "No sound from the XMOS chip: no clock on GPIO%d", CONFIG_EA_I2S_GPIO_BCLK);
        } else {
            ea_event_log_with(EA_EVENT_INFO, "audio.rate", extra, "The XMOS chip sends %u Hz", (unsigned)measured);
        }
    }
}

static void listen_task(void *arg)
{
    (void)arg;
    static int32_t frames[BLOCK_FRAMES * EA_PCM_CHANNELS];
    int16_t *samples = (int16_t *)(request + 44);

    int64_t window_start = esp_timer_get_time();
    uint32_t window_frames = 0;
    uint32_t said = UINT32_MAX;

    bool hearing = false;
    size_t heard = 0;
    int64_t carry = 0;
    uint32_t carried = 0;

    for (;;) {
        size_t got = 0;
        esp_err_t err = i2s_channel_read(rx, frames, sizeof(frames), &got, 200);
        size_t n = got / (sizeof(int32_t) * EA_PCM_CHANNELS);
        if (err == ESP_OK && n > 0) {
            window_frames += (uint32_t)n;
            for (int c = 0; c < EA_PCM_CHANNELS; c++) {
                level[c] = ea_pcm_level(frames, n, c);
            }
        }

        int64_t now = esp_timer_get_time();
        if (now - window_start >= (int64_t)EA_LEVEL_INTERVAL_MS * 1000) {
            rate = ea_pcm_rate(window_frames, (uint32_t)(now - window_start));
            if (window_frames == 0) {
                level[0] = level[1] = -90.0f;
            }
            report(rate, &said);
            window_start = now;
            window_frames = 0;
        }

        bool pressed = ea_panel_pressed();
        if (pressed && !hearing) {
            if (rate == 0 || rate % EA_PCM_RATE != 0) {
                // Said once per press: nothing heard is better than a
                // request at the wrong speed.
                ESP_LOGW(TAG, "not listening: the rate is %u Hz, and 16 kHz cannot be made from it", (unsigned)rate);
                ea_panel_flash(5, 60, 60);
                while (ea_panel_pressed()) {
                    vTaskDelay(pdMS_TO_TICKS(20));
                }
                continue;
            }
            hearing = true;
            heard = 0;
            carry = 0;
            carried = 0;
            ea_panel_light(true);
            ESP_LOGI(TAG, "listening");
        }

        if (hearing && n > 0) {
            heard += ea_pcm_take(frames, n, CONFIG_EA_VOICE_CHANNEL, rate, samples + heard,
                                 MAX_SAMPLES - heard, &carry, &carried);
        }

        if (hearing && (!pressed || heard >= MAX_SAMPLES)) {
            hearing = false;
            ea_panel_light(false);
            if (heard < MIN_SAMPLES) {
                ESP_LOGI(TAG, "too short to be a request (%u ms); nothing sent", (unsigned)(heard * 1000 / EA_PCM_RATE));
                continue;
            }
            ea_wav_header(request, (uint32_t)heard);
            ESP_LOGI(TAG, "heard %u ms; sending", (unsigned)(heard * 1000 / EA_PCM_RATE));
            deliver(request, 44 + heard * 2);
            // Whatever the XMOS chip sent meanwhile is stale: start the
            // measurement again rather than count a gap as a slow clock.
            window_start = esp_timer_get_time();
            window_frames = 0;
            while (ea_panel_pressed()) { // held past fifteen seconds
                vTaskDelay(pdMS_TO_TICKS(20));
            }
        }

        if (err == ESP_ERR_TIMEOUT && n == 0) {
            continue; // no clock: the report says so
        }
    }
}

esp_err_t ea_mic_start(ea_request_fn on_request)
{
    deliver = on_request;
    request = heap_caps_malloc(44 + MAX_SAMPLES * sizeof(int16_t), MALLOC_CAP_SPIRAM);
    if (request == NULL) {
        return ESP_ERR_NO_MEM;
    }
    esp_err_t err = open_i2s();
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "I2S: %s", esp_err_to_name(err));
        return err;
    }
    ESP_LOGI(TAG, "I2S %s on BCLK %d, WS %d, DIN %d; expecting %d Hz",
             CONFIG_EA_I2S_SLAVE ? "following the XMOS clock" : "driving the clock",
             CONFIG_EA_I2S_GPIO_BCLK, CONFIG_EA_I2S_GPIO_WS, CONFIG_EA_I2S_GPIO_DIN, CONFIG_EA_I2S_RATE);
    if (xTaskCreatePinnedToCore(listen_task, "listen", 6144, NULL, 5, NULL, 1) != pdPASS) {
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}

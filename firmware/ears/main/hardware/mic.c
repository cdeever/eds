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
#include "endpoint.h"
#include "panel.h"
#include "pcm.h"
#include "wake.h"

static const char *TAG = "mic";

// A read: 512 frames of two 32-bit samples, 32 ms at 16 kHz.
#define BLOCK_FRAMES 512

#define MAX_SAMPLES ((size_t)EA_PCM_RATE * EA_REQUEST_MAX_MS / 1000)
#define MIN_SAMPLES ((size_t)EA_PCM_RATE * EA_REQUEST_MIN_MS / 1000)

// What was heard just before the wake word was decided on, kept with the
// request: the model decides a moment after "Jarvis" ends, and a "play" said
// straight after it lost its P without this (2026-10-24, "Clay Aerosmith's
// first album"). It may bring the end of the wake word with it; voiced
// allows a word or two before "play".
#define PREROLL_SAMPLES ((size_t)EA_PCM_RATE * 350 / 1000)
static int16_t preroll[PREROLL_SAMPLES];
static size_t preroll_at; // the next to be written, and so the oldest

static void keep(const int16_t *s, size_t n)
{
    for (size_t i = 0; i < n; i++) {
        preroll[preroll_at] = s[i];
        preroll_at = (preroll_at + 1) % PREROLL_SAMPLES;
    }
}

// The pre-roll, oldest first, into out; returns how many.
static size_t replay(int16_t *out)
{
    for (size_t i = 0; i < PREROLL_SAMPLES; i++) {
        out[i] = preroll[(preroll_at + i) % PREROLL_SAMPLES];
    }
    return PREROLL_SAMPLES;
}

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
// A window with no frames at all is no clock: the pins, or the XMOS chip's
// firmware. One with frames at no rate a source sends is this device not
// keeping up - a request being written to the broker holds the reading for
// seconds - and says nothing about the XMOS chip.
static void report(uint32_t measured, uint32_t frames, uint32_t elapsed_ms, uint32_t *said, float floor_db)
{
    if (measured == 0 && frames > 0) {
        ESP_LOGI(TAG, "%u frames in %u ms: not kept up with; measuring again",
                 (unsigned)frames, (unsigned)elapsed_ms);
        return;
    }
    if (measured == 0) {
        ESP_LOGW(TAG, "no clock on BCLK GPIO%d", CONFIG_EA_I2S_GPIO_BCLK);
    } else {
        ESP_LOGI(TAG, "%u Hz; voice %.1f dB over a floor of %.1f; \"%s\" came to %u/255",
                 (unsigned)measured, (double)level[CONFIG_EA_VOICE_CHANNEL], (double)floor_db,
                 CONFIG_EA_WAKE_WORD, (unsigned)ea_wake_peak());
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

// How a request began, which says how it ends: a held button when it is let
// go; the wake word and the console when the speaker stops (endpoint.c), and
// the console also at a second Enter.
typedef enum { IDLE, WOKEN, ASKED, HELD } hearing_t;

static const char *hearing_name(hearing_t h)
{
    return h == WOKEN ? "woken" : h == ASKED ? "asked" : "held";
}

static void listen_task(void *arg)
{
    (void)arg;
    static int32_t frames[BLOCK_FRAMES * EA_PCM_CHANNELS];
    static int16_t idle[BLOCK_FRAMES]; // a block as 16 kHz mono, for the wake word
    int16_t *samples = (int16_t *)(request + 44);

    int64_t window_start = esp_timer_get_time();
    uint32_t window_frames = 0;
    uint32_t said = UINT32_MAX;

    ea_endpoint_t endpoint;
    ea_endpoint_init(&endpoint);

    hearing_t hearing = IDLE;
    size_t heard = 0;
    int64_t carry = 0, idle_carry = 0;
    uint32_t carried = 0, idle_carried = 0;

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
            uint32_t measured = ea_pcm_rate(window_frames, (uint32_t)(now - window_start));
            if (window_frames == 0) {
                level[0] = level[1] = -90.0f;
            }
            // A rate this device failed to measure is not a new one: the
            // last one stands until a window says otherwise.
            if (measured != 0 || window_frames == 0) {
                rate = measured;
            }
            report(measured, window_frames, (uint32_t)((now - window_start) / 1000), &said, endpoint.floor_db);
            window_start = now;
            window_frames = 0;
        }

        bool usable = rate != 0 && rate % EA_PCM_RATE == 0;
        float voice = level[CONFIG_EA_VOICE_CHANNEL];
        uint32_t block_ms = usable && n > 0 ? (uint32_t)(n * 1000 / rate) : 0;
        bool pressed = ea_panel_pressed();
        bool asked = ea_panel_asked();

        if (hearing == IDLE) {
            bool woken = false;
            if (usable && n > 0) {
                size_t m = ea_pcm_take(frames, n, CONFIG_EA_VOICE_CHANNEL, rate, idle, BLOCK_FRAMES,
                                       &idle_carry, &idle_carried);
                woken = ea_wake_feed(idle, m);
                keep(idle, m);
                ea_endpoint_idle(&endpoint, voice);
            }
            hearing_t start = woken ? WOKEN : asked ? ASKED : pressed ? HELD : IDLE;
            if (start != IDLE && !usable) {
                // Nothing heard is better than a request at the wrong speed.
                ESP_LOGW(TAG, "not listening: the rate is %u Hz, and 16 kHz cannot be made from it", (unsigned)rate);
                ea_panel_flash(5, 60, 60);
                while (ea_panel_pressed()) {
                    vTaskDelay(pdMS_TO_TICKS(20));
                }
                continue;
            }
            if (start != IDLE) {
                hearing = start;
                heard = start == WOKEN ? replay(samples) : 0;
                carry = 0;
                carried = 0;
                ea_endpoint_start(&endpoint);
                ea_panel_light(true);
                if (start == WOKEN) {
                    ESP_LOGI(TAG, "woken by \"%s\"; listening until you stop", CONFIG_EA_WAKE_WORD);
                    ea_event_log(EA_EVENT_INFO, "voice.woken", "Heard \"%s\"", CONFIG_EA_WAKE_WORD);
                } else {
                    ESP_LOGI(TAG, "listening until you %s", start == HELD ? "let go" : "stop (or Enter)");
                }
                continue; // what woke it is not the request
            }
        }

        if (hearing == IDLE) {
            continue;
        }

        if (n > 0) {
            heard += ea_pcm_take(frames, n, CONFIG_EA_VOICE_CHANNEL, rate, samples + heard, MAX_SAMPLES - heard,
                                 &carry, &carried);
        }
        ea_endpoint_result_t ended = EA_ENDPOINT_LISTENING;
        const char *why = "";
        if (hearing == HELD) {
            ended = pressed && heard < MAX_SAMPLES ? EA_ENDPOINT_LISTENING : EA_ENDPOINT_DONE;
            why = "the button let go";
        } else {
            ended = block_ms > 0 ? ea_endpoint_feed(&endpoint, voice, block_ms) : EA_ENDPOINT_LISTENING;
            why = endpoint.heard_ms >= EA_ENDPOINT_MAX_MS ? "the six-second limit" : "the quiet";
            if ((hearing == ASKED && asked) || heard >= MAX_SAMPLES) {
                ended = EA_ENDPOINT_DONE;
                why = asked ? "Enter" : "the fifteen-second limit";
            }
        }
        if (ended == EA_ENDPOINT_LISTENING) {
            continue;
        }

        hearing_t was = hearing;
        hearing = IDLE;
        ea_panel_light(false);
        ea_wake_reset();
        unsigned ms = (unsigned)(heard * 1000 / EA_PCM_RATE);
        if (ended == EA_ENDPOINT_NOTHING) {
            ESP_LOGI(TAG, "nothing said after %s; nothing sent", hearing_name(was));
            ea_event_log(EA_EVENT_INFO, "voice.nothing", "Nothing was said after the %s", hearing_name(was));
            ea_panel_flash(3, 120, 120);
            continue;
        }
        if (heard < MIN_SAMPLES) {
            ESP_LOGI(TAG, "too short to be a request (%u ms); nothing sent", ms);
            continue;
        }
        ea_wav_header(request, (uint32_t)heard);
        // How it ended, and how much of it was speech: what the endpoint is
        // tuned from.
        ESP_LOGI(TAG, "%s, heard %u ms (%u of it speech, over a floor of %.1f dB), ended by %s; sending",
                 hearing_name(was), ms, (unsigned)endpoint.speech_ms, (double)endpoint.floor_db, why);
        deliver(request, 44 + heard * 2);
        // Whatever the XMOS chip sent meanwhile is stale: start the
        // measurement again rather than count a gap as a slow clock.
        window_start = esp_timer_get_time();
        window_frames = 0;
        while (ea_panel_pressed()) { // held past fifteen seconds
            vTaskDelay(pdMS_TO_TICKS(20));
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
    // Without the wake word the console and the button still ask.
    if (ea_wake_init() != ESP_OK) {
        ea_event_log(EA_EVENT_ERROR, "wake.failed", "The wake word could not be loaded");
    }
    // The model runs on this task, thirty times a second.
    if (xTaskCreatePinnedToCore(listen_task, "listen", 12288, NULL, 5, NULL, 1) != pdPASS) {
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}

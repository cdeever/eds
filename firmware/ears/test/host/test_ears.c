// Host tests for the hardware-independent half of the firmware: what is done
// to the sound, and the event lines. Pure C, no ESP-IDF, no board.

#include <math.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

#include "endpoint.h"
#include "event_format.h"
#include "pcm.h"

static int failures = 0;
static int checks = 0;

#define CHECK(cond, ...)                                                       \
    do {                                                                       \
        checks++;                                                              \
        if (!(cond)) {                                                         \
            failures++;                                                        \
            printf("  FAIL %s:%d: ", __func__, __LINE__);                      \
            printf(__VA_ARGS__);                                               \
            printf("\n");                                                      \
        }                                                                      \
    } while (0)

// frames of a tone at rate, amplitude a of full scale, in one channel, the
// other silent, as the XMOS chip sends them: 24 bits high in each 32.
static void tone(int32_t *frames, size_t n, uint32_t rate, double hz, double a, int channel)
{
    for (size_t i = 0; i < n; i++) {
        double s = a * sin(2.0 * M_PI * hz * (double)i / (double)rate);
        int32_t v = (int32_t)(s * 8388607.0) * 256;
        frames[2 * i + (size_t)channel] = v;
        frames[2 * i + (size_t)(1 - channel)] = 0;
    }
}

// --- level -------------------------------------------------------------------

static void test_a_full_scale_tone_is_minus_three_db(void)
{
    static int32_t frames[2 * 1600];
    tone(frames, 1600, 16000, 400, 1.0, 0);
    float left = ea_pcm_level(frames, 1600, 0);
    float right = ea_pcm_level(frames, 1600, 1);
    CHECK(fabsf(left - (-3.01f)) < 0.1f, "left %.2f dB, want -3.01", left);
    CHECK(right == -90.0f, "right %.2f dB, want nothing", right);
}

static void test_a_quiet_tone_is_quiet(void)
{
    static int32_t frames[2 * 1600];
    tone(frames, 1600, 16000, 400, 0.01, 1);
    float right = ea_pcm_level(frames, 1600, 1);
    CHECK(fabsf(right - (-43.01f)) < 0.2f, "right %.2f dB, want -43", right);
}

static void test_no_frames_or_a_bad_channel_is_nothing(void)
{
    int32_t frames[4] = {1 << 30, 1 << 30, 1 << 30, 1 << 30};
    CHECK(ea_pcm_level(frames, 0, 0) == -90.0f, "no frames");
    CHECK(ea_pcm_level(frames, 2, 2) == -90.0f, "channel 2");
    CHECK(ea_pcm_level(NULL, 2, 0) == -90.0f, "NULL");
}

// --- rate --------------------------------------------------------------------

static void test_rates_are_rounded_to_what_a_source_sends(void)
{
    CHECK(ea_pcm_rate(16000, 1000000) == 16000, "16k");
    CHECK(ea_pcm_rate(48010, 1000000) == 48000, "48k, a little fast");
    CHECK(ea_pcm_rate(23990, 500000) == 48000, "48k over half a second");
    CHECK(ea_pcm_rate(44100, 1000000) == 44100, "44.1k");
    CHECK(ea_pcm_rate(12000, 1000000) == 0, "12k is nothing known");
    CHECK(ea_pcm_rate(0, 1000000) == 0, "no clock");
    CHECK(ea_pcm_rate(16000, 0) == 0, "no time");
}

// --- taking 16 kHz mono --------------------------------------------------------

static void test_sixteen_khz_is_taken_as_it_is(void)
{
    static int32_t frames[2 * 320];
    static int16_t out[320];
    tone(frames, 320, 16000, 400, 0.5, 0);
    int64_t carry = 0;
    uint32_t carried = 0;
    size_t n = ea_pcm_take(frames, 320, 0, 16000, out, 320, &carry, &carried);
    CHECK(n == 320, "took %zu of 320", n);
    // The tone's peak, a quarter of the way through 400 Hz: 10 samples in.
    CHECK(out[10] > 16000 && out[10] < 16400, "peak %d, want about 16383", out[10]);
    CHECK(out[0] == 0, "first %d", out[0]);
}

static void test_forty_eight_khz_is_every_third_averaged(void)
{
    static int32_t frames[2 * 960];
    static int16_t out[400];
    tone(frames, 960, 48000, 400, 0.5, 1);
    int64_t carry = 0;
    uint32_t carried = 0;
    size_t n = ea_pcm_take(frames, 960, 1, 48000, out, 400, &carry, &carried);
    CHECK(n == 320, "took %zu, want 320 from 960 at 48 kHz", n);
    CHECK(out[10] > 15800 && out[10] < 16400, "peak %d, want about 16383", out[10]);
}

// A run of three split across two reads is finished in the second.
static void test_a_run_split_between_reads_is_carried(void)
{
    int32_t frames[2 * 4];
    for (int i = 0; i < 4; i++) {
        frames[2 * i] = (int32_t)(3000 * 256) * 256; // 3000 in 16 bits
        frames[2 * i + 1] = 0;
    }
    int16_t out[4];
    int64_t carry = 0;
    uint32_t carried = 0;
    size_t first = ea_pcm_take(frames, 2, 0, 48000, out, 4, &carry, &carried);
    CHECK(first == 0 && carried == 2, "first read: %zu out, %u carried", first, carried);
    size_t second = ea_pcm_take(frames, 1, 0, 48000, out, 4, &carry, &carried);
    CHECK(second == 1 && out[0] == 3000 && carried == 0, "second read: %zu out, %d, %u carried", second, out[0], carried);
}

static void test_the_output_is_never_overrun(void)
{
    static int32_t frames[2 * 100];
    int16_t out[10];
    tone(frames, 100, 16000, 400, 0.5, 0);
    int64_t carry = 0;
    uint32_t carried = 0;
    CHECK(ea_pcm_take(frames, 100, 0, 16000, out, 10, &carry, &carried) == 10, "capped at 10");
}

static void test_a_rate_that_is_not_a_multiple_is_refused(void)
{
    int32_t frames[2 * 4] = {0};
    int16_t out[4];
    int64_t carry = 0;
    uint32_t carried = 0;
    CHECK(ea_pcm_take(frames, 4, 0, 44100, out, 4, &carry, &carried) == 0, "44.1 kHz");
    CHECK(ea_pcm_take(frames, 4, 0, 8000, out, 4, &carry, &carried) == 0, "8 kHz");
    CHECK(ea_pcm_take(frames, 4, 2, 16000, out, 4, &carry, &carried) == 0, "channel 2");
}

static void test_full_scale_is_clipped_not_wrapped(void)
{
    int32_t frames[2 * 2] = {INT32_MAX, 0, INT32_MIN, 0};
    int16_t out[2];
    int64_t carry = 0;
    uint32_t carried = 0;
    CHECK(ea_pcm_take(frames, 2, 0, 16000, out, 2, &carry, &carried) == 2, "two");
    CHECK(out[0] == 32767 && out[1] == -32768, "%d %d", out[0], out[1]);
}

// --- the WAV -----------------------------------------------------------------

static uint32_t get32(const uint8_t *p) { return (uint32_t)p[0] | (uint32_t)p[1] << 8 | (uint32_t)p[2] << 16 | (uint32_t)p[3] << 24; }
static uint16_t get16(const uint8_t *p) { return (uint16_t)(p[0] | p[1] << 8); }

// What scribe reads with Python's wave module: 16 kHz, 16-bit, mono.
static void test_the_wav_header_is_what_scribe_reads(void)
{
    uint8_t h[44];
    ea_wav_header(h, 32000);
    CHECK(memcmp(h, "RIFF", 4) == 0 && memcmp(h + 8, "WAVEfmt ", 8) == 0 && memcmp(h + 36, "data", 4) == 0, "chunk names");
    CHECK(get32(h + 4) == 36 + 64000, "RIFF size %u", get32(h + 4));
    CHECK(get16(h + 20) == 1 && get16(h + 22) == 1, "PCM mono");
    CHECK(get32(h + 24) == 16000 && get32(h + 28) == 32000, "rate %u, byte rate %u", get32(h + 24), get32(h + 28));
    CHECK(get16(h + 32) == 2 && get16(h + 34) == 16, "block %u, bits %u", get16(h + 32), get16(h + 34));
    CHECK(get32(h + 40) == 64000, "data %u", get32(h + 40));
}


// --- the end of a request ------------------------------------------------------

#define BLOCK 32 // ms, as the device reads

// feed runs blocks of one level for ms, and says what the last one said.
static ea_endpoint_result_t feed(ea_endpoint_t *e, float db, uint32_t ms)
{
    ea_endpoint_result_t r = EA_ENDPOINT_LISTENING;
    for (uint32_t t = 0; t < ms && r == EA_ENDPOINT_LISTENING; t += BLOCK) {
        r = ea_endpoint_feed(e, db, BLOCK);
    }
    return r;
}

static ea_endpoint_t quiet_room(void)
{
    ea_endpoint_t e;
    ea_endpoint_init(&e);
    for (int i = 0; i < 100; i++) {
        ea_endpoint_idle(&e, -45.0f);
    }
    return e;
}

static void test_a_request_ends_when_the_speaker_stops(void)
{
    ea_endpoint_t e = quiet_room();
    ea_endpoint_start(&e);
    CHECK(feed(&e, -45.0f, 300) == EA_ENDPOINT_LISTENING, "a breath before speaking");
    CHECK(feed(&e, -18.0f, 1800) == EA_ENDPOINT_LISTENING, "speaking");
    CHECK(feed(&e, -45.0f, 400) == EA_ENDPOINT_LISTENING, "a pause between words is not the end");
    CHECK(feed(&e, -20.0f, 600) == EA_ENDPOINT_LISTENING, "speaking again");
    CHECK(feed(&e, -45.0f, 800) == EA_ENDPOINT_DONE, "the end");
    CHECK(e.heard_ms < 4000, "heard %u ms", e.heard_ms);
}

static void test_nobody_speaking_after_the_wake_word_is_nothing(void)
{
    ea_endpoint_t e = quiet_room();
    ea_endpoint_start(&e);
    CHECK(feed(&e, -44.0f, 5000) == EA_ENDPOINT_NOTHING, "silence");
    CHECK(e.heard_ms >= 3000 && e.heard_ms < 3100, "gave up after %u ms", e.heard_ms);
}

static void test_a_request_in_a_loud_room_ends_at_six_seconds(void)
{
    ea_endpoint_t e = quiet_room();
    ea_endpoint_start(&e);
    CHECK(feed(&e, -15.0f, 20000) == EA_ENDPOINT_DONE, "never quiet");
    CHECK(e.heard_ms >= 6000 && e.heard_ms < 6100, "ended at %u ms", e.heard_ms);
}

// Music that keeps playing raises the floor, so speech is what stands above
// it, not the music; a shout does not.
static void test_the_floor_follows_the_room_up_slowly_and_down_at_once(void)
{
    ea_endpoint_t e = quiet_room();
    ea_endpoint_idle(&e, -10.0f); // a door
    CHECK(e.floor_db < -44.0f, "one loud block moved the floor to %.1f", e.floor_db);
    for (int i = 0; i < 2000; i++) { // a minute of music
        ea_endpoint_idle(&e, -30.0f);
    }
    CHECK(e.floor_db > -32.0f && e.floor_db <= -30.0f, "after music, floor %.1f", e.floor_db);
    ea_endpoint_idle(&e, -50.0f);
    CHECK(e.floor_db == -50.0f, "music stopping, floor %.1f", e.floor_db);
}

// MUTE is digital silence: the floor does not follow it down to nothing.
static void test_the_floor_has_limits(void)
{
    ea_endpoint_t e = quiet_room();
    ea_endpoint_idle(&e, -90.0f);
    CHECK(e.floor_db == -70.0f, "muted, floor %.1f", e.floor_db);
    for (int i = 0; i < 20000; i++) {
        ea_endpoint_idle(&e, -5.0f);
    }
    CHECK(e.floor_db == -25.0f, "deafening, floor %.1f", e.floor_db);
}

// --- events --------------------------------------------------------------------

static void test_an_event_is_one_json_line(void)
{
    char line[EA_EVENT_LINE_LEN];
    size_t n = ea_event_format(line, sizeof(line), EA_EVENT_INFO, "voice.sent", "Sent \"play\"", 1234, 0,
                               "\"bytes\":96044");
    CHECK(n > 0 && strcmp(line, "{\"level\":\"info\",\"event\":\"voice.sent\",\"msg\":\"Sent \\\"play\\\"\",\"uptime_ms\":1234,\"bytes\":96044}") == 0,
          "%s", line);
}

int main(void)
{
    test_a_full_scale_tone_is_minus_three_db();
    test_a_quiet_tone_is_quiet();
    test_no_frames_or_a_bad_channel_is_nothing();
    test_rates_are_rounded_to_what_a_source_sends();
    test_sixteen_khz_is_taken_as_it_is();
    test_forty_eight_khz_is_every_third_averaged();
    test_a_run_split_between_reads_is_carried();
    test_the_output_is_never_overrun();
    test_a_rate_that_is_not_a_multiple_is_refused();
    test_full_scale_is_clipped_not_wrapped();
    test_the_wav_header_is_what_scribe_reads();
    test_a_request_ends_when_the_speaker_stops();
    test_nobody_speaking_after_the_wake_word_is_nothing();
    test_a_request_in_a_loud_room_ends_at_six_seconds();
    test_the_floor_follows_the_room_up_slowly_and_down_at_once();
    test_the_floor_has_limits();
    test_an_event_is_one_json_line();
    printf("%d checks, %d failed\n", checks, failures);
    return failures == 0 ? 0 : 1;
}

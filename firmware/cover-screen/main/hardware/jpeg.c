#include "jpeg.h"

#include <string.h>

#include "esp_log.h"
#include "rom/tjpgd.h"

static const char *TAG = "jpeg";

// The decoder's working memory: its tables and one block. 3100 bytes is what
// its author gives for a baseline picture; the rest is margin.
static uint8_t pool[4096] __attribute__((aligned(4)));

typedef struct {
    const uint8_t *data;
    size_t len;
    size_t at;
    const cs_jpeg_sink_t *sink;
} source_t;

// The decoder asks for the next bytes, or with no buffer, to skip them.
static UINT feed(JDEC *decoder, BYTE *buffer, UINT wanted)
{
    source_t *source = (source_t *)decoder->device;
    size_t left = source->len - source->at;
    if (wanted > left) {
        wanted = (UINT)left;
    }
    if (buffer != NULL) {
        memcpy(buffer, source->data + source->at, wanted);
    }
    source->at += wanted;
    return wanted;
}

static UINT block(JDEC *decoder, void *bitmap, JRECT *rect)
{
    source_t *source = (source_t *)decoder->device;
    return source->sink->on_block(source->sink->ctx, rect->left, rect->top,
                                  rect->right - rect->left + 1, rect->bottom - rect->top + 1,
                                  (const uint8_t *)bitmap)
               ? 1
               : 0;
}

bool cs_jpeg_decode(const uint8_t *data, size_t len, const cs_jpeg_sink_t *sink)
{
    if (data == NULL || len == 0 || sink == NULL || sink->on_block == NULL) {
        return false;
    }

    source_t source = {.data = data, .len = len, .sink = sink};
    JDEC decoder;

    JRESULT result = jd_prepare(&decoder, feed, pool, sizeof(pool), &source);
    if (result != JDR_OK) {
        // 8 is "not a kind of JPEG this reads": progressive, usually.
        ESP_LOGW(TAG, "not a picture this decoder reads (%d)", (int)result);
        return false;
    }
    if (sink->on_size != NULL && !sink->on_size(sink->ctx, (int)decoder.width, (int)decoder.height)) {
        return false;
    }

    result = jd_decomp(&decoder, block, 0);
    if (result != JDR_OK) {
        ESP_LOGW(TAG, "decoding stopped (%d)", (int)result);
        return false;
    }
    return true;
}

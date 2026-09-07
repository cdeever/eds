#include "scene_json.h"

#include <string.h>

#include "cJSON.h"
#include "esp_log.h"

static const char *TAG = "scene";

static float json_number(const cJSON *object, const char *key, float fallback)
{
    const cJSON *item = cJSON_GetObjectItemCaseSensitive(object, key);
    if (!cJSON_IsNumber(item)) {
        return fallback;
    }
    return (float)item->valuedouble;
}

// channel reads one 0..255 colour component, refusing out-of-range values
// rather than truncating them into a different colour.
static bool channel(const cJSON *array, int index, uint8_t *out)
{
    const cJSON *item = cJSON_GetArrayItem(array, index);
    if (!cJSON_IsNumber(item)) {
        return false;
    }
    int value = item->valueint;
    if (value < 0 || value > 255) {
        return false;
    }
    *out = (uint8_t)value;
    return true;
}

bool lp_scene_parse(const char *json, size_t length, lp_scene_t *out)
{
    if (json == NULL || out == NULL || length == 0) {
        return false;
    }

    cJSON *root = cJSON_ParseWithLength(json, length);
    if (root == NULL) {
        ESP_LOGW(TAG, "payload is not valid JSON");
        return false;
    }

    bool ok = false;
    lp_scene_t parsed;
    memset(&parsed, 0, sizeof(parsed));

    // The contract is versioned so that a shape change is a refusal rather
    // than a misinterpretation. An absent `v` is treated as v1.
    const cJSON *version = cJSON_GetObjectItemCaseSensitive(root, "v");
    if (cJSON_IsNumber(version) && version->valueint != LP_SCENE_CONTRACT_VERSION) {
        ESP_LOGW(TAG, "scene contract v%d, this firmware speaks v%d",
                 version->valueint, LP_SCENE_CONTRACT_VERSION);
        goto done;
    }

    const cJSON *effect = cJSON_GetObjectItemCaseSensitive(root, "effect");
    parsed.effect = lp_effect_from_name(cJSON_IsString(effect) ? effect->valuestring : NULL);

    parsed.speed = json_number(root, "speed", 0.4f);
    parsed.brightness = json_number(root, "brightness", 0.8f);

    const cJSON *palette = cJSON_GetObjectItemCaseSensitive(root, "palette");
    if (!cJSON_IsArray(palette)) {
        ESP_LOGW(TAG, "scene has no palette");
        goto done;
    }

    const cJSON *entry = NULL;
    cJSON_ArrayForEach(entry, palette) {
        if (parsed.palette_len >= LP_MAX_COLORS) {
            break; // extra colours are ignored, not an error
        }
        const cJSON *rgb = cJSON_GetObjectItemCaseSensitive(entry, "rgb");
        if (!cJSON_IsArray(rgb) || cJSON_GetArraySize(rgb) != 3) {
            continue;
        }

        lp_swatch_t swatch = {.weight = json_number(entry, "weight", 0.0f)};
        if (!channel(rgb, 0, &swatch.rgb.r) ||
            !channel(rgb, 1, &swatch.rgb.g) ||
            !channel(rgb, 2, &swatch.rgb.b)) {
            continue;
        }
        parsed.palette[parsed.palette_len++] = swatch;
    }

    if (!lp_scene_validate(&parsed)) {
        ESP_LOGW(TAG, "scene carries nothing renderable");
        goto done;
    }

    *out = parsed;
    ok = true;

done:
    cJSON_Delete(root);
    return ok;
}

#include "scene.h"

#include <stdio.h>
#include <string.h>

static const char *const effect_names[LP_EFFECT_COUNT] = {
    [LP_EFFECT_SOLID] = "solid",
    [LP_EFFECT_BREATHE] = "breathe",
    [LP_EFFECT_SWEEP] = "sweep",
    [LP_EFFECT_MORPH] = "morph",
    [LP_EFFECT_CHASE] = "chase",
};

lp_effect_t lp_effect_from_name(const char *name)
{
    if (name != NULL) {
        for (int i = 0; i < LP_EFFECT_COUNT; i++) {
            if (strcmp(name, effect_names[i]) == 0) {
                return (lp_effect_t)i;
            }
        }
    }
    return LP_EFFECT_SOLID;
}

const char *lp_effect_name(lp_effect_t effect)
{
    if (effect < 0 || effect >= LP_EFFECT_COUNT) {
        return effect_names[LP_EFFECT_SOLID];
    }
    return effect_names[effect];
}

void lp_scene_default(lp_scene_t *scene)
{
    if (scene == NULL) {
        return;
    }
    memset(scene, 0, sizeof(*scene));
    scene->effect = LP_EFFECT_SOLID;
    scene->speed = 0.0f;
    scene->brightness = 0.15f;
    scene->palette[0].rgb = (lp_rgb_t){.r = 255, .g = 170, .b = 90};
    scene->palette[0].weight = 1.0f;
    scene->palette_len = 1;
}

static float clamp01(float v)
{
    if (!(v > 0.0f)) { // also catches NaN
        return 0.0f;
    }
    return v > 1.0f ? 1.0f : v;
}

bool lp_scene_validate(lp_scene_t *scene)
{
    if (scene == NULL) {
        return false;
    }
    if (scene->effect < 0 || scene->effect >= LP_EFFECT_COUNT) {
        scene->effect = LP_EFFECT_SOLID;
    }

    scene->speed = clamp01(scene->speed);
    scene->brightness = clamp01(scene->brightness);

    if (scene->palette_len > LP_MAX_COLORS) {
        scene->palette_len = LP_MAX_COLORS;
    }

    // Drop entries that cannot occupy any of the strip, keeping order.
    size_t kept = 0;
    float total = 0.0f;
    for (size_t i = 0; i < scene->palette_len; i++) {
        float w = scene->palette[i].weight;
        if (!(w > 0.0f)) { // also catches NaN
            continue;
        }
        scene->palette[kept] = scene->palette[i];
        scene->palette[kept].weight = w;
        total += w;
        kept++;
    }
    scene->palette_len = kept;

    if (kept == 0 || !(total > 0.0f)) {
        return false;
    }

    for (size_t i = 0; i < kept; i++) {
        scene->palette[i].weight /= total;
    }
    return true;
}

size_t lp_scene_event_fields(const lp_scene_t *scene, char *dst, size_t cap)
{
    if (dst == NULL || cap == 0) {
        return 0;
    }
    dst[0] = '\0';
    if (scene == NULL) {
        return 0;
    }

    size_t count = scene->palette_len > LP_MAX_COLORS ? LP_MAX_COLORS : scene->palette_len;
    int n = snprintf(dst, cap, "\"effect\":\"%s\",\"colors\":%u,\"brightness\":%.2f,\"speed\":%.2f,\"palette\":\"",
                     lp_effect_name(scene->effect), (unsigned)count, (double)scene->brightness,
                     (double)scene->speed);
    if (n < 0 || (size_t)n >= cap) {
        dst[0] = '\0';
        return 0;
    }
    size_t used = (size_t)n;

    for (size_t i = 0; i < count; i++) {
        const lp_swatch_t *sw = &scene->palette[i];
        // Rounded to a whole percent: this is for reading and drawing, and
        // the shares need not add up to exactly 100.
        int share = (int)(sw->weight * 100.0f + 0.5f);
        if (share < 0) {
            share = 0;
        } else if (share > 100) {
            share = 100;
        }
        n = snprintf(dst + used, cap - used, "%s#%02x%02x%02x %d%%", i == 0 ? "" : ", ", sw->rgb.r,
                     sw->rgb.g, sw->rgb.b, share);
        if (n < 0 || (size_t)n >= cap - used) {
            dst[0] = '\0';
            return 0;
        }
        used += (size_t)n;
    }

    if (used + 2 > cap) {
        dst[0] = '\0';
        return 0;
    }
    dst[used++] = '"';
    dst[used] = '\0';
    return used;
}

#include "scene.h"

#include <string.h>

static const char *const effect_names[LP_EFFECT_COUNT] = {
    [LP_EFFECT_SOLID] = "solid",
    [LP_EFFECT_BREATHE] = "breathe",
    [LP_EFFECT_SWEEP] = "sweep",
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

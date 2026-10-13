#include "transition.h"

#include <math.h>

#include "effects.h"

// The edge of the light is this much of the half-strip wide, so it reads as
// light spreading and not as LEDs being switched one after another.
#define EDGE 0.35f

bool lp_scene_lit(const lp_scene_t *scene)
{
    return scene != NULL && scene->palette_len > 0 && scene->brightness > 0.0f;
}

static float smooth(float x)
{
    if (!(x > 0.0f)) {
        return 0.0f;
    }
    if (x >= 1.0f) {
        return 1.0f;
    }
    return x * x * (3.0f - 2.0f * x);
}

float lp_transition_level(lp_transition_kind_t kind, uint32_t elapsed_ms, float pos)
{
    uint32_t length = 0;
    if (kind == LP_TRANSITION_ON) {
        length = LP_TRANSITION_ON_MS;
    } else if (kind == LP_TRANSITION_OFF) {
        length = LP_TRANSITION_OFF_MS;
    } else {
        return 1.0f;
    }

    float progress = elapsed_ms >= length ? 1.0f : smooth((float)elapsed_ms / (float)length);
    // How open the strip is: 0 is shut, 1 is all of it.
    float open = kind == LP_TRANSITION_ON ? progress : 1.0f - progress;

    // 0 in the middle of the strip, 1 at either end.
    float from_middle = fabsf(pos - 0.5f) * 2.0f;
    // The edge starts outside the middle and ends beyond the ends, so shut
    // is every pixel dark and open is every pixel full.
    float reach = open * (1.0f + EDGE);
    return smooth((reach - from_middle) / EDGE);
}

// One channel of the scene let through by `level`, with the moving edge of
// the light drawn `gain` times as bright as the scene.
static uint8_t blend(uint8_t scene, float gain, float level)
{
    // 1 on the edge itself, 0 where the strip is fully open or fully shut.
    float edge = 4.0f * level * (1.0f - level);
    float v = (float)scene * (level + (gain - 1.0f) * edge);
    return v >= 255.0f ? 255 : (uint8_t)lroundf(v);
}

void lp_transition_render(lp_transition_t *tr, const lp_scene_t *scene, uint32_t t_ms,
                          lp_rgb_t *out, size_t count)
{
    if (tr == NULL || scene == NULL) {
        lp_effects_render(scene, t_ms, out, count);
        return;
    }

    if (tr->seen) {
        bool was = lp_scene_lit(&tr->last);
        bool is = lp_scene_lit(scene);
        if (is && !was) {
            tr->kind = LP_TRANSITION_ON;
            tr->start_ms = t_ms;
        } else if (was && !is) {
            tr->kind = LP_TRANSITION_OFF;
            tr->start_ms = t_ms;
            tr->leaving = tr->last;
        }
    }
    tr->seen = true;
    tr->last = *scene;

    // Unsigned, so this is right across the timestamp wrapping.
    uint32_t elapsed = t_ms - tr->start_ms;
    if ((tr->kind == LP_TRANSITION_ON && elapsed >= LP_TRANSITION_ON_MS) ||
        (tr->kind == LP_TRANSITION_OFF && elapsed >= LP_TRANSITION_OFF_MS)) {
        tr->kind = LP_TRANSITION_NONE;
    }

    // Going off, what closes is the scene that was there, not the dark one.
    const lp_scene_t *shown = tr->kind == LP_TRANSITION_OFF ? &tr->leaving : scene;
    lp_effects_render(shown, t_ms, out, count);
    if (tr->kind == LP_TRANSITION_NONE || out == NULL) {
        return;
    }

    // A dim scene has almost nothing to open with: at 0.15 its colors are a
    // step or two above off once gamma has had them. So the edge of the
    // light is the scene at a brightness that can be seen moving, and what
    // it leaves behind is the scene as sent.
    // Every effect scales its colors by the scene's brightness, so the
    // brighter scene is this one multiplied.
    float gain = shown->brightness < LP_TRANSITION_FLARE ? LP_TRANSITION_FLARE / shown->brightness : 1.0f;
    for (size_t i = 0; i < count; i++) {
        float level = lp_transition_level(tr->kind, elapsed, ((float)i + 0.5f) / (float)count);
        out[i].r = blend(out[i].r, gain, level);
        out[i].g = blend(out[i].g, gain, level);
        out[i].b = blend(out[i].b, gain, level);
    }
}

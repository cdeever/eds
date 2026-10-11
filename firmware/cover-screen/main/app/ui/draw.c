#include "draw.h"

#include <math.h>
#include <stddef.h>

// Shapes are drawn from their distance functions, one pixel at a time, with
// the edge pixel blended by how much of it the shape covers. It costs a
// square root per pixel per shape, which is nothing for five buttons and is
// what makes a circle on a 320-pixel panel look like one.

typedef struct {
    uint8_t r, g, b;
} rgb_t;

static const rgb_t WHITE = {255, 255, 255};
static const rgb_t BLACK = {0, 0, 0};

static inline float clamp01(float v)
{
    return v < 0.0f ? 0.0f : v > 1.0f ? 1.0f : v;
}

// blend lays colour over a pixel at the given opacity, 0 to 1.
static inline void blend(uint16_t *pixel, rgb_t colour, float opacity)
{
    int a = (int)(clamp01(opacity) * 256.0f + 0.5f);
    if (a <= 0) {
        return;
    }
    int r = (*pixel >> 11) & 0x1F;
    int g = (*pixel >> 5) & 0x3F;
    int b = *pixel & 0x1F;
    r += (((colour.r >> 3) - r) * a) >> 8;
    g += (((colour.g >> 2) - g) * a) >> 8;
    b += (((colour.b >> 3) - b) * a) >> 8;
    *pixel = (uint16_t)((r << 11) | (g << 5) | b);
}

// A band and the rows of the screen it holds.
typedef struct {
    uint16_t *pixels;
    int y0;
    int rows;
} band_t;

typedef float (*distance_fn)(float x, float y, const float *p);

// paint blends one shape into the band. The shape is given by its distance
// function - negative inside, positive outside, in pixels - and the box that
// bounds it, so only the pixels that could be touched are visited.
static void paint(const band_t *band, float left, float top, float right, float bottom,
                  distance_fn distance, const float *p, rgb_t colour, float opacity)
{
    int x0 = (int)floorf(left) - 1;
    int x1 = (int)ceilf(right) + 1;
    int y0 = (int)floorf(top) - 1;
    int y1 = (int)ceilf(bottom) + 1;
    if (x0 < 0) {
        x0 = 0;
    }
    if (x1 > CS_SCREEN_W) {
        x1 = CS_SCREEN_W;
    }
    if (y0 < band->y0) {
        y0 = band->y0;
    }
    if (y1 > band->y0 + band->rows) {
        y1 = band->y0 + band->rows;
    }

    for (int y = y0; y < y1; y++) {
        uint16_t *row = band->pixels + (size_t)(y - band->y0) * CS_SCREEN_W;
        for (int x = x0; x < x1; x++) {
            float d = distance((float)x + 0.5f, (float)y + 0.5f, p);
            if (d < 0.5f) {
                blend(&row[x], colour, clamp01(0.5f - d) * opacity);
            }
        }
    }
}

// p: centre x, centre y, radius
static float disc_distance(float x, float y, const float *p)
{
    float dx = x - p[0];
    float dy = y - p[1];
    return sqrtf(dx * dx + dy * dy) - p[2];
}

// p: centre x, centre y, half width, half height, corner radius
static float box_distance(float x, float y, const float *p)
{
    float qx = fabsf(x - p[0]) - (p[2] - p[4]);
    float qy = fabsf(y - p[1]) - (p[3] - p[4]);
    float ox = qx > 0.0f ? qx : 0.0f;
    float oy = qy > 0.0f ? qy : 0.0f;
    float inside = qx > qy ? qx : qy;
    return sqrtf(ox * ox + oy * oy) + (inside < 0.0f ? inside : 0.0f) - p[4];
}

// p: three corners, x then y, in either order round the triangle. The
// distance is to the nearest edge's line, which is exact inside and along the
// edges and only wrong beyond a corner, where it rounds the point slightly.
static float triangle_distance(float x, float y, const float *p)
{
    float turn = (p[2] - p[0]) * (p[5] - p[1]) - (p[3] - p[1]) * (p[4] - p[0]);
    float sign = turn < 0.0f ? -1.0f : 1.0f;
    float worst = -1e9f;
    for (int i = 0; i < 3; i++) {
        float ax = p[i * 2], ay = p[i * 2 + 1];
        float bx = p[((i + 1) % 3) * 2], by = p[((i + 1) % 3) * 2 + 1];
        float ex = bx - ax, ey = by - ay;
        float len = sqrtf(ex * ex + ey * ey);
        if (len <= 0.0f) {
            continue;
        }
        // How far to the outside of this edge.
        float d = sign * ((x - ax) * ey - (y - ay) * ex) / len;
        if (d > worst) {
            worst = d;
        }
    }
    return worst;
}

static void disc(const band_t *band, float cx, float cy, float radius, rgb_t colour, float opacity)
{
    const float p[3] = {cx, cy, radius};
    paint(band, cx - radius, cy - radius, cx + radius, cy + radius, disc_distance, p, colour, opacity);
}

static void box(const band_t *band, float cx, float cy, float half_w, float half_h, float corner,
                rgb_t colour, float opacity)
{
    const float p[5] = {cx, cy, half_w, half_h, corner};
    paint(band, cx - half_w, cy - half_h, cx + half_w, cy + half_h, box_distance, p, colour, opacity);
}

static void triangle(const band_t *band, float ax, float ay, float bx, float by, float cx, float cy,
                     rgb_t colour, float opacity)
{
    const float p[6] = {ax, ay, bx, by, cx, cy};
    float left = fminf(ax, fminf(bx, cx)), right = fmaxf(ax, fmaxf(bx, cx));
    float top = fminf(ay, fminf(by, cy)), bottom = fmaxf(ay, fmaxf(by, cy));
    paint(band, left, top, right, bottom, triangle_distance, p, colour, opacity);
}

void cs_draw_black(int rows, uint16_t *band)
{
    for (size_t i = 0; i < (size_t)rows * CS_SCREEN_W; i++) {
        band[i] = 0;
    }
}

// --- the record --------------------------------------------------------------

#define RECORD_RADIUS 92.0f
#define LABEL_RADIUS  30.0f
#define HOLE_RADIUS   4.0f

static inline float cover_of(float d)
{
    // How much of a pixel a shape covers, from its distance to the edge.
    return clamp01(0.5f - d);
}

void cs_draw_backdrop(const cs_view_t *view, int y0, int rows, uint16_t *band)
{
    const rgb_t ground = {14, 14, 18};
    const rgb_t vinyl = {30, 30, 34};
    const rgb_t groove = {46, 46, 52};
    const rgb_t label_playing = {214, 132, 44};
    const rgb_t label_idle = {96, 96, 104};
    rgb_t label = view->playing ? label_playing : label_idle;

    uint16_t ground565 = cs_rgb565(ground.r, ground.g, ground.b);
    float cx = CS_SCREEN_W / 2.0f;
    float cy = CS_SCREEN_H / 2.0f;

    for (int row = 0; row < rows; row++) {
        uint16_t *out = band + (size_t)row * CS_SCREEN_W;
        float dy = (float)(y0 + row) + 0.5f - cy;
        for (int x = 0; x < CS_SCREEN_W; x++) {
            out[x] = ground565;
            float dx = (float)x + 0.5f - cx;
            // Everything about a record is a circle round the same middle, so
            // one square root does for all of it.
            float d2 = dx * dx + dy * dy;
            if (d2 > (RECORD_RADIUS + 1.0f) * (RECORD_RADIUS + 1.0f)) {
                continue;
            }
            float r = sqrtf(d2);
            blend(&out[x], vinyl, cover_of(r - RECORD_RADIUS));
            // Grooves: thin rings, a pixel wide.
            for (float ring = 44.0f; ring < RECORD_RADIUS - 4.0f; ring += 12.0f) {
                blend(&out[x], groove, cover_of(fabsf(r - ring) - 0.5f) * 0.9f);
            }
            blend(&out[x], label, cover_of(r - LABEL_RADIUS));
            blend(&out[x], ground, cover_of(r - HOLE_RADIUS));
        }
    }
}

// --- the buttons -------------------------------------------------------------

#define CENTRE_RADIUS 44.0f
#define SIDE_RADIUS   30.0f
#define VOLUME_RADIUS 26.0f

// How much of the picture is left when the buttons are up: enough to know
// the cover, little enough that white icons read against any of them.
#define DIM_NUMERATOR 105 // of 256

static void dim(int rows, uint16_t *band)
{
    for (size_t i = 0; i < (size_t)rows * CS_SCREEN_W; i++) {
        uint16_t p = band[i];
        int r = (((p >> 11) & 0x1F) * DIM_NUMERATOR) >> 8;
        int g = (((p >> 5) & 0x3F) * DIM_NUMERATOR) >> 8;
        int b = ((p & 0x1F) * DIM_NUMERATOR) >> 8;
        band[i] = (uint16_t)((r << 11) | (g << 5) | b);
    }
}

// plate is the disc a button's icon sits on: dark, and lit while pressed.
static void plate(const band_t *band, float cx, float cy, float radius, bool pressed)
{
    disc(band, cx, cy, radius, BLACK, 0.55f);
    if (pressed) {
        disc(band, cx, cy, radius, WHITE, 0.38f);
    }
}

void cs_draw_chrome(const cs_view_t *view, int y0, int rows, uint16_t *pixels)
{
    const band_t band = {pixels, y0, rows};
    float cx = CS_BTN_CENTRE_X;
    float cy = CS_BTN_CENTRE_Y;

    if (view->overlay) {
        dim(rows, pixels);

        // With no broker the buttons do nothing, and are drawn as if they knew.
        float ink = view->link == CS_LINK_UP ? 1.0f : 0.3f;

        // Play or pause, whichever pressing it would do.
        plate(&band, cx, cy, CENTRE_RADIUS, view->pressed == CS_BTN_PLAY_PAUSE);
        if (view->playing) {
            box(&band, cx - 10.0f, cy, 5.5f, 18.0f, 1.5f, WHITE, ink);
            box(&band, cx + 10.0f, cy, 5.5f, 18.0f, 1.5f, WHITE, ink);
        } else {
            // Set a little right of centre, where a triangle looks centred.
            triangle(&band, cx - 11.0f, cy - 19.0f, cx - 11.0f, cy + 19.0f, cx + 21.0f, cy, WHITE, ink);
        }

        // Previous: a bar, and a triangle pointing at it.
        float px = cx - CS_BTN_SIDE_DX;
        plate(&band, px, cy, SIDE_RADIUS, view->pressed == CS_BTN_PREVIOUS);
        box(&band, px - 11.0f, cy, 2.5f, 12.0f, 1.0f, WHITE, ink);
        triangle(&band, px + 12.0f, cy - 12.0f, px + 12.0f, cy + 12.0f, px - 7.0f, cy, WHITE, ink);

        // Next: the same, the other way round.
        float nx = cx + CS_BTN_SIDE_DX;
        plate(&band, nx, cy, SIDE_RADIUS, view->pressed == CS_BTN_NEXT);
        box(&band, nx + 11.0f, cy, 2.5f, 12.0f, 1.0f, WHITE, ink);
        triangle(&band, nx - 12.0f, cy - 12.0f, nx - 12.0f, cy + 12.0f, nx + 7.0f, cy, WHITE, ink);

        // Volume: a plus above, a minus below.
        float uy = cy - CS_BTN_VOLUME_DY;
        plate(&band, cx, uy, VOLUME_RADIUS, view->pressed == CS_BTN_VOLUME_UP);
        box(&band, cx, uy, 11.0f, 2.5f, 1.0f, WHITE, ink);
        box(&band, cx, uy, 2.5f, 11.0f, 1.0f, WHITE, ink);

        float dy = cy + CS_BTN_VOLUME_DY;
        plate(&band, cx, dy, VOLUME_RADIUS, view->pressed == CS_BTN_VOLUME_DOWN);
        box(&band, cx, dy, 11.0f, 2.5f, 1.0f, WHITE, ink);
    }

    // Red for no network, amber for a network with no broker. Small, and in a
    // corner: it is for whoever wonders why the picture is an hour old.
    if (view->link != CS_LINK_UP) {
        const rgb_t red = {226, 62, 52};
        const rgb_t amber = {236, 164, 40};
        float mx = CS_SCREEN_W - 13.0f;
        float my = 13.0f;
        disc(&band, mx, my, 7.5f, BLACK, 0.6f);
        disc(&band, mx, my, 5.0f, view->link == CS_LINK_NO_WIFI ? red : amber, 1.0f);
    }
}

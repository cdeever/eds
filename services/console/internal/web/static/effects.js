// The stand's effects, as the firmware renders them
// (firmware/lp-stand/main/app/effects/effects.c, and the validation in
// scene.c). Same scene and same time, same pixels: this is a port, and if the
// firmware's effects change this has to change with them, or the picture on
// the page is of a stand that does not exist.
//
// Pure: no page, no clock. Gamma is left out on purpose, as it is on the
// wire - these are the colours intended, which is what a screen should show.

const PERIOD_SLOW_MS = 6000;
const PERIOD_FAST_MS = 1000;
const BREATHE_FLOOR = 0.25;
const SWEEP_BLEND = 0.06;
const MORPH_BLEND = 0.45;
const CHASE_MIN_WIDTH = 0.12;
const CHASE_MAX_WIDTH = 0.3;
const MAX_COLORS = 8;

// The effects the firmware knows. A name it does not know is shown as solid,
// which is what a stand does with one.
export const EFFECTS = ["solid", "breathe", "sweep", "morph", "chase"];

const clamp01 = (v) => (v > 0 ? (v < 1 ? v : 1) : 0); // NaN falls to 0

export function periodMs(speed) {
  return PERIOD_SLOW_MS + (PERIOD_FAST_MS - PERIOD_SLOW_MS) * clamp01(speed);
}

// validate is what the stand does to a scene before it shows it: clamp,
// drop colours with no share, and make the shares add up to one. Returns
// null for a scene with nothing in it to show.
export function validate(scene) {
  if (!scene || !Array.isArray(scene.palette)) return null;
  const palette = [];
  let total = 0;
  for (const sw of scene.palette.slice(0, MAX_COLORS)) {
    const w = Number(sw && sw.weight);
    if (!(w > 0) || !Array.isArray(sw.rgb)) continue;
    palette.push({ rgb: sw.rgb.slice(0, 3).map((c) => Math.max(0, Math.min(255, c | 0))), weight: w });
    total += w;
  }
  if (palette.length === 0 || !(total > 0)) return null;
  for (const sw of palette) sw.weight /= total;
  const effect = EFFECTS.includes(scene.effect) ? scene.effect : "solid";
  return { effect, speed: clamp01(scene.speed), brightness: clamp01(scene.brightness), palette };
}

function scale8(value, factor) {
  const scaled = value * factor;
  if (scaled <= 0) return 0;
  if (scaled >= 255) return 255;
  return Math.floor(scaled + 0.5);
}

const dim = (c, f) => [scale8(c[0], f), scale8(c[1], f), scale8(c[2], f)];
const mix = (a, b, t) => a.map((x, i) => Math.floor(x + (b[i] - x) * t + 0.5));

// phaseIn is where in a cycle of `period` milliseconds tMs falls, 0..1. The
// period is taken to the whole millisecond, as the firmware takes it.
function phaseIn(tMs, period) {
  const whole = Math.floor(period + 0.5);
  if (whole <= 0) return 0;
  return (Math.floor(tMs) % whole) / whole;
}

const phase01 = (tMs, speed) => phaseIn(tMs, periodMs(speed));

const smooth = (t) => (t <= 0 ? 0 : t >= 1 ? 1 : t * t * (3 - 2 * t));

// morphColor is the one colour the whole strip shows at `phase`: each colour
// has a turn, half by its share of the cover and half equal, and spends the
// end of it turning into the next.
function morphColor(scene, phase) {
  const n = scene.palette.length;
  if (n < 2) return scene.palette[0].rgb;
  let start = 0;
  for (let i = 0; i < n; i++) {
    const turn = 0.5 / n + 0.5 * scene.palette[i].weight;
    if (phase < start + turn || i + 1 === n) {
      const into = (phase - start) / turn;
      const hold = 1 - MORPH_BLEND;
      if (into <= hold) return scene.palette[i].rgb;
      return mix(scene.palette[i].rgb, scene.palette[(i + 1) % n].rgb, smooth((into - hold) / MORPH_BLEND));
    }
    start += turn;
  }
  return scene.palette[0].rgb;
}

// chaseColor is what position u shows when the lap is at `phase`: the
// dominant colour, with each accent a soft-edged patch travelling over it,
// alternate accents going opposite ways.
function chaseColor(scene, u, phase) {
  let color = scene.palette[0].rgb;
  const accents = scene.palette.length - 1;
  for (let k = 0; k < accents; k++) {
    const accent = scene.palette[k + 1];
    const half = Math.min(CHASE_MAX_WIDTH, CHASE_MIN_WIDTH + accent.weight) * 0.5;
    let centre = k / accents + (k % 2 === 0 ? phase : -phase);
    centre -= Math.floor(centre);
    let distance = Math.abs(u - centre);
    if (distance > 0.5) distance = 1 - distance;
    if (distance < half) {
      color = mix(color, accent.rgb, 0.5 + 0.5 * Math.cos((Math.PI * distance) / half));
    }
  }
  return color;
}

// sweepColor samples the palette at u (0..1 round the strip): bands as wide
// as their shares, cross-faded over the tail of each.
function sweepColor(scene, u) {
  u -= Math.floor(u);
  const n = scene.palette.length;
  let start = 0;
  for (let i = 0; i < n; i++) {
    const width = scene.palette[i].weight;
    const end = start + width;
    if (u < end || i + 1 === n) {
      const here = scene.palette[i].rgb;
      const next = scene.palette[(i + 1) % n].rgb;
      let blend = SWEEP_BLEND;
      if (blend > width * 0.5) blend = width * 0.5;
      if (blend <= 0) return here;
      const intoTail = u - (end - blend);
      if (intoTail <= 0) return here;
      return mix(here, next, intoTail / blend);
    }
    start = end;
  }
  return scene.palette[0].rgb;
}

// render returns `count` pixels, each [r, g, b], for a validated scene at
// tMs milliseconds.
export function render(scene, tMs, count) {
  const out = new Array(count);
  if (!scene || scene.palette.length === 0) return out.fill([0, 0, 0]);

  const dominant = scene.palette[0].rgb;
  const brightness = scene.brightness;

  if (scene.effect === "breathe") {
    const wave = 0.5 + 0.5 * Math.sin(2 * Math.PI * phase01(tMs, scene.speed));
    const level = BREATHE_FLOOR + (1 - BREATHE_FLOOR) * wave;
    return out.fill(dim(dominant, brightness * level));
  }
  if (scene.effect === "sweep") {
    const offset = phase01(tMs, scene.speed);
    for (let i = 0; i < count; i++) out[i] = dim(sweepColor(scene, i / count + offset), brightness);
    return out;
  }
  if (scene.effect === "morph") {
    const loop = periodMs(scene.speed) * 2 * scene.palette.length;
    return out.fill(dim(morphColor(scene, phaseIn(tMs, loop)), brightness));
  }
  if (scene.effect === "chase" && scene.palette.length > 1) {
    const phase = phaseIn(tMs, periodMs(scene.speed) * 2);
    for (let i = 0; i < count; i++) out[i] = dim(chaseColor(scene, i / count, phase), brightness);
    return out;
  }
  return out.fill(dim(dominant, brightness));
}

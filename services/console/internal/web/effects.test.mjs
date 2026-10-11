// The firmware's own effect tests (test/host/test_lp_stand.c), asked of the
// port. Run with: node --test
import assert from "node:assert/strict";
import { test } from "node:test";
import { periodMs, render, validate } from "./static/effects.js";

const STRIP = 60;
const twoColour = (over = {}) =>
  validate({
    effect: "solid", speed: 0.5, brightness: 1,
    palette: [{ rgb: [200, 20, 20], weight: 0.75 }, { rgb: [20, 20, 200], weight: 0.25 }],
    ...over,
  });

test("solid is uniform and scaled", () => {
  const px = render(twoColour({ brightness: 0.5 }), 0, STRIP);
  assert.deepEqual(px[0], [100, 10, 10]);
  assert.ok(px.every((p) => p.join() === px[0].join()));
});

test("zero brightness is black", () => {
  for (const effect of ["solid", "breathe", "sweep"]) {
    const px = render(twoColour({ effect, brightness: 0 }), 1234, STRIP);
    assert.ok(px.every((p) => p.join() === "0,0,0"), effect);
  }
});

test("breathe never goes fully dark, and does vary", () => {
  const scene = twoColour({ effect: "breathe" });
  let lo = 255, hi = 0;
  for (let t = 0; t < 8000; t += 25) {
    const r = render(scene, t, STRIP)[0][0];
    lo = Math.min(lo, r); hi = Math.max(hi, r);
  }
  assert.ok(lo > 20, `dropped to ${lo}`);
  assert.ok(hi - lo > 60, `range is only ${hi - lo}`);
});

test("breathe is the dominant colour, uniform across the strip", () => {
  const px = render(twoColour({ effect: "breathe" }), 1234, STRIP);
  assert.ok(px.every((p) => p.join() === px[0].join()));
  assert.ok(px[0][0] > px[0][2], "not the dominant colour");
});

test("sweep honours palette weights", () => {
  const px = render(twoColour({ effect: "sweep", speed: 0 }), 0, STRIP);
  const share = px.filter((p) => p[0] > p[2]).length / STRIP;
  assert.ok(share > 0.6 && share < 0.9, `the 0.75 colour covers ${share}`);
});

test("sweep scrolls, and comes round", () => {
  const scene = twoColour({ effect: "sweep", speed: 1 });
  const first = render(scene, 0, STRIP);
  assert.notDeepEqual(render(scene, 300, STRIP), first);
  assert.deepEqual(render(scene, periodMs(1), STRIP), first);
});

test("the period runs from six seconds to one", () => {
  assert.equal(periodMs(0), 6000);
  assert.equal(periodMs(1), 1000);
  assert.equal(periodMs(0.4), 4000);
  assert.equal(periodMs(7), 1000);
});

test("validate does what the stand does to a scene", () => {
  assert.equal(validate(null), null);
  assert.equal(validate({ effect: "solid", palette: [] }), null);
  assert.equal(validate({ effect: "solid", palette: [{ rgb: [1, 2, 3], weight: 0 }] }), null);

  const s = validate({
    effect: "strobe", speed: 9, brightness: -1,
    palette: [{ rgb: [300, -5, 7.9], weight: 3 }, { rgb: [1, 1, 1], weight: NaN }, { rgb: [2, 2, 2], weight: 1 }],
  });
  // An effect the stand does not know is shown as solid, not refused.
  assert.equal(s.effect, "solid");
  assert.equal(s.speed, 1);
  assert.equal(s.brightness, 0);
  assert.deepEqual(s.palette, [{ rgb: [255, 0, 7], weight: 0.75 }, { rgb: [2, 2, 2], weight: 0.25 }]);

  const many = validate({ effect: "sweep", palette: Array.from({ length: 12 }, () => ({ rgb: [9, 9, 9], weight: 1 })) });
  assert.equal(many.palette.length, 8);
});

// --- the effects that use the accents ---------------------------------------------

const accented = (over = {}) =>
  validate({
    effect: "morph", speed: 0.5, brightness: 1,
    palette: [
      { rgb: [30, 40, 200], weight: 0.8 },
      { rgb: [230, 30, 20], weight: 0.15 },
      { rgb: [240, 220, 90], weight: 0.05 },
    ],
    ...over,
  });
const far = (a, b) => Math.max(...a.map((x, i) => Math.abs(x - b[i])));

test("morph and chase are known effects, not shown as solid", () => {
  assert.equal(accented().effect, "morph");
  assert.equal(accented({ effect: "chase" }).effect, "chase");
});

test("morph is one colour for the whole strip, visits every colour, and returns", () => {
  const scene = accented();
  const loop = 3500 * 2 * 3;
  assert.deepEqual(render(scene, 0, STRIP)[0], [30, 40, 200]);
  const nearest = [255, 255, 255];
  for (let t = 0; t < loop; t += 25) {
    const px = render(scene, t, STRIP);
    assert.ok(px.every((p) => p.join() === px[0].join()), `not uniform at ${t}`);
    scene.palette.forEach((sw, c) => { nearest[c] = Math.min(nearest[c], far(px[0], sw.rgb)); });
  }
  assert.deepEqual(nearest, [0, 0, 0]);
  assert.deepEqual(render(scene, loop, STRIP)[0], render(scene, 0, STRIP)[0]);
});

test("morph gives the dominant colour the longest turn, and never jumps", () => {
  const scene = accented();
  const shown = [0, 0, 0];
  let before = render(scene, 0, 1)[0], worst = 0;
  for (let t = 0; t < 21000; t += 20) {
    const p = render(scene, t, 1)[0];
    scene.palette.forEach((sw, c) => { if (far(p, sw.rgb) === 0) shown[c]++; });
    worst = Math.max(worst, far(p, before));
    before = p;
  }
  assert.ok(shown[0] > shown[1] && shown[1] > shown[2], `turns ${shown}`);
  assert.ok(shown[2] * 20 > 1000, `the smallest accent shows for ${shown[2] * 20} ms`);
  assert.ok(worst < 14, `jumped ${worst} in one step`);
});

test("chase shows each accent over the dominant colour, wider for a larger share", () => {
  const scene = accented({ effect: "chase" });
  const px = render(scene, 875, STRIP);
  const ground = px.filter((p) => far(p, [30, 40, 200]) === 0).length;
  assert.ok(ground > STRIP / 2, `only ${ground} pixels are the dominant colour`);
  const patches = px.filter((p) => far(p, [30, 40, 200]) !== 0);
  const reddish = patches.filter((p) => p[1] < 100).length;
  const yellowish = patches.length - reddish;
  assert.ok(reddish > yellowish && yellowish >= 2, `${reddish} red, ${yellowish} yellow`);
  assert.ok(Math.min(...px.map((p) => far(p, [230, 30, 20]))) < 25);
  assert.ok(Math.min(...px.map((p) => far(p, [240, 220, 90]))) < 25);
});

test("chase moves, comes round, and has no edge at the seam", () => {
  const scene = accented({ effect: "chase" });
  const first = render(scene, 0, STRIP);
  assert.notDeepEqual(render(scene, 300, STRIP), first);
  assert.deepEqual(render(scene, 7000, STRIP), first);
  for (let t = 0; t < 7000; t += 50) {
    const px = render(scene, t, STRIP);
    let inside = 0;
    for (let i = 0; i + 1 < STRIP; i++) inside = Math.max(inside, far(px[i], px[i + 1]));
    assert.ok(far(px[STRIP - 1], px[0]) <= inside + 8, `seam at ${t}`);
  }
});

test("with one colour the accent effects are that colour", () => {
  for (const effect of ["morph", "chase"]) {
    const scene = validate({ effect, speed: 0.5, brightness: 1, palette: [{ rgb: [30, 40, 200], weight: 1 }] });
    for (let t = 0; t < 9000; t += 450) {
      assert.ok(render(scene, t, STRIP).every((p) => p.join() === "30,40,200"), effect);
    }
  }
});

// What the firmware's own code renders for the same scene, printed from
// firmware/lp-stand/main/app/effects/effects.c built for the host. The port is
// only a port if it lands on the same pixels. One step either way is allowed:
// the firmware works in single precision and this in double.
const FIRMWARE = {
  morph9000: [118, 36, 121],
  chase875: [[33,40,198],[49,39,183],[77,38,158],[113,36,126],[151,34,91],[187,32,59],[214,31,35],[228,30,22],[228,30,22],[214,31,35],[187,32,59],[151,34,91],[113,36,126],[77,38,158],[49,39,183],[33,40,198],[30,40,200],[30,40,200],[37,46,196],[77,80,175],[138,133,143],[198,184,112],[235,216,93],[235,216,93],[198,184,112],[138,133,143],[77,80,175],[37,46,196],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200],[30,40,200]],
};

test("the port lands on the firmware's pixels", () => {
  const close = (got, want, what) =>
    assert.ok(far(got, want) <= 1, `${what}: ${got} is not the firmware's ${want}`);
  close(render(accented(), 9000, STRIP)[0], FIRMWARE.morph9000, "morph at 9 s");
  const chase = render(accented({ effect: "chase" }), 875, STRIP);
  chase.forEach((p, i) => close(p, FIRMWARE.chase875[i], `chase pixel ${i}`));
  // And the sample is of something: patches, not sixty pixels of background.
  assert.ok(FIRMWARE.chase875.filter((p) => far(p, [30, 40, 200]) !== 0).length > 8);
});

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

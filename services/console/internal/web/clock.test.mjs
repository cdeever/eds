// Run with: node --test
import assert from "node:assert/strict";
import { test } from "node:test";
import { label, position, start } from "./static/clock.js";

const track = { title: "Abacab", duration_ms: 417959 };
const playing = (over = {}) => ({
  state: "playing", track, position_ms: 126243, changed_at: "2026-10-11T19:04:49.944Z", ...over,
});

test("a playing track counts on from where it was when it last changed", () => {
  // The picture was sent 20 s after the change, and 5 s have passed here since.
  const from = start(playing(), "2026-10-11T19:05:09.944Z", 1000);
  assert.equal(position(from, 1000), 126243 + 20000);
  assert.equal(position(from, 6000), 126243 + 25000);
});

test("a paused track stays where it was", () => {
  const from = start(playing({ state: "paused" }), "2026-10-11T19:30:00Z", 1000);
  assert.equal(position(from, 1000), 126243);
  assert.equal(position(from, 600000), 126243);
});

test("it never runs past the end of the track", () => {
  // Left open overnight with the last report still saying "playing".
  const from = start(playing(), "2026-10-12T07:00:00Z", 0);
  assert.equal(position(from, 0), track.duration_ms);
  assert.equal(position(from, 99999), track.duration_ms);
});

test("a track with no known length just counts", () => {
  const from = start(playing({ track: { title: "A stream" }, position_ms: 0 }), "2026-10-11T19:04:59.944Z", 0);
  assert.equal(position(from, 3000), 13000);
});

test("a sender's clock ahead of the server's does not run time backwards", () => {
  const from = start(playing(), "2026-10-11T19:04:48.000Z", 0);
  assert.equal(position(from, 0), 126243);
  assert.equal(position(from, 2000), 128243);
});

test("the viewer's own clock is never used", () => {
  // Only the difference between two readings of the page's own time matters.
  const a = start(playing(), "2026-10-11T19:05:09.944Z", 5);
  const b = start(playing(), "2026-10-11T19:05:09.944Z", 5e12);
  assert.equal(position(a, 5 + 3000), position(b, 5e12 + 3000));
});

test("with nothing to count from, nothing is shown", () => {
  assert.equal(start(null, "2026-10-11T19:05:09Z", 0), null);
  assert.equal(start({ state: "stopped" }, "2026-10-11T19:05:09Z", 0), null);
  assert.equal(start(playing({ state: "stopped" }), "2026-10-11T19:05:09Z", 0), null);
  assert.equal(start(playing({ position_ms: undefined }), "2026-10-11T19:05:09Z", 0), null);
  assert.equal(start(playing({ position_ms: -4 }), "2026-10-11T19:05:09Z", 0), null);
  // Playing, with no instant to count from: where it was is not where it is.
  assert.equal(start(playing({ changed_at: undefined }), "2026-10-11T19:05:09Z", 0), null);
  assert.equal(start(playing(), undefined, 0), null);
  assert.equal(position(null, 0), null);
});

test("times read as minutes and seconds", () => {
  assert.equal(label(0), "0:00");
  assert.equal(label(999), "0:00");
  assert.equal(label(126243), "2:06");
  assert.equal(label(417959), "6:57");
  assert.equal(label(3726000), "1:02:06");
});

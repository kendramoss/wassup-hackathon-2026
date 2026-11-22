import assert from "node:assert/strict";
import { test } from "node:test";
import { advanceScroll, newScrollState, startScroll } from "../src/components/op/landing/arrival.ts";

test("the app's own smooth scroll to fig.2 keeps its arrival replay", () => {
  let s = startScroll(newScrollState(1000), 1000, 3000, 3000, 2, 500);
  for (const [y, now] of [[1200, 600], [1600, 700], [2200, 800], [2900, 900], [3000, 1000]]) {
    const r = advanceScroll(s, y, now);
    assert.equal(r.interrupted, false, `frame at y=${y}`);
    assert.equal(r.state.arrival?.pane, 2, `frame at y=${y}`);
    s = r.state;
  }
  assert.equal(s.seek, null, "the settled seek is released");
  assert.equal(s.arrival?.pane, 2, "the replay still runs to its own clock");
});

test("scrolling backward during fig.3's smooth arrival interrupts the replay", () => {
  const s = startScroll(newScrollState(1000), 1000, 3000, 3000, 3, 500);
  const r = advanceScroll(s, 800, 600); // distance grows from 2000 to 2200
  assert.equal(r.interrupted, true);
  assert.equal(r.state.arrival, null);
  assert.equal(r.state.seek, null, "the app stops seeking too");
  const later = advanceScroll(r.state, 800, 5000); // past the old 3.4s expiry
  assert.equal(later.state.arrival, null, "the replay does not come back at expiry");
});

test("a visitor scroll after the seek settled interrupts fig.2 immediately", () => {
  let s = startScroll(newScrollState(1000), 1000, 3000, 2000, 2, 500);
  s = advanceScroll(s, 3000, 1000).state;
  assert.equal(s.seek, null);
  assert.equal(s.arrival?.pane, 2);
  const idle = advanceScroll(s, 3000, 1100);
  assert.equal(idle.interrupted, false, "staying put does not interrupt");
  const moved = advanceScroll(idle.state, 2600, 1200);
  assert.equal(moved.interrupted, true);
  assert.equal(moved.state.arrival, null);
});

test("an idle replay and a non-replay step are left alone", () => {
  const s = startScroll(newScrollState(3000), 3000, 3000, 3000, null, 500);
  const r = advanceScroll(s, 2000, 600);
  assert.equal(r.interrupted, false, "page-edge stepping has no replay to cancel");
  assert.equal(r.state.arrival, null);
});

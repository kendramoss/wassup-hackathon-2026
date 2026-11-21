import assert from "node:assert/strict";
import { test } from "node:test";
import { ARRIVE, FREE_SECTIONS, PHASE_STOPS, SCREENS, STOPS, arrivalFor, fig2At, fig3At, nextDestination, paneAt, phaseStop, stepAt } from "../src/components/op/landing/timeline.ts";

test("each heat-gun phase stop lands inside its own step", () => {
  PHASE_STOPS.forEach((_, i) => {
    assert.equal(stepAt(phaseStop(i) / 4), i, `phase ${i + 1}`);
  });
});

test("phase stops are ordered and stay inside fig.1", () => {
  for (let i = 1; i < PHASE_STOPS.length; i++) assert.ok(phaseStop(i) > phaseStop(i - 1));
  assert.ok(phaseStop(0) >= 0);
  assert.ok(phaseStop(PHASE_STOPS.length - 1) < 4);
});

test("figure stops land on their own pane", () => {
  assert.equal(Math.round(paneAt(ARRIVE[0])), 0);
  assert.equal(Math.round(paneAt(ARRIVE[1])), 1);
  assert.equal(Math.round(paneAt(ARRIVE[2])), 2);
});

test("a next-figure jump arrives with that figure fully drawn", () => {
  assert.equal(fig2At(ARRIVE[1]), 1);
  assert.equal(fig3At(ARRIVE[2]), 1);
  assert.ok(ARRIVE.every((p) => p >= 0 && p <= SCREENS));
});

test("the old fig.2 stop landed at the start of its animation", () => {
  assert.ok(fig2At(5.5) < 0.1);
  assert.equal(fig2At(ARRIVE[1]), 1);
});

test("wheel and key steps walk the four phases then the two finished figures", () => {
  assert.equal(STOPS.length, 6);
  STOPS.slice(0, 4).forEach((s, i) => assert.equal(stepAt(s / 4), i, `step ${i + 1}`));
  assert.equal(STOPS[4], ARRIVE[1]);
  assert.equal(STOPS[5], ARRIVE[2]);
  for (let i = 1; i < STOPS.length; i++) assert.ok(STOPS[i] > STOPS[i - 1]);
});

test("only fig.2 and fig.3 replay a timed arrival animation", () => {
  assert.equal(arrivalFor(ARRIVE[1]), 2);
  assert.equal(arrivalFor(ARRIVE[2]), 3);
  STOPS.slice(0, 4).forEach((s) => assert.equal(arrivalFor(s), null));
});

test("one more down from fig.3 enters the first free section, not the page bottom (#409)", () => {
  assert.deepEqual(nextDestination(ARRIVE[2], 1), { kind: "section", index: 0 });
  assert.deepEqual(nextDestination(SCREENS, 1), { kind: "section", index: 0 });
  assert.notDeepEqual(nextDestination(ARRIVE[2], 1), { kind: "bottom" });
  assert.deepEqual(FREE_SECTIONS.map((s) => s.key), ["can", "who", "story", "footer"]);
});

test("back from below the stage returns to fig.3; back from the first stop reaches the page top", () => {
  assert.deepEqual(nextDestination(SCREENS, -1), { kind: "stop", progress: ARRIVE[2] });
  assert.deepEqual(nextDestination(ARRIVE[2], -1), { kind: "stop", progress: ARRIVE[1] });
  assert.deepEqual(nextDestination(ARRIVE[0], -1), { kind: "top" });
  assert.deepEqual(nextDestination(0, -1), { kind: "top" });
  assert.deepEqual(nextDestination(ARRIVE[0], 1), { kind: "stop", progress: STOPS[1] });
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { watchView } from "../src/lib/watch-state.ts";

test("legacy watch (no basis) keeps its old reading", () => {
  assert.equal(watchView("changed", undefined, undefined).kind, "changed");
  assert.equal(watchView("unchanged", null, null).kind, "unchanged");
  assert.equal(watchView(null, null, null).kind, "none");
});

test("precedent basis keeps the status meaning", () => {
  assert.equal(watchView("changed", "precedent", "auto").kind, "changed");
  assert.equal(watchView("unchanged", "precedent", "auto").kind, "unchanged");
  assert.equal(watchView("changed", "precedent", "needs_review").kind, "changed");
});

test("no non-precedent basis ever reads as unchanged or as a ruling change", () => {
  for (const basis of ["regime", "method", "specialist_unavailable", "unverified"]) {
    for (const status of ["changed", "unchanged"]) {
      const v = watchView(status, basis, "needs_review", "an expert reviews");
      assert.equal(v.kind, "review", `${basis}/${status}`);
      assert.equal(v.label, "Needs review");
      assert.match(v.note, /an expert reviews/);
      assert.doesNotMatch(v.note, /^The origin is the same/);
    }
  }
});

test("unchanged precedent result flagged for review is not plain unchanged", () => {
  const v = watchView("unchanged", "precedent", "needs_review", "low confidence");
  assert.equal(v.kind, "review");
  assert.match(v.note, /low confidence/);
});

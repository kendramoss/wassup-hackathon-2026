import assert from "node:assert/strict";
import { test } from "node:test";
import { watchView } from "../src/lib/watch-state.ts";

test("a watch with no basis is never read as Changed or Unchanged", () => {
  // Saved before the legal regime was recorded: it cannot say the rulings
  // moved or that they did not (D101). Re-check replaces it.
  for (const status of ["changed", "unchanged"]) {
    for (const basis of [undefined, null, "", "something-new"]) {
      const v = watchView(status, basis, undefined);
      assert.equal(v.kind, "recheck", `${status}/${basis}`);
      assert.equal(v.label, "Re-check needed");
      assert.match(v.note, /older version of Unde that did not record which set of origin rules/);
      assert.match(v.note, /Re-check/);
      assert.doesNotMatch(v.note, /^(Changed|Unchanged)|with today's rulings|the rulings (moved|did not)/i);
    }
  }
  assert.equal(watchView(null, null, null).kind, "none");
  assert.equal(watchView(undefined, undefined, undefined).kind, "none");
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

test("a non-precedent basis explains itself in words and does not repeat an approving triage reason", () => {
  const approving = "confidence 0.97 >= 0.92 and in scope";
  for (const basis of ["regime", "method", "specialist_unavailable", "unverified"]) {
    // route auto (or absent) with an approving reason: the reason is dropped.
    for (const route of ["auto", undefined, null]) {
      const v = watchView("changed", basis, route, approving);
      assert.equal(v.kind, "review", `${basis}/${route}`);
      assert.doesNotMatch(v.note, /0\.97|0\.92|in scope|>=/, `${basis}/${route}`);
      assert.match(v.note, /not a ruling change|cannot be compared/, basis);
    }
    // a review route keeps its own reason, since that is why a person looks.
    const r = watchView("unchanged", basis, "needs_review", "an expert reviews");
    assert.match(r.note, /an expert reviews/, basis);
  }
});

test("each non-precedent basis has its own plain explanation", () => {
  const notes = new Set();
  for (const basis of ["regime", "method", "specialist_unavailable", "unverified"]) notes.add(watchView("changed", basis, "auto").note);
  assert.equal(notes.size, 4);
  assert.match(watchView("changed", "method", "auto").note, /engine version/);
  assert.match(watchView("changed", "regime", "auto").note, /different legal regime/);
  assert.match(watchView("changed", "specialist_unavailable", "auto").note, /failed/);
  assert.match(watchView("changed", "unverified", "auto").note, /not confirmed/);
});

test("a precedent result sent to review keeps the triage reason only when it is a review reason", () => {
  const v = watchView("unchanged", "precedent", "needs_review", "low confidence");
  assert.match(v.note, /low confidence/);
  const approving = watchView("unchanged", "precedent", "needs_info", "more facts needed");
  assert.match(approving.note, /more facts needed/);
});

// ---- N1: attention state and the changed+review pair ------------------------

test("every state that needs a person's attention says so; unchanged and none do not", () => {
  const attention = (status, basis, route) => watchView(status, basis, route).attention;
  assert.equal(attention("changed", "precedent", "auto"), true);
  assert.equal(attention("changed", undefined, undefined), true, "legacy Re-check needed");
  assert.equal(attention("unchanged", undefined, undefined), true, "legacy Re-check needed");
  for (const basis of ["regime", "method", "specialist_unavailable", "unverified"]) assert.equal(attention("unchanged", basis, "auto"), true, basis);
  assert.equal(attention("unchanged", "precedent", "needs_review"), true);
  assert.equal(attention("unchanged", "precedent", "auto"), false);
  assert.equal(attention(null, null, null), false);
});

test("a genuine precedent change stays Changed and also shows that a person must look", () => {
  for (const route of ["needs_review", "needs_info"]) {
    const v = watchView("changed", "precedent", route, "confidence 0.60 is below the bar");
    assert.equal(v.kind, "changed");
    assert.equal(v.label, "Changed");
    assert.equal(v.attention, true);
    assert.match(v.reviewNote, /needs a person to look/);
    assert.match(v.reviewNote, /below the bar/);
  }
});

test("a precedent change on the auto route shows no review text and no approving reason", () => {
  const v = watchView("changed", "precedent", "auto", "confidence 0.97 >= 0.92 and in scope");
  assert.equal(v.kind, "changed");
  assert.equal(v.reviewNote, "");
  assert.equal(v.attention, true, "a change is always worth a look");
  const none = watchView("changed", "precedent", undefined, "confidence 0.97 >= 0.92 and in scope");
  assert.equal(none.reviewNote, "");
});

test("the conflict panel says 'follows' only for a ruling that held the predicted origin (D180)", async () => {
  const { conflictLean } = await import("../src/lib/conflict.ts");
  assert.equal(conflictLean(true, "China", "China"), "follows");
  assert.equal(conflictLean(true, "Vietnam", "China"), "closest");
  assert.equal(conflictLean(true, undefined, "China"), "closest");
  assert.equal(conflictLean(false, "China", "China"), null);
});

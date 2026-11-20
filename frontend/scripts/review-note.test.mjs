import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// #278 / #282: the shared ReviewNote and its copy keys. The strings are the
// reviewed #282 wording, pinned here so a change to them is a deliberate edit.
const { ReviewNote } = await import("../src/components/op/review-note.tsx");
const { copy } = await import("../src/lib/copy.ts");

const EXACT = {
  originScope: "U.S. origin only. Not for trade-agreement claims, certificates of origin or non-U.S. rules.",
  outOfScope: "This looks like a case this tool does not decide. A licensed customs broker should review it.",
  part102: "Marking origin under the Part 102 rules. This is not USMCA qualification or regional value content.",
  planScope: "Plans use the general origin test. They do not apply textile, Part 102 or pharmaceutical rules, and only motorised tools and small appliances, LiDAR modules and LED lights are planned.",
  draftNote: "A draft ruling request for a broker to complete and file. It asks CBP a question; it is not a determination.",
  part177Label: "Ruling request letter (19 CFR Part 177)",
  part177Help: "Part 177 is how to ask CBP for a ruling. It is not an origin rule.",
  compareScope: "Compared on U.S. origin only. Section 232 duties are not included.",
};

test("the #282 strings are the reviewed wording, character for character", () => {
  for (const [k, v] of Object.entries(EXACT)) assert.equal(copy.jurisdiction[k], v, k);
});

test("strings whose screen does not exist are not shipped", () => {
  // No plan-error screen for a family without a template, and no settings or
  // about page for the "What this does not cover" list.
  assert.equal(copy.jurisdiction.noTemplate, undefined);
  assert.equal(copy.jurisdiction.notCovered, undefined);
});

test("each place renders its lines once, as a note a screen reader can name", () => {
  const origin = html(ReviewNote, { place: "origin" });
  const t = text(origin);
  assert.equal((t.match(/U\.S\. origin only/g) ?? []).length, 1);
  assert.match(origin, /role="note"/);
  assert.match(origin, /aria-label="[^"]+"/);
  assert.match(origin, /class="[^"]*no-print/);
  assert.match(t, new RegExp(copy.review.origin.slice(0, 30)));
  for (const place of ["plan", "compare", "scenarios", "draft"]) {
    const m = html(ReviewNote, { place });
    assert.equal((m.match(/role="note"/g) ?? []).length, 1, place);
    assert.match(m, /class="[^"]*no-print/, place);
  }
  assert.ok(text(html(ReviewNote, { place: "plan", best: true })).includes(EXACT.planScope));
  assert.ok(text(html(ReviewNote, { place: "compare" })).includes(EXACT.compareScope));
  assert.ok(text(html(ReviewNote, { place: "scenarios" })).includes(EXACT.compareScope));
  assert.ok(text(html(ReviewNote, { place: "draft" })).includes(EXACT.draftNote));
});

test("bounded best-plan wording: searched moves, the entered costs, no unmodelled factors; never optimal, cheapest or guaranteed", () => {
  for (const place of ["plan", "compare", "scenarios"]) {
    const t = text(html(ReviewNote, { place, best: true }));
    assert.match(t, /moves searched|scenarios you entered/i, place);
    assert.match(t, /costs you entered/i, place);
    assert.match(t, /do(es)? not include|not counted/i, place);
  }
  const everything = JSON.stringify([copy.review, copy.jurisdiction]);
  assert.doesNotMatch(everything, /optimal|cheapest|guarantee|certif(ied|ication) by|approved by CBP/i);
  assert.doesNotMatch(JSON.stringify(copy.review), /\bblock|reject|detect/i, "no claim that the engine blocks or detects non-U.S. queries");
  assert.doesNotMatch(JSON.stringify(copy.jurisdiction), /\bblock|reject|detect/i);
});

test("the plan note without a best plan drops only the best-of line", () => {
  const t = text(html(ReviewNote, { place: "plan", best: false }));
  assert.doesNotMatch(t, /Best of the moves searched/);
  assert.ok(t.includes(EXACT.planScope));
});

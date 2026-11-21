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
  planScope: "Plans use the general origin test. They do not apply textile, Part 102 or pharmaceutical rules, and only products in a family with a plan template (13 families, listed in the family picker) are planned.",
  draftNote: "A draft ruling request for a broker to complete and file. It asks CBP a question; it is not a determination.",
  part177Label: "Ruling request letter (19 CFR Part 177)",
  part177Help: "Part 177 is how to ask CBP for a ruling. It is not an origin rule.",
  compareScope: "Compared on U.S. origin only. Section 232 duties are not included.",
};

test("the #282 strings are the reviewed wording, character for character", () => {
  for (const [k, v] of Object.entries(EXACT)) assert.equal(copy.jurisdiction[k], v, k);
});

test("strings whose screen does not exist are not shipped", () => {
  // No plan-error screen for a family without a template. (The not-covered list
  // lives on the accuracy page, "What we don't claim".)
  assert.equal(copy.jurisdiction.noTemplate, undefined);
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
    assert.match(t, /inputs shown|assumptions shown/i, place);
    assert.match(t, /prefilled or illustrative/i, place);
    assert.match(t, /do(es)? not include/i, place);
    assert.doesNotMatch(t, /costs you entered/i, place);
    // Blank-as-zero is a legacy plan ranking fact only; compare and scenarios
    // price a missing input as unpriced.
    if (place === "plan") assert.match(t, /When you leave a cost blank, the ranking may count it as zero, which is not a verified expense/);
    else {
      assert.doesNotMatch(t, /zero/i, place);
      assert.match(t, /missing required input leaves that figure unpriced/, place);
    }
  }
  const everything = JSON.stringify([copy.review, copy.jurisdiction]);
  assert.doesNotMatch(everything, /optimal|cheapest|guarantee|certif(ied|ication) by|approved by CBP/i);
  assert.doesNotMatch(JSON.stringify(copy.review), /\bblock|reject|detect/i, "no claim that the engine blocks or detects non-U.S. queries");
  // The one line that says "detect"/"block" is the accuracy-page line saying there is NO such check.
  const scope = { ...copy.jurisdiction };
  delete scope.noDetector;
  assert.doesNotMatch(JSON.stringify(scope), /\bblock|reject|detect/i);
});

test("the plan note without a best plan drops only the best-of line", () => {
  const t = text(html(ReviewNote, { place: "plan", best: false }));
  assert.doesNotMatch(t, /Best of the moves searched/);
  assert.ok(t.includes(EXACT.planScope));
});

test("the Auto-accepted chip's hover text no longer says it skips expert review and agrees with the origin note", () => {
  const t = copy.terms.auto.text;
  assert.doesNotMatch(t, /skip expert review/i);
  assert.match(t, /configured confidence and scope checks/);
  assert.match(t, /not legal approval/);
  assert.match(t, /licensed customs broker or counsel should review/);
  assert.doesNotMatch(t, /human|queue/i, "no claim of a human review queue");
  assert.match(t, /96% of the time/, "the held-out figure is kept");
  assert.equal(copy.terms.auto.match.source, "Auto-accepted");
});

test("the Part 102 chip and card are named for what they are; internal regime ids are unchanged", async () => {
  const { regimeOf } = await import("../src/lib/regime.ts");
  const r = regimeOf({ prompt_versions: { engine: "usmca-v1" } });
  assert.equal(r.key, "usmca");
  assert.equal(r.label, "Part 102 marking origin");
  assert.equal(regimeOf({ triage: { reason: "usmca rules: x" } }).key, "usmca");
  assert.equal(regimeOf({ prompt_versions: { engine: "textile-v1" } }).label, "Textile rules (19 CFR 102.21)");
  assert.equal(regimeOf({ prompt_versions: { engine: "pharma-v1" } }).label, "Pharmaceutical rules");
  const { readFile } = await import("node:fs/promises");
  const src = await readFile(new URL("../src/components/op/screens/accuracy-page.tsx", import.meta.url), "utf8");
  assert.doesNotMatch(src, /USMCA rules \(19 CFR 102\)/);
  assert.match(src, /Card title="Part 102 marking origin"/);
});

test("the accuracy page lists what is not covered, with no detector claim and no invented Section 301 limit", async () => {
  const { readFile } = await import("node:fs/promises");
  const src = await readFile(new URL("../src/components/op/screens/accuracy-page.tsx", import.meta.url), "utf8");
  assert.match(src, /copy\.jurisdiction\.notCovered/);
  assert.match(src, /copy\.jurisdiction\.noDetector/);
  const n = copy.jurisdiction.notCovered;
  for (const s of ["EU Binding Origin Information", "USMCA regional value content", "certificates of origin", "drawback", "antidumping and countervailing duty", "Section 232 duties are not included"]) assert.ok(n.includes(s), s);
  assert.doesNotMatch(n, /301/);
  assert.match(copy.jurisdiction.noDetector, /no automatic check/);
  assert.doesNotMatch(copy.jurisdiction.noDetector, /is blocked|are blocked|rejects/);
});

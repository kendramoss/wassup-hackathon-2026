import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// Judge panel round 13, item 1 (D620): the words around the Part 102 engine
// follow the engine in production. usmca-v1's figures are labelled as
// usmca-v1's, and usmca-v5 (D602) has its own auto-accept share (D613).
const root = new URL("../../", import.meta.url);
const read = (p) => readFileSync(new URL(p, root), "utf8");
const summary = JSON.parse(read("backend/internal/evalsummary/summary.json"));
const item = (k) => summary.items.find((i) => i.key === k);

test("the served summary carries usmca-v5's own share at confidence 0.95 (11 of 32, all right)", () => {
  const cov = item("usmca_v5_final_conf95_coverage");
  const acc = item("usmca_v5_final_conf95_accuracy");
  assert.equal(cov.n, 32);
  assert.equal(Math.round(cov.value * cov.n), 11);
  assert.equal(acc.n, 11);
  assert.equal(acc.value, 1);
  const cov2 = item("usmca_v5_final2_conf95_coverage");
  const acc2 = item("usmca_v5_final2_conf95_accuracy");
  assert.equal(Math.round(cov2.value * cov2.n), 6);
  assert.equal(Math.round(acc2.value * acc2.n), 5);
});

test("/accuracy gives usmca-v1's 0.95 share as the earlier engine's and usmca-v5's beside it", () => {
  const page = read("frontend/src/components/op/screens/accuracy-page.tsx").replace(/\s+/g, " ");
  assert.match(page, /scored first, with the earlier engine \(usmca-v1\)\. That engine was \{pct\(usAuto\?\.value\)\} right at confidence 0\.95 or above/);
  assert.match(page, /get\("usmca_v5_final_conf95_coverage"\)/);
  assert.match(page, /It auto-accepts fewer answers: confidence 0\.95 or above on/);
  assert.match(page, /That is a candidate comparison, not a fresh test \(p = 0\.22\)/);
});

test("/method labels the plan agreement figures usmca-v1's and names D591 and D602", async () => {
  const { MethodPage } = await import("../src/components/op/screens/method-page.tsx");
  const t = text(html(MethodPage, {}));
  assert.match(t, /Re-run with the Part 102 engine then in use \(usmca-v1\) predicting each move, its prediction agrees with the rule on 13 of the 14 moves/);
  assert.match(t, /usmca-v1 agreed with the rule on 3 of 6 moves/);
  assert.match(t, /D591/);
  assert.match(t, /Since 22 Nov the Part 102 engine is usmca-v5, which fills a short 102\.11 checklist first\. On the development plan rows it agrees with the rule on 56 of 57 decided rows, against 43 for usmca-v1/);
  // D631: the held-out plan set was re-run with usmca-v5; the line reports it.
  assert.doesNotMatch(t, /have not been re-run with it yet/);
  assert.match(t, /with usmca-v5 and 20 families, planned 8 of 18; the engine agreed with the rule on 6 of 6 decided moves/);
});

test("README, DEMO and the Part 102 example note carry the same labels (#631, #634)", () => {
  const readme = read("README.md");
  for (const l of readme.split("\n").filter((l) => /14 of 29 held out/.test(l))) assert.match(l, /12 of 29 held out \(11 agree; D560\)/);
  assert.match(readme, /`usmca-v1`\) agrees with the rule on 13 of 14 moves/);
  assert.match(readme, /40 against 36 \(a candidate comparison on rulings scored before, not a fresh test; p = 0\.22\)/);
  assert.match(readme, /17 of 45 \(16 right\) against 22 \(D602, D613\)/);
  assert.doesNotMatch(read("docs/DEMO.md"), /\b14 product families/);
  const copy = read("frontend/src/lib/copy.ts");
  // D631: the example is now a usmca-v5 run; usmca-v1's figures stay labelled as usmca-v1's.
  assert.match(copy, /On the three development searches usmca-v1 agreed with the rule on 13 of 14 moves \(D541\)/);
  assert.match(copy, /one plan search with the production Part 102 engine \(usmca-v5\), run once \(D630\)/);
  assert.match(readme, /usmca-v5` and 20 families planned 8 of 18, and the engine agreed with the rule on 6 of 6 decided moves/);
  assert.doesNotMatch(readme, /have not been re-run with `usmca-v5`/);
});

test("the new figures are in docs/NUMBERS.md", () => {
  const numbers = read("docs/NUMBERS.md");
  assert.ok(numbers.includes("window 1: 11 of 32 (34.4%), 11 right; window 2: 6 of 13 (46.2%), 5 right"));
  assert.ok(numbers.includes("56 of 57"));
  assert.ok(numbers.includes("40 of 45 (usmca-v1 25 / 11 / 36)"));
  assert.ok(numbers.includes("17 accepted, 16 right"));
});

// Judge panel round 14 (#643, D642): window 2's Part 102 figures carry D612's
// precedent-pool note; /method no longer calls window 2 usmca-v5's clean
// check; the held-out plan figure on /accuracy carries the base-rate caveat.
test("window-2 Part 102 figures carry the 15-ruling precedent pool note (D612)", () => {
  for (const k of ["usmca_final2_accuracy", "usmca_v5_final2_accuracy", "usmca_v5_final2_conf95_accuracy", "usmca_v5_final2_conf95_coverage"]) {
    assert.match(item(k).label, /15 window-2 rulings only,? \(?D612/, k);
  }
  const page = read("frontend/src/components/op/screens/accuracy-page.tsx").replace(/\s+/g, " ");
  assert.match(page, /\{later\(us2\)\} \{us2 \? " The later window's Part 102 runs had only the 15 window-2 rulings as precedents, not the full pool \(D612\)\." : ""\}/);
  assert.match(page, /with usmca-v5 and 20 families, 8 of 18 and 6 of 6; five of the six are moves where the rule keeps the country CBP held\./);
});

test("/method: usmca-v5 has no clean held-out check (D602, D612)", async () => {
  const { MethodPage } = await import("../src/components/op/screens/method-page.tsx");
  const t = text(html(MethodPage, {}));
  assert.doesNotMatch(t, /T[h]eir clean check is the second window/); // spelled so claimscheck does not flag the test
  assert.match(t, /For the textile and pharmaceutical engines the clean check is the second window, scored once with nothing changed/);
  assert.match(t, /The Part 102 engine in use, usmca-v5, has no clean held-out check: it was picked on development rulings and then scored once on both windows, which usmca-v1 had already been scored on, so that is a candidate comparison/);
  assert.match(t, /only the 15 window-2 rulings as precedents/);
});

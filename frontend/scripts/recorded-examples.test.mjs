import { html, text } from "./render-support.mjs";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { stubNavigation } from "./lib/hook-runtime.mjs";

// Judge panel round 10, item 2: the per-move rule check and the revocation
// alert had no visible instance on the site. Two recorded, read-only pages
// show them from committed files (rebuilt by plans-example and
// alerts-example in make reproduce); the tour's last step and /method link
// both.
stubNavigation({ id: "case_x", search: "tour=1&from=case_878d76540b91" });
const { PlanRuleExample } = await import("../src/components/op/screens/plan-rule-example.tsx");
const { RevocationExample } = await import("../src/components/op/screens/revocation-example.tsx");
const { RuleCheckExample } = await import("../src/components/op/screens/rule-check-example.tsx");
const { Part102PlanExample } = await import("../src/components/op/screens/part102-plan-example.tsx");
const { DemoTour } = await import("../src/components/op/demo-tour.tsx");
const json = (f) => JSON.parse(readFileSync(new URL(`../src/lib/${f}`, import.meta.url), "utf8"));
const plan = json("plan-rule-example.json");
const rev = json("revocation-example.json");

test("the plan example is N354220's development search, with two moves flagged: precedent supports, the rule gives China", () => {
  assert.equal(plan.ruling, "N354220");
  assert.equal(plan.split, "dev");
  assert.equal(plan.target, "Vietnam");
  assert.equal(plan.flagged, 2);
  const flagged = plan.moves.filter((m) => m.rule_check.review);
  assert.equal(flagged.length, 2);
  for (const m of flagged) {
    assert.equal(m.verdict.precedent_supported, true);
    assert.equal(m.rule_check.origin, "China");
    assert.equal(m.shown, false, "both below the bar (D442)");
  }
});

test("the plan example page shows both checks on each move, the amber flag, and stays read only", () => {
  const m = html(PlanRuleExample, {});
  const t = text(m);
  assert.match(t, /Recorded example/);
  assert.match(t, /nothing here calls a model or saves anything/);
  assert.match(t, /14-in-1 outdoor and wilderness survival tool/);
  assert.match(t, /An earlier ruling supports this move, but the 102\.20 rule gives China: worth a broker's look\./);
  assert.match(t, /102\.20 gives China: finished there/);
  assert.match(t, /An earlier ruling supports it \( ?N334762 ?\), check confidence 0\.85\./);
  assert.match(t, /Below the 0\.92 bar, so not a shown plan\./);
  assert.match(t, /Unverified suggestions/);
  assert.equal((m.match(/data-move="flagged"/g) ?? []).length, 2);
  assert.match(m, /data-plan-rule="other_country"/);
  assert.match(m, /border-ask-line bg-ask-wash/, "the flagged chip is amber");
  assert.match(t, /Part 102 plan searches, where 102\.20 is the test, are scored apart: three on development rulings \(D502\)\./);
  assert.doesNotMatch(m, /<form|<button|cases\/new/);
});

test("the revocation example is N356467 citing N344694, modified by H344999 after the case date", () => {
  assert.equal(rev.ruling, "N356467");
  assert.equal(rev.split, "dev");
  assert.deepEqual(rev.ruling_alerts.counts, { final: 1, proposed: 0 });
  const a = rev.ruling_alerts.items[0];
  assert.equal(a.ruling, "N344694");
  assert.equal(a.by, "H344999");
  assert.equal(a.date, "2026-05-06");
  assert.ok(a.roles.includes("cited"));
  assert.ok(!a.before_case && a.in_effect);
  assert.ok(rev.case_date < a.date);
});

test("the revocation page shows the box, whether the change touches the answer, a disabled re-check and what a re-check does", () => {
  const m = html(RevocationExample, {});
  const t = text(m);
  assert.match(t, /Recorded example/);
  assert.match(t, /A ruling this case relies on has changed/);
  assert.match(t, /Ruling N344694 cited by this answer was modified by H344999 on 6 May 2026/);
  // D531: the box says the change does not touch this answer (Vietnam).
  assert.match(t, /H344999 modified N344694, and the origin held moved from China to Taiwan\. This case's answer was Vietnam, so the change does not touch the answer directly\. Re-check still recommended\./);
  assert.match(m, /data-relevance="not_answer"/);
  assert.match(t, /1 cited ruling revoked or modified/);
  assert.doesNotMatch(t, /bearing on the answer/);
  assert.match(t, /recorded example: nothing is run here/);
  assert.match(m, /<button type="button" disabled=""/, "Re-check now is shown disabled");
  assert.equal((m.match(/<button/g) ?? []).length, 1, "the only button is the disabled one");
  assert.match(t, /What a re-check does/);
  assert.match(t, /The saved answer is never changed\./);
  assert.match(t, /Taiwan/);
  assert.doesNotMatch(m, /<form|cases\/new/);
});

test("the recorded examples link back to the tour and on to the next one", () => {
  const rc = html(RuleCheckExample, {});
  assert.match(rc, /href="\/examples\/plan-rule-check\?tour=1&amp;from=case_878d76540b91"/);
  assert.match(text(rc), /Next recorded example: a plan move the rule does not support →/);
  const pl = html(PlanRuleExample, {});
  assert.match(pl, /href="\/cases\/case_878d76540b91\/draft\?tour=1"/);
  assert.match(pl, /href="\/examples\/part102-plan\?tour=1&amp;from=case_878d76540b91"/);
  const p1 = html(Part102PlanExample, {});
  assert.match(p1, /href="\/cases\/case_878d76540b91\/draft\?tour=1"/);
  assert.match(p1, /href="\/examples\/revocation-alert\?tour=1&amp;from=case_878d76540b91"/);
  const rv = html(RevocationExample, {});
  assert.match(rv, /href="\/cases\/case_878d76540b91\/draft\?tour=1"/);
  assert.doesNotMatch(text(rv), /Next recorded example/);
});

test("the tour's last step lists every recorded example; earlier steps do not", () => {
  const draft = html(DemoTour, { id: "case_878d76540b91", tab: "draft" });
  assert.match(text(draft), /More recorded examples:/);
  for (const p of ["rule-check", "plan-rule-check", "part102-plan", "revocation-alert"]) assert.match(draft, new RegExp(`href="/examples/${p}\\?tour=1&amp;from=case_878d76540b91"`));
  assert.doesNotMatch(text(html(DemoTour, { id: "case_878d76540b91", tab: "plan" })), /More recorded examples/);
});

test("/method and the landing link both examples", () => {
  const src = (f) => readFileSync(new URL(`../src/components/op/${f}`, import.meta.url), "utf8");
  const method = src("screens/method-page.tsx");
  assert.match(method, /href="\/examples\/plan-rule-check"/);
  assert.match(method, /href="\/examples\/revocation-alert"/);
  assert.match(method, /href="\/examples\/part102-plan"/);
  const landing = src("landing/landing.tsx");
  assert.match(landing, /"\/examples\/plan-rule-check"/);
  assert.match(landing, /"\/examples\/revocation-alert"/);
  // Judge panel round 12, N9: the landing links the Part 102 plan too.
  assert.match(landing, /"\/examples\/part102-plan"/);
});

// Judge panel round 11, N1: a merge left "See a recorded move check.
// accuracy (D442)." on /method. No sentence on the page may start with a
// lowercase word after a full stop.
test("/method has no sentence that starts with a lowercase word", async () => {
  const { MethodPage } = await import("../src/components/op/screens/method-page.tsx");
  const t = text(html(MethodPage, {}));
  // Identifiers and commands may start lowercase: router-v4, git clone.
  const allowed = new Set(["router", "git"]);
  const broken = [...t.matchAll(/[a-z)]\. ([a-z][a-z]+)\b/g)].filter((m) => !allowed.has(m[1])).map((m) => t.slice(Math.max(0, m.index - 40), m.index + 30));
  assert.deepEqual(broken, []);
  assert.match(t, /Dual-sided precedent gate/);
  assert.doesNotMatch(t, /accuracy \(D442\)\. On three|move check\. accuracy/);
});

// Judge panel round 11, item 3; round 12, item 4; round 13, item 3: the D630
// actuator search (N348465), with the production Part 102 engine (usmca-v5)
// predicting each move and the D541 (usmca-v1) and D501 (general engine)
// predictions beside it, rebuilt by plans102-example in make reproduce.
const p102 = json("part102-plan-example.json");

test("the Part 102 example is N348465's development search: every motor move shifts to Canada, no other move does", () => {
  assert.equal(p102.ruling, "N348465");
  assert.equal(p102.split, "dev");
  assert.equal(p102.target, "Canada");
  assert.equal(p102.cbp_origin, "China");
  assert.equal(p102.ranking, "tier_then_rule_then_net");
  const all = [...p102.shown, ...p102.others];
  assert.equal(all.length, 12);
  for (const m of all) assert.equal(m.rule_check.status === "shifts", m.moves_motor, m.key);
  assert.equal(all.filter((m) => m.moves_motor).length, 6);
  assert.equal(p102.shown.length, 2);
  for (const m of p102.shown) assert.ok(m.moves_motor && m.verdict.confidence >= 0.92);
  const alone = all.find((m) => m.parts_moved.length === 1 && m.moves_motor);
  assert.equal(alone.verdict.confidence, 0.85);
  assert.equal(alone.shown, false);
  assert.doesNotMatch(p102.product, /country of origin/i, "the fixture names the product (D532)");
  // D631: the run is the production Part 102 engine's (usmca-v5, D630); it agrees with the rule on every move,
  // as usmca-v1 did in D541; the D501 general engine on 9.
  assert.match(p102.run, /-d630-litellm\.json$/);
  assert.match(p102.run_d541, /-d540-litellm\.json$/);
  assert.match(p102.run_d501, /-d501-litellm\.json$/);
  assert.equal(p102.engine, "usmca-v5");
  assert.equal(p102.moves, 12);
  assert.equal(p102.engine_agrees_with_rule, 12);
  assert.equal(p102.engine_agrees_with_rule_d541, 12);
  assert.equal(p102.engine_agrees_with_rule_d501, 9);
  for (const m of all) assert.equal(m.engine_origin_d541 === "Canada", m.moves_motor, m.key);
  for (const m of all) assert.equal(m.engine_origin === "Canada", m.moves_motor, m.key);
  assert.equal(all.filter((m) => !m.moves_motor && m.engine_origin_d501 === "Canada").length, 3);
});

test("the Part 102 page shows the moves, the shown plan, the draft's Part 102 question and 102.20 paragraph, and stays read only", () => {
  const m = html(Part102PlanExample, {});
  const t = text(m);
  assert.match(t, /Recorded example/);
  assert.match(t, /nothing here calls a model or saves anything/);
  assert.match(t, /The shown plans \(2\)/);
  assert.match(t, /Other moves that bring the motor to Canada \(4\)/);
  assert.match(t, /Moves without the motor \(6\)/);
  assert.equal((m.match(/data-move="shown"/g) ?? []).length, 2);
  assert.match(t, /The smallest move the rule supports\./);
  assert.match(t, /under the Part 102 rules of origin \(the general rules of 19 C\.F\.R\. § 102\.11, applied in order, with the tariff shifts of § 102\.20\)/);
  assert.match(t, /Tariff-shift check \(19 C\.F\.R\. § 102\.20\): applied to the component codes/);
  assert.match(t, /the rule gives Canada/);
  // The 3 moves the general engine sent to Canada in D502 stay visible as corrected.
  assert.equal((t.match(/The general engine predicted Canada here, though the rule says the move does not shift; the Part 102 engine does not\./g) ?? []).length, 3);
  assert.doesNotMatch(t, /The Part 102 engine and the rule disagree/);
  assert.match(t, /On 12 of 12 moves it agrees with the rule/);
  assert.match(t, /usmca-v1 agreed on 12 of 12 \(D541\) and the general engine on 9 \(D502\)/);
  // Round 13, item 3: the page names the run and engine of each prediction.
  assert.match(t, /The production Part 102 engine, usmca-v5, predicted each move \(D630/);
  assert.match(t, /Part 102 engine \(usmca-v5\): Canada\. usmca-v1 in the earlier run \(D541\): Canada\./);
  // Round 12, N6: the order key is not shown raw; round 12, N5: the note beside the letter is the Part 102 engine's.
  assert.doesNotMatch(t, /tier_then_rule_then_net/);
  assert.match(t, /Asked under the Part 102 rules \(19 C\.F\.R\. § 102\.11\) for marking because the prediction behind the requester's position was made under that regime/);
  assert.doesNotMatch(t, /made by Unde's general engine/);
  assert.match(t, /cleaned in D532, after the two earlier runs/);
  assert.doesNotMatch(t, /a The country of origin|of the country of origin of/i);
  assert.doesNotMatch(m, /<form|<button|cases\/new/);
});

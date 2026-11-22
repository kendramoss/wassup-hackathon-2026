import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D441: the 19 CFR 102.20 rule check on each plan move. A chip with the
// server's label: calm when the rule and the precedent check agree or the
// rule cannot decide, amber with a note when they disagree. Nothing when the
// plan carries no check (a substantial-transformation case without codes).
const { PlanRuleChip, PlanRuleNote, planRuleCaption } = await import("../src/components/op/plan-rule.tsx");

const shifts = { applies: "informational", country: "Thailand", status: "shifts", label: "Shifts under 102.20 (8543.70)", reason: "r", review: false, agrees_with_precedent: true };
const flagged = {
  applies: "informational",
  country: "China",
  status: "other_country",
  label: "102.20 gives China: finished there",
  origin: "China",
  reason: "r",
  review: true,
  review_note: "An earlier ruling supports this move, but the 102.20 rule gives China: worth a broker's look.",
  agrees_with_precedent: false,
};

test("a decided, agreeing check is a calm chip with the label", () => {
  const m = html(PlanRuleChip, { plan: { rule_check: shifts } });
  assert.match(text(m), /^Shifts under 102\.20 \(8543\.70\)$/);
  assert.match(m, /data-plan-rule="shifts"/);
  assert.doesNotMatch(m, /bg-ask-wash/);
  assert.equal(html(PlanRuleNote, { plan: { rule_check: shifts } }), "");
});

test("a disagreement is amber, with the note", () => {
  const m = html(PlanRuleChip, { plan: { rule_check: flagged } });
  assert.match(m, /bg-ask-wash/);
  assert.match(text(html(PlanRuleNote, { plan: { rule_check: flagged } })), /worth a broker's look/);
});

test("no check, nothing shown", () => {
  assert.equal(html(PlanRuleChip, { plan: {} }), "");
  assert.equal(planRuleCaption({ baseline: {}, plans: [{}], rejected: [] }), null);
});

test("the caption says when the check is for comparison only", () => {
  assert.match(planRuleCaption({ baseline: { rule_check: shifts }, plans: [], rejected: [] }), /for comparison: this case is decided under substantial transformation/);
  assert.doesNotMatch(planRuleCaption({ plans: [{ rule_check: { ...shifts, applies: "part102" } }] }), /for comparison/);
});

// D500: on a Part 102 case the caption says the check orders shown plans
// within each tier; the comparison copy says the same.
test("the Part 102 caption names the D500 order", async () => {
  const { copy } = await import("../src/lib/copy.ts");
  assert.match(planRuleCaption({ plans: [{ rule_check: { ...shifts, applies: "part102" } }] }), /within each tier the moves the rule supports come first/);
  assert.equal(copy.plan.tier.orderRule, "clear margin first, then the 102.20 check, then net");
  assert.match(copy.planRule.compareHow, /ranks last, flagged for a broker/);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D690: the RCEP preference check on a case's parts list. Rules on the
// server; the screen sends the parts as they are and shows yes / no / not
// sure with the parts that block it.
const { rcepBody, blankForm, madeIn, verdict, RcepResult, RcepCheck } = await import("../src/components/op/rcep-check.tsx");

const c = {
  id: "case_1",
  product: {
    name: "spice mix",
    hts: "0910.91.00",
    plants: [{ id: "my1", country: "Malaysia" }, { id: "vn1", country: "Vietnam" }],
    operations: [{ name: "grind", plant: "vn1" }, { name: "blend and pack", plant: "my1" }],
    components: [
      { name: "vanilla pods", made_in: "Madagascar", hts: "0905.10" },
      { name: "ground cinnamon", made_in: "Vietnam", hts: "0906.20" },
      { name: "label", made_in: "Malaysia" },
    ],
  },
};

test("made in: the plant of the last operation", () => {
  assert.equal(madeIn(c), "Malaysia");
  assert.equal(madeIn({ ...c, rule_check: { country: "Thailand" } }), "Thailand");
  assert.equal(madeIn({ product: { plants: [{ id: "a", country: "Japan" }], components: [] } }), "Japan");
  assert.equal(madeIn({ product: { plants: [{ id: "a", country: "Japan" }, { id: "b", country: "China" }], components: [] } }), "");
});

test("the request: unticked parts are unknown, values only when given", () => {
  const f = blankForm(c);
  f.fob = "18";
  f.parts[1] = { originating: true, value: "6" };
  const b = rcepBody(c, f);
  assert.equal(b.hs, "0910.91.00");
  assert.equal(b.produced_in, "Malaysia");
  assert.equal(b.fob, 18);
  assert.deepEqual(b.materials[0], { name: "vanilla pods", hs: "0905.10", country: "Madagascar", status: "unknown" });
  assert.deepEqual(b.materials[1], { name: "ground cinnamon", hs: "0906.20", country: "Vietnam", status: "originating", value: 6 });
  assert.equal(b.materials[2].hs, "");
  assert.equal("fob" in rcepBody(c, blankForm(c)), false);
});

test("the result: a plain verdict and the parts that block it", () => {
  assert.equal(verdict({ outcome: "yes" }).text, "Qualifies");
  assert.equal(verdict({ outcome: "no" }).tone, "fail");
  assert.equal(verdict({ outcome: "undetermined" }).text, "Not sure yet");
  const t = text(
    html(RcepResult, {
      res: {
        outcome: "no",
        failing: [{ index: 1, name: "ground cinnamon", hs: "0906.20", reason: "same chapter as the good (09), so no change" }],
        rule: { text: "CC or RVC40" },
        reasons: ["Rule for 0910.91: CC or RVC40."],
        note: "RCEP preferential origin from the facts given. Not marking origin, not a ruling, not a certificate of origin.",
      },
    }),
  );
  assert.match(t, /Does not qualify/);
  assert.match(t, /ground cinnamon · 0906\.20 · same chapter/);
  assert.match(t, /Not marking origin/);
});

// #729: RCEP is spelled out once, and a short "what you need" list sits above
// the form. #732: the parts list stacks at 390px with 44px tap targets.
const panel = html(RcepCheck, { c });
const panelText = text(panel);
const noCode = html(RcepCheck, { c: { ...c, product: { ...c.product, hts: "" } } });

test("RCEP is spelled out exactly once in the panel", () => {
  assert.equal(panelText.match(/Regional Comprehensive Economic Partnership/g)?.length, 1);
  assert.match(panelText, /Regional Comprehensive Economic Partnership \(RCEP\)/);
});

test("a short list of what you need sits above the form, with or without a code", () => {
  for (const m of [panel, noCode]) {
    const t = text(m);
    for (const need of [/HS code/, /Where it is made/i, /origin and value/i, /FOB.*optional|optional.*FOB/i]) assert.match(t, need);
    assert.ok(m.includes("data-rcep-needs"));
  }
  assert.ok(panel.indexOf("data-rcep-needs") < panel.indexOf("<input"), "needs list comes before the first field");
  assert.equal((panel.match(/data-rcep-needs/g) ?? []).length, 1);
});

test("the panel copy is plain: no statute or jargon in the visible summary", () => {
  const needs = panel.match(/<ul[^>]*data-rcep-needs[^>]*>.*?<\/ul>/s)[0];
  assert.doesNotMatch(text(needs), /Art\.|non-originating|tariff-shift|RVC|CTC/i);
});

test("parts are stacked rows, not a table that clips at 390px", () => {
  assert.doesNotMatch(panel, /<table/);
  assert.equal((panel.match(/data-rcep-part/g) ?? []).length, 3);
  assert.match(panel, /flex-wrap/);
});

test("each checkbox has a 44px (min-h-11 min-w-11) tap target and a visible label", () => {
  const labels = panel.match(/<label[^>]*>(?:(?!<\/label>).)*type="checkbox"(?:(?!<\/label>).)*<\/label>/gs) ?? [];
  assert.equal(labels.length, 3);
  for (const l of labels) {
    assert.match(l, /min-h-11/);
    assert.match(l, /min-w-11/);
    assert.match(text(l), /RCEP origin/);
  }
});

test("value fields are at least 44px tall on every row", () => {
  const inputs = panel.match(/<input[^>]*aria-label="[^"]*: value"[^>]*>/g) ?? [];
  assert.equal(inputs.length, 3);
  for (const i of inputs) assert.match(i, /h-11/);
});

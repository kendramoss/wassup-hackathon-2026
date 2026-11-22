import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D690: the RCEP preference check on a case's parts list. Rules on the
// server; the screen sends the parts as they are and shows yes / no / not
// sure with the parts that block it.
const { rcepBody, blankForm, madeIn, verdict, RcepResult } = await import("../src/components/op/rcep-check.tsx");

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

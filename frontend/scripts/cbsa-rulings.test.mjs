import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// D923: published CBSA rulings on a Canada case, display only.
const { CbsaRulings } = await import("../src/components/op/cbsa-rulings.tsx");

const rulings = [
  { id: "8000002867", date: "2022-09-21", kind: "origin", product: "Decorative Cushions", agreement: "CUSMA", holding: "Not originating under CUSMA", url: "https://ccp-pcc.cbsa-asfc.cloud-nuage.canada.ca/en/national-rulings/national-rulings-details/8000002867" },
  { id: "8000002155", date: "2022-03-29", kind: "origin", product: "Pillow", agreement: "CUSMA", holding: "Originating under CUSMA", url: "https://ccp-pcc.cbsa-asfc.cloud-nuage.canada.ca/en/national-rulings/national-rulings-details/8000002155" },
];

test("lists each ruling with its holding and a link to the CBSA page", () => {
  const m = html(CbsaRulings, { rulings });
  const t = text(m);
  assert.match(t, /CBSA rulings · 2 related/);
  assert.match(t, /Not originating under CUSMA\s*Decorative Cushions/);
  assert.match(m, /href="https:\/\/ccp-pcc\.cbsa-asfc\.cloud-nuage\.canada\.ca\/en\/national-rulings\/national-rulings-details\/8000002155"[^>]*target="_blank"/);
  assert.match(m, /<details[^>]*open/);
});

test("nothing when no ruling relates", () => {
  assert.equal(html(CbsaRulings, { rulings: [] }), "");
  assert.equal(html(CbsaRulings, {}), "");
});

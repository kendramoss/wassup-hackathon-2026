import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { csvNumber, csvText, preview } from "../src/lib/triage-csv.ts";

// #310: the preview must accept every alias the backend parser accepts.
test("preview accepts product name / product description aliases", () => {
  const rows = preview("Product Name,Product Description,Case Date\nWidget,Made in Thailand,2026-01-02");
  assert.deepEqual(rows, [{ name: "Widget", facts: "Made in Thailand", date: "2026-01-02" }]);
});

test("preview accepts the original aliases too", () => {
  const rows = preview("sku,description\nSKU-1,Facts here");
  assert.deepEqual(rows, [{ name: "SKU-1", facts: "Facts here", date: "" }]);
});

test("preview still rejects a CSV with no facts column", () => {
  assert.deepEqual(preview("name,date\nWidget,2026-01-02"), []);
});

// #299: untrusted text cells must not become spreadsheet formulas.
test("csvText neutralises formula prefixes", () => {
  for (const v of ["=1+1", "+cmd", "-SUM(A1)", "@import", " =5", "\t=HYPERLINK(...)"]) {
    assert.equal(csvText(v).startsWith("'"), true, `expected apostrophe for ${JSON.stringify(v)}`);
  }
});

test("csvText leaves ordinary text alone but quotes commas/quotes/newlines", () => {
  assert.equal(csvText("Thailand"), "Thailand");
  assert.equal(csvText("a,b"), '"a,b"');
  assert.equal(csvText('say "hi"'), '"say ""hi"""');
  assert.equal(csvText("two\nlines"), '"two\nlines"');
  assert.equal(csvText(null), "");
});

test("numeric columns stay numeric; nonfinite numbers are emitted as safe text", () => {
  assert.equal(csvNumber(0.9), "0.9");
  assert.equal(csvNumber(0), "0");
  for (const v of [NaN, Infinity, -Infinity]) {
    const cell = csvNumber(v);
    assert.equal(/^[=+\-@]/.test(cell), false, `${cell} must not be a formula`);
  }
});

// Issue #492 / D265: the CSV's first confidence column is the model's own
// number. Its header must say so; an unlabelled "confidence" invited readers
// to treat the stated value as the calibrated one shown on the Result screen.
test("triage CSV labels the model's own confidence column", () => {
  const src = readFileSync(new URL("../src/components/op/screens/triage-page.tsx", import.meta.url), "utf8");
  assert.match(src, /\[\s*"confidence",\s*"stated_confidence"\s*\]/);
  assert.doesNotMatch(src, /\[\s*"name",\s*"case_id",\s*"predicted_origin",\s*"confidence"/);
});

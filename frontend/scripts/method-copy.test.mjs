import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { ARMS, BANDS } from "../src/lib/method.ts";

// Copy sweep 3 (#676, rule 16): /method is written for a first-time visitor.
// Decision numbers, p-values, run ids and protocol detail moved into Details
// folds, but every honesty line must stay visible in its shorter form. These
// pins fail if a later edit drops one or puts jargon back in the open.
const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8").replace(/\s+/g, " ");
const page = read("../src/components/op/screens/method-page.tsx");
const stability = read("../src/components/op/method-stability.tsx");

// The page source with every <Details>…</Details> block removed.
const outside = (src, tag = "Details") =>
  src
    .split(new RegExp(`<${tag}[ >]`))
    .map((s, i) => (i === 0 ? s : s.slice(s.indexOf(`</${tag}>`) + tag.length + 3)))
    .join(" ");
const visible = outside(page);
const jsx = visible.slice(visible.indexOf("export function MethodPage"));

test("the run labels stay on the page: official run, not the official run, mixed runs", () => {
  assert.match(page, /<Chip tone="ok">official run<\/Chip>/);
  assert.match(page, /<Chip>not the official run<\/Chip>/);
  assert.match(page, /<Chip>mixed runs<\/Chip>/);
  assert.match(page, /<Chip>not a blind test<\/Chip>/);
  assert.match(stability, /<Chip>mixed runs<\/Chip>/);
  // The gateway is named in the chip tooltip and in Details, not in the open.
  assert.match(page, /OpenAI-compatible gateway, not Vertex AI/);
});

test("the honesty lines stay outside the Details folds", () => {
  for (const s of [
    "Unde is not a trained model",
    "It was worse, so it was dropped",
    "It never changes the answer",
    "Only CBP's ruling binds",
    "Not a blind test: the search was changed after the team saw this change",
    "The 18 later pairs are the clean test",
    "The money is illustrative unless you enter your own costs",
    "There are too few wrong verdicts to tell",
    "One reader, not a customs expert",
    "The first auto-accept bar also looked at test results",
    "The Part 102 engine in use has no clean test",
    "later router gains are not a clean test",
    "The model-sweep rule was written while two of its runs were still going",
    "Earlier test rulings can be precedents for later ones",
    "Search word counts use the whole corpus",
    "Gemini may have seen some rulings in training",
    "None of these was scored on the unseen test set",
    "not a fair test; failed its rule",
  ])
    assert.ok(visible.includes(s) || ARMS.some((g) => g.rows.some((r) => r.note === s)), s);
  assert.match(outside(stability, "Fold"), /Two repeats per test set is a small sample\./);
});

test("the visible page uses plain names, not internal ids", () => {
  assert.doesNotMatch(jsx, /held-out|<D id=|<Refs|McNemar|usmca-v\d|router-v\d|engine-v\d|rules-v\d|st-checklist|p = |gateway/i);
  for (const b of BANDS) assert.doesNotMatch(`${b.label} ${b.note}`, /held-out|gateway/i, b.label);
  for (const g of ARMS)
    for (const r of g.rows) assert.doesNotMatch(`${r.label} ${r.note}`, /held-out|gateway|-v\d|p = /i, r.label);
});

test("each question opens with a short plain lead", () => {
  const leads = [...page.matchAll(/<Section id="\w+" lead="([^"]+)">/g)].map((m) => m[1]);
  assert.equal(leads.length, 8);
  for (const l of leads) assert.ok(l.split(" ").length <= 20, `lead over 20 words: ${l}`);
});

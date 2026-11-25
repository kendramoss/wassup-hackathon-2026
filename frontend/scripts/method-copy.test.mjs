import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";
import { kobalt, methodExperiments, methodStages } from "../src/lib/method-inspector.ts";
const { MethodPage } = await import("../src/components/op/screens/method-page.tsx");

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8").replace(/\s+/g, " ");
const markup = html(MethodPage, {});
const page = text(markup);
const architecture = read("../src/components/op/method-architecture.tsx");

test("Method uses a connected stage inspector and comparison cards without text dumps", () => {
  assert.doesNotMatch(markup, /<(?:ul|ol)\b/);
  // Only two short disclosures on the page: the leakage limits and the figures' sources.
  assert.equal((markup.match(/<details\b/g) || []).length, 2);
  assert.match(markup, /<details class="mi-limits"><summary>What this does not show<\/summary><p[^>]*>[^<]*pretraining exposure is unknown[^<]*<\/p><\/details>/);
  assert.equal((architecture.match(/<details\b/g) || []).length, 0);
  assert.match(markup, /role="tablist"[^>]*aria-orientation="vertical"/);
  assert.equal((markup.match(/role="tab"/g) || []).length, 6);
  assert.match(markup, /role="tab"[^>]*aria-controls="architecture-detail"/);
  assert.match(markup, /role="tabpanel"[^>]*id="architecture-detail"|id="architecture-detail"[^>]*role="tabpanel"/);
  assert.equal(methodStages.length, 6);
  assert.ok(methodStages.every((stage) => stage.input.length && stage.output.length && stage.guard && stage.trace.length));
  assert.equal(methodExperiments.length, 3);
  assert.equal((markup.match(/<button[^>]*aria-pressed="true"[^>]*>Failed approach<\/button>/g) || []).length, 3);
  assert.equal((markup.match(/<button[^>]*aria-pressed="false"[^>]*>Production fix<\/button>/g) || []).length, 3);
});

test("Method scopes its temporal claim and keeps the 2022 and 2026 dates distinct", () => {
  // #685: plain wording. Still scoped to the live (production) model, and
  // the search statistics still say they include later rulings.
  assert.match(page, /The live service uses Gemini without fine-tuning\./);
  // #685 review: "retrieves", not "reads" (pretraining exposure is unknown).
  assert.match(page, /Each answer retrieves only CBP rulings dated before the case\./);
  assert.doesNotMatch(page, /Each answer reads only/);
  // #685: the pretraining caveat is visible next to the lead, outside the fold.
  const outsideFolds = markup.replace(/<details\b[\s\S]*?<\/details>/g, "");
  assert.match(text(outsideFolds), /Gemini may have seen some rulings in pretraining; we can’t measure how many\./);
  assert.match(page, /Gemini’s pretraining exposure is unknown/);
  assert.match(page, /The ruling search \(BM25\) weighs words by counts taken from every ruling, later ones included\./);
  assert.doesNotMatch(page, /zero training leakage/i);
  assert.match(page, /Illustrated 2022 cutoff/);
  assert.ok(page.includes(`Same-day rulings are blocked. ${kobalt.supporting.id} and ${kobalt.backtestRuling.id} are later. The saved demo uses ${kobalt.cutoff.label}.`));
  assert.ok(kobalt.setupRuling.date < kobalt.supporting.date);
  assert.ok(kobalt.supporting.date < kobalt.cutoff.date);
  assert.ok(kobalt.cutoff.date < kobalt.backtestRuling.date);
});

test("The archived and current confidence gates stay distinct", () => {
  assert.match(methodStages[2].guard, /saved run used stated ≥0\.92/);
  assert.match(methodStages[2].guard, /in-scope cases at calibrated ≥0\.90/);
  assert.match(methodStages[4].guard, /eligible support and scores at least 0\.92/);
  assert.match(page, /Only CBP’s ruling binds/);
  assert.match(page, /No qualifying counter-ruling recorded/);
  assert.match(page, /It was not a blind test/);
});

test("The Ask inspector shows the live run and the local repeat apart", () => {
  assert.match(architecture, /stage\.id === "ask".*Run 1, live, passed the tool-use checks on 9 of 12 products/);
  assert.match(architecture, /Run 2, on a local copy of the server, passed 12 of 12/);
  assert.match(architecture, /ADK evaluation details/);
});

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const { MarketEvidence } = await import("../src/components/op/landing/market-evidence.tsx");
const markup = html(MarketEvidence, {});
const page = text(markup);
const { held_out: h } = JSON.parse(read("../../eval/rcep/japan-summary.json"));
const o = h.published_positives.outcomes;
const crit = h.published_positives.deciding_criterion_agreement;

test("the rendered RCEP chart and limits match the committed held-out summary", () => {
  assert.ok(page.includes(`on ${o.yes.total} unseen Japan Customs rulings`));
  assert.ok(page.includes(`${o.yes.count} Yes ${o.undetermined.count} Undetermined`));
  assert.ok(markup.includes(`aria-label="${o.yes.count} yes, ${o.undetermined.count} undetermined, out of ${o.yes.total}"`));
  assert.ok(page.includes(`matched in ${crit.count} of ${crit.total} yes answers`));
  assert.equal(o.no.count + o.error.count, 0);
  assert.ok(page.includes("It is not an accuracy test."));
  assert.ok(page.includes("No rulings that deny the preference were tested, so refusals are unmeasured."));
  assert.ok(page.includes("Customs specialist review is pending."));
  assert.ok(page.includes("The calculator's answer on whether the product gets the trade-deal rate."));
});

test("the limits sit in a closed disclosure and the evidence link resolves to the Accuracy block", () => {
  const details = markup.match(/<details[^>]*>[\s\S]*?<\/details>/g) ?? [];
  assert.equal(details.length, 1);
  assert.ok(!/<details[^>]*open/.test(details[0]));
  assert.ok(text(details[0]).startsWith("Limits"));
  assert.ok(markup.includes('href="/accuracy#market-evidence"'));
});

test("each market names what was shown and invents no Canada or Japan score", () => {
  const items = [...markup.matchAll(/<li[^>]*><b>([^<]*)<\/b><span[^>]*>([^<]*)<\/span><span[^>]*>([^<]*)<\/span><\/li>/g)].map((m) => m.slice(1));
  assert.deepEqual(items.map((i) => i[0]), ["U.S. origin", "RCEP tariff", "Canada marking", "Japan origin", "Other markets"]);
  const by = Object.fromEntries(items.map((i) => [i[0], i]));
  assert.notEqual(by["U.S. origin"][1], by["RCEP tariff"][1]);
  assert.equal(by["U.S. origin"][2], "Single-origin U.S. substantial-transformation rulings only.");
  assert.deepEqual(by["Canada marking"].slice(1), ["Rules only", "No score."]);
  assert.deepEqual(by["Japan origin"].slice(1), ["Not available yet", "Turns on only when the Japan model is configured. Not measured."]);
  assert.ok(!/\d/.test([...by["Canada marking"], ...by["Japan origin"]].join(" ")));
  assert.ok(!page.includes("Supported check"));
});

test("the status list has an accessible name", () => {
  assert.match(markup, /<article[^>]*aria-labelledby="marketstatush"[^>]*>\s*<h3 id="marketstatush"[^>]*>Market status<\/h3>/);
});

test("the landing page renders the market block", () => {
  assert.ok(read("../src/components/op/landing/landing.tsx").includes("<MarketEvidence />"));
});

test("the evidence link is at least 44px tall", () => {
  assert.ok(/<a href="\/accuracy#market-evidence" class="judge-source mkt-cta">/.test(markup));
  const css = read("../src/components/op/landing/market-evidence.css");
  const rule = css.match(/\.lp \.judge-source\.mkt-cta \{([^}]*)\}/);
  assert.ok(rule);
  assert.match(rule[1], /min-height:\s*44px/);
  assert.match(rule[1], /display:\s*inline-flex/);
});

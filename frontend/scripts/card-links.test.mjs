// Cards on /cases must not put a link inside a link: browsers re-parent the
// inner <a>, React logs a hydration warning, and the card's own layout breaks.
// Renders each card component with react-dom/server and walks the markup.
import assert from "node:assert/strict";
import { test } from "node:test";
import { html } from "./render-support.mjs";

const { LoadDemoCard, DemoCard } = await import("../src/components/op/screens/cases-home.tsx");

const INTERACTIVE = new Set(["a", "button"]);
const VOID = new Set(["area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "source", "track", "wbr"]);

// Returns "outer > inner" for every interactive element inside another one.
function nested(markup) {
  const open = [];
  const found = [];
  for (const [, close, name] of markup.matchAll(/<(\/?)([a-zA-Z][\w-]*)\b[^>]*?>/g)) {
    const tag = name.toLowerCase();
    if (VOID.has(tag)) continue;
    if (close) {
      const i = open.lastIndexOf(tag);
      if (i >= 0) open.length = i;
      continue;
    }
    const outer = open.find((t) => INTERACTIVE.has(t));
    if (outer && INTERACTIVE.has(tag)) found.push(`${outer} > ${tag}`);
    open.push(tag);
  }
  return found;
}

test("the nested-link check catches a link inside a link", () => {
  assert.deepEqual(nested('<a href="/x"><span><a href="/y">y</a></span></a>'), ["a > a"]);
  assert.deepEqual(nested('<article><a href="/x">x</a><a href="/y">y</a></article>'), []);
});

const recorded = {
  run: "dev run",
  file: "eval/results/demo.json",
  engine_origin: "China",
  stated_confidence: 0.95,
  calibrated_confidence: 0.9,
  agrees_with_cbp: true,
  best_plan: "Move the motor to Thailand",
  best_plan_words: "Move the motor to Thailand",
  plan_check: 0.96,
  plan_ruling: "N000001",
  best_plan_includes: ["Motor"],
};
const demo = { id: "demo-1", title: "Heat gun", description: "Assembled in Thailand.", cbp_held: "China (N328954)", has_plan: true };

test("a demo card with a committed-run link has no link inside a link", () => {
  const m = html(LoadDemoCard, { d: { ...demo, recorded } });
  assert.match(m, /href="\/cases\/new\?demo=demo-1"/);
  assert.match(m, /href="https:\/\/github\.com\/[^"]+eval\/results\/demo\.json"/);
  assert.deepEqual(nested(m), []);
});

test("a demo card without a recorded run has no nested interactive elements", () => {
  assert.deepEqual(nested(html(LoadDemoCard, { d: demo })), []);
});

test("a pinned demo card has no nested interactive elements", () => {
  const c = { id: "case_1", display_name: "Heat gun", step: "origin", predicted_origin: "China", confidence: 0.9, triage_route: "auto_accept", pinned: true };
  assert.deepEqual(nested(html(DemoCard, { c })), []);
});

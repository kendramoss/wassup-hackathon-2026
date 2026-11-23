import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// #685, last residual: the landing page (outside the market-evidence section),
// the try-it flow and the recorded /examples pages. Plain labels on the
// visible page, 44 px tap targets and no text under 12 px on phones, with the
// evidence scope left as it was.
const read = (p) => readFileSync(new URL(p, import.meta.url), "utf8");
const landing = read("../src/components/op/landing/landing.tsx");
const landingCss = read("../src/components/op/landing/landing.css");
const globals = read("../src/app/globals.css");
const copy = read("../src/lib/copy.ts");

test("the landing sheets name what each step does, not internal parts", () => {
  for (const jargon of ["Router · Gemini", "Origin engine · Gemini", "Planner + verifier agent"]) {
    assert.ok(!landing.includes(jargon), jargon);
  }
  assert.match(landing, /Origin rules picked by Gemini 3\.8 Flash/);
  assert.match(landing, /Origin read by Gemini 4 Argon/);
  assert.match(landing, /Each move checked for and against/);
});

test("the landing's model names match the server defaults, which deploy does not override", () => {
  const config = read("../../backend/internal/config/config.go");
  assert.match(config, /RouterModel:\s+e\.str\("ROUTER_MODEL", "gemini-3\.8-flash"\)/);
  assert.match(config, /EngineModel:\s+e\.str\("ENGINE_MODEL", "gemini-4-argon"\)/);
  const deploy = read("../../.github/workflows/deploy.yml");
  assert.ok(!/\bROUTER_MODEL=/.test(deploy), "deploy.yml sets ROUTER_MODEL");
  assert.ok(!/\bENGINE_MODEL=/.test(deploy), "deploy.yml sets ENGINE_MODEL");
});

test("phones hide the wire caption with the wire, and keep the not-blind note", () => {
  assert.match(landing, /className="wirenote">\{L\.fig2Foot\}/);
  assert.match(landing, /<span>\{L\.fig2BacktestNote\}<\/span>/);
  assert.match(landingCss, /\.lp \.f2 \.foot \.wirenote \{ display: none; \}/);
  assert.match(landingCss, /\.lp svg\.path \{ display: none; \}/);
});

test("landing evidence scope is unchanged: 377 rulings, 288 of 300 auto-accepted right", () => {
  assert.match(landing, /auto-accepted, right <b>288<\/b>/);
  assert.match(landing, /wrong <b>12<\/b>/);
  assert.match(landing, /to review, right <b>52<\/b>/);
  assert.match(landing, /wrong <b>25<\/b>/);
  assert.match(copy, /fig3Title: "Tested on 377 CBP rulings, scored once"/);
});

test("landing controls are 44 px tall on phones", () => {
  const phone = landingCss.slice(landingCss.indexOf("---- phones (#685)"));
  assert.match(phone, /\.lp \.ctas a, \.lp \.hero \.also a, \.lp \.pcfoot a, \.lp \.testfoot \.endcta > a \{ min-height: 44px;/);
  assert.match(phone, /\.lp details > summary \{ display: list-item; min-height: 44px;/);
  assert.match(phone, /\.lp \.eyebrow, \.lp \.stamp \.rul \{ font-size: 12px; \}/);
  assert.match(read("../src/components/op/landing/judge-tabs.css"), /\.lp \.judgePersonaActions a,\n\s+\.lp \.judgePersonaPreviewFoot > a,\n\s+\.lp \.judgePersonaTerminalTop button \{\n\s+min-height: 44px;/);
  assert.match(read("../src/components/op/landing/judge-proof.css"), /\.lp \.judge-impact \.judge-source \{\n\s+min-height: 44px;/);
});

test("the try-it flow and the examples opt in to phone tap targets", () => {
  assert.match(globals, /@media \(max-width: 640px\) \{\n\s+\.touch44 :is\(button, summary, select, a, \[role="button"\], \[role="tab"\]\)/);
  // Buttons inside a sentence keep the line height and get a 44 px ring.
  assert.match(globals, /:is\(p, dd\) button\):not\(\.sr-only\)::after \{[^}]*height: 44px;/);
  assert.match(globals, /min-height: 44px;/);
  const pages = [
    "cases/new",
    ...["product", "origin", "plan", "scenarios", "compare", "draft"].map((s) => `cases/[id]/${s}`),
    ...["rule-check", "plan-rule-check", "revocation-alert", "part102-plan"].map((s) => `examples/${s}`),
  ];
  for (const p of pages) {
    assert.match(read(`../src/app/${p}/page.tsx`), /<div className="touch44 contents">/, p);
  }
  // Out-of-scope pages are untouched.
  for (const p of ["cases", "accuracy", "method", "developers"]) {
    assert.ok(!read(`../src/app/${p}/page.tsx`).includes("touch44"), p);
  }
});

test("the compare table caption is short; the full order sits in its fold", () => {
  const view = read("../src/components/op/screens/compare-view.tsx");
  assert.match(view, /Ranked, best plan first · how it works is below the table/);
  assert.match(view, /After them come countries whose best move is below the bar, then those with no supported plan\./);
  // The Part 102 addition stays short too; the fold explains the tiers.
  assert.match(copy, /compareCaption: "Part 102: moves the 102\.20 rule supports rank first"/);
});

test("examples explain the two checks and say whose confidence it is", () => {
  assert.match(copy, /movesNote: "Precedent check: an AI agent reads earlier CBP rulings\. Rule check: code applies the tariff-shift rule/);
  assert.match(copy, /For origin marking \(19 CFR Part 102\) the tariff-shift rule is the test/);
  const rev = read("../src/components/op/screens/revocation-example.tsx");
  assert.ok(!rev.includes("} stated)"));
  assert.match(rev, /model&apos;s own confidence \{ex\.confidence\.toFixed\(2\)\}/);
});

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
    ...["rule-check", "plan-rule-check", "revocation-alert", "part102-plan", "blind-plan"].map((s) => `examples/${s}`),
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

// #685, final pass: the site-wide header on phones, the shared label sizes,
// and the jargon left on /method, /developers, /accuracy and /cases.
test("phones: header controls are 44 px and shared labels are 12 px, below 640 px only", () => {
  const chrome = read("../src/components/op/chrome.tsx");
  const signIn = read("../src/components/op/sign-in.tsx");
  // Nav links, the logo and "+ New" keep their desktop heights and add 44 px below sm.
  assert.match(chrome, /"flex min-h-10 items-center rounded-\[2px\] px-3 transition-colors max-sm:min-h-11 /);
  assert.match(chrome, /tracking-\[-0\.03em\] max-sm:min-h-11"/);
  assert.match(chrome, /className="min-h-9 py-1\.5 max-sm:min-h-11"/);
  assert.match(signIn, /size-10 shrink-0 max-sm:size-11/);
  assert.match(signIn, /grid size-10 place-items-center overflow-hidden max-sm:size-11/);
  // One shared rule for .eyebrow and the 10-11.5 px label classes.
  const block = globals.slice(globals.indexOf("#685, site-wide"));
  assert.match(block, /@media \(max-width: 639px\) \{/);
  assert.match(block, /:where\(\.eyebrow, \[class\*="text-\[10px\]"\], \[class\*="text-\[10\.5px\]"\], \[class\*="text-\[11px\]"\], \[class\*="text-\[11\.5px\]"\]\)/);
  assert.match(block, /font-size: 12px;/);
  // The base .eyebrow stays 11 px for wide screens.
  assert.match(globals, /\.eyebrow \{\n\s+font-size: 11px;/);
  const mi = read("../src/app/method/method-inspector.css");
  assert.match(mi.slice(mi.indexOf("#685")), /@media \(max-width: 639px\) \{[^}]*\.mi-node-label[^}]*font-size: 12px;/);
});

test("the jargon on /method, /developers, /accuracy and /cases is plain on the visible page", () => {
  const method = read("../src/components/op/screens/method-page.tsx");
  const stages = read("../src/lib/method-inspector.ts");
  const portal = read("../src/app/developers/portal.tsx");
  const examples = read("../src/app/developers/examples.ts");
  const accuracy = read("../src/components/op/screens/accuracy-page.tsx");
  for (const [src, gone] of [
    [method, "Zero production fine-tuning"],
    [method, "BM25 word counts span the full corpus"],
    [stages, "Gemini 4 Argon + BM25"],
    [stages, "ADK what-if"],
    [stages, '"Go search"'],
    [portal, "ERP"],
    [portal, '"ADK for Go"'],
    [examples, '"ADK for Go"'],
    [accuracy, "substantial-transformation rulings only"],
    [accuracy, '"Auto-accepted accuracy"'],
    [copy, "Runs live, with a plan search"],
  ])
    assert.ok(!src.includes(gone), gone);
  // MCP keeps its name with a short gloss; ADK is spelled out once.
  assert.match(portal, /label: "MCP server", subtext: "Tools your AI agent can call"/);
  assert.match(examples, /Google’s Agent Development Kit \(ADK\)/);
  // Schema field names moved behind a tooltip, not removed.
  assert.match(read("../src/components/op/method-architecture.tsx"), /<Term explain=\{`API field: \$\{field\}`\}>\{name\}<\/Term>/);
  assert.match(stages, /\["Rulings to leave out", "optional list", "exclude_rulings\?: ID\[\]"\]/);
});

test("phones: in-page controls left under 44 px get 44 px below 640 px only", () => {
  const src = (p) => read(`../src/${p}`);
  assert.match(src("app/developers/page.tsx"), /inline-flex min-h-10 max-sm:min-h-11 justify-self-end/);
  assert.match(src("app/developers/portal.tsx"), /const field = "min-h-10 max-sm:min-h-11 /);
  assert.match(src("app/developers/portal.tsx"), /block min-h-10 max-sm:min-h-11 w-full border-l-2/);
  assert.match(src("app/developers/portal.tsx"), /\$\{docLink\} max-sm:inline-flex max-sm:min-h-11 max-sm:items-center`\} href=\{`\$\{repo\}\/blob\/main\/docs\/api\/SECURITY\.md`\}/);
  const accuracy = src("components/op/screens/accuracy-page.tsx");
  assert.match(accuracy, /<ButtonLink href="#misses" onClick=\{openFailures\} className="max-sm:min-h-11">/);
  assert.match(accuracy, /<ButtonLink href="\/method" variant="ghost" className="max-sm:min-h-11">/);
  assert.match(src("components/op/screens/cases-home.tsx"), /<ButtonLink href="\/triage" variant="ghost" className="max-sm:min-h-11">/);
  const triage = src("components/op/screens/triage-page.tsx");
  assert.equal((triage.match(/max-sm:min-h-11/g) || []).length, 4);
  const mi = src("app/method/method-inspector.css");
  assert.match(mi.slice(mi.indexOf("#685")), /\.mi-evidence-pair a \{ min-height: 44px; \}/);
});

test("the landing's 'What is new' line no longer repeats the subtitle", () => {
  const sub = copy.match(/sub: "([^"]+)"/)[1];
  const novel = copy.match(/novel: \{\s+lead: "What is new:",\s+text: "([^"]+)"/)[1];
  assert.match(sub, /which production moves would change it/);
  assert.equal(novel, "Unde shows only the moves an earlier CBP ruling backs.");
  assert.ok(!/change the origin|production moves/.test(novel));
});

test("review fixes: live-run labels are true, method terms are 24 px targets, fold controls are 44 px on phones", () => {
  assert.match(copy, /unseeded: "Live run: starts when you click Find origin"/);
  assert.match(copy, /planRuns: "Live run, then you can search for moves"/);
  assert.ok(!/Runs when opened/.test(copy));
  const mi = read("../src/app/method/method-inspector.css");
  assert.match(mi.slice(mi.indexOf("#685")), /\n\.mi-schema dt > \[tabindex\] \{ display: inline-block; min-height: 24px; line-height: 24px; \}/);
  assert.match(read("../src/lib/method-inspector.ts"), /label: "Rule-based search"/);
  const fc = read("../src/components/op/failure-cases.tsx");
  assert.match(fc, /max-sm:inline-flex max-sm:min-h-11 max-sm:items-center">\s+How the test set is built/);
  assert.match(fc, /hover:underline max-sm:min-h-11" aria-expanded=\{all\}>/);
  assert.match(read("../src/components/op/screens/accuracy-page.tsx"), /hover:underline max-sm:min-h-11">\s+\{all \? "Show the main numbers"/);
  assert.match(read("../src/components/op/screens/cases-home.tsx"), /max-sm:inline-flex max-sm:min-h-11 max-sm:items-center"\s+>\s+\{T\.recordLink\}/);
});

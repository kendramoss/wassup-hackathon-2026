import "./render-support.mjs";
import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { createElement as h } from "react";
import { json, stubNavigation } from "./lib/hook-runtime.mjs";
import { deepMount, textOf } from "./lib/deep-mount.mjs";

// #278 / #282: the real screens (CaseShell + useCase + the step) are mounted,
// and each placement is read from what they render: the right note, once, in
// the right place, with the controls and pinned flow around it intact.
globalThis.window = new EventTarget();
globalThis.document = new EventTarget();
stubNavigation({ id: "c1", search: "target=Vietnam" });
const live = [];
afterEach(() => live.splice(0).forEach((a) => a.unmount()));

const { copy } = await import("../src/lib/copy.ts");
const OriginStep = (await import("../src/components/op/screens/origin-step.tsx")).OriginStep;
const PlanStep = (await import("../src/components/op/screens/plan-step.tsx")).PlanStep;
const CompareView = (await import("../src/components/op/screens/compare-view.tsx")).CompareView;
const ScenariosView = (await import("../src/components/op/screens/scenarios-view.tsx")).ScenariosView;

const J = copy.jurisdiction;
const R = copy.review;
const product = {
  name: "W",
  plants: [{ id: "a", name: "A", country: "China" }, { id: "b", name: "B", country: "Vietnam" }, { id: "c", name: "C", country: "India" }],
  components: [{ name: "x", made_in: "China" }],
  operations: [],
};
const move = (extra = {}) => ({
  key: "m1", description: "Make the motor in Vietnam", shown: true, net_per_year: 1000, duty_effect_per_year: 1500, added_cost_per_year: 500,
  prediction: { origin_country: "Vietnam", confidence: 0.95 }, verdict: { confidence: 0.96, agreement: "3/3" }, ...extra,
});
const planResult = (extra = {}) => ({ target: "Vietnam", plans: [move()], rejected: [], stats: { moves_searched: 3, engine_calls: 4, wall_seconds: 12 }, ...extra });
const caseJson = (extra = {}) => ({
  id: "c1", facts: "f", step: "origin", date: "2026-01-01", display_name: "Widget",
  prediction: { origin_country: "China", confidence: 0.97 }, triage: { route: "auto", reason: "confidence 0.97 >= 0.92 and in scope" },
  product, ...extra,
});

async function screen(Step, c) {
  globalThis.fetch = async (url, init = {}) => {
    const u = String(url);
    const m = init.method ?? "GET";
    // The case object itself is handed over (not re-serialised), so a test can
    // pass a Proxy that fails if the screen reads a field it should not.
    if (/\/cases\/c1$/.test(u) && m === "GET") return { ok: true, status: 200, headers: new Headers(), json: async () => c };
    if (u.endsWith("/plan/estimate")) return json({});
    if (u.includes("/demos")) return json({ demos: [] });
    if (u.endsWith("/api/session")) return json({ email: null });
    return json({});
  };
  const el = Step();
  const app = deepMount(h(el.type, el.props));
  live.push(app);
  app.refresh();
  await app.flush();
  return app;
}
const notes = (app) => app.find((e) => e.props?.["data-review-note"]);
const noteText = (n) => textOf(n.props.children);
const all = (app) => app.all().map((e) => textOf(e.props?.children)).join(" ");
// How many paragraphs on the page are exactly this line.
const count = (app, line) => app.find((e) => e.type === "p" && e.props.children === line).length;
const contains = (node, pred) => {
  let found = false;
  const walk = (n) => {
    if (!n || typeof n !== "object" || found) return;
    if (Array.isArray(n)) return n.forEach(walk);
    if (pred(n)) found = true;
    walk(n.props?.children);
  };
  walk(node);
  return found;
};

// ---- origin ---------------------------------------------------------------

test("origin: a high-confidence auto result still gets the local broker note and the scope line, once, beside the verdict", async () => {
  const app = await screen(OriginStep, caseJson());
  const n = notes(app);
  assert.equal(n.length, 1);
  assert.equal(n[0].props["data-review-note"], "origin");
  assert.ok(noteText(n[0]).includes(R.origin));
  assert.ok(noteText(n[0]).includes(J.originScope));
  assert.doesNotMatch(noteText(n[0]), /Marking origin under the Part 102|does not decide/);
  assert.match(all(app), /Likely origin/);
  // It sits in the verdict section, not in the footer or the app chrome.
  const band = app.find((e) => e.props?.["aria-label"] === "Likely origin")[0];
  assert.ok(contains(band.props.children, (e) => e.type?.name === "ReviewNote"), "inside the verdict band");
});

test("origin: a needs-review result carries the same single note", async () => {
  const app = await screen(OriginStep, caseJson({ prediction: { origin_country: "China", confidence: 0.6 }, triage: { route: "needs_review", reason: "confidence 0.60 is below the bar" } }));
  assert.equal(notes(app).length, 1);
  assert.equal(count(app, J.originScope), 1);
});

test("origin: the out-of-scope reviewer line shows only when triage says out of scope", async () => {
  const oos = await screen(OriginStep, caseJson({ triage: { route: "needs_review", reason: "out of scope: textile goods follow 19 CFR 102.21" } }));
  assert.ok(noteText(notes(oos)[0]).includes(J.outOfScope));
  assert.equal(count(oos, J.outOfScope), 1);
  const ok = await screen(OriginStep, caseJson());
  assert.ok(!noteText(notes(ok)[0]).includes(J.outOfScope));
});

test("origin: the Part 102 marking label is stated beside a Part 102 (USMCA label) result", async () => {
  const c = caseJson({ prompt_versions: { engine: "usmca-v1" }, triage: { route: "needs_review", reason: "usmca rules: a new regime without a held-out result yet" } });
  const app = await screen(OriginStep, c);
  assert.ok(noteText(notes(app)[0]).includes(J.part102));
  assert.equal(count(app, J.part102), 1);
  const other = await screen(OriginStep, caseJson({ prompt_versions: { engine: "textile-v1" }, triage: { route: "needs_review", reason: "textile rules" } }));
  assert.ok(!noteText(notes(other)[0]).includes(J.part102));
});

test("origin: existing controls and the next-step links are untouched", async () => {
  const app = await screen(OriginStep, caseJson({ clarify: { questions: ["Where is it assembled?"] } }));
  assert.ok(app.btn("re-run"), "answers re-run button");
  assert.match(all(app), /Plan a move/);
  assert.match(all(app), /Draft a ruling request/);
});

// ---- plan -----------------------------------------------------------------

test("plan (legacy result): suggestion note, bounded best-of wording and the plan scope line, once, with the search controls intact", async () => {
  const app = await screen(PlanStep, caseJson({ step: "plan", plan: planResult() }));
  const n = notes(app);
  assert.equal(n.length, 1);
  assert.equal(n[0].props["data-review-note"], "plan");
  const t = noteText(n[0]);
  for (const s of [R.plan, R.bestBounded, J.planScope]) assert.ok(t.includes(s), s);
  assert.match(t, /^Best of the moves searched|Best of the moves searched/);
  assert.doesNotMatch(t, /optimal|cheapest|guarantee/i);
  assert.match(all(app), /Moves searched/, "the moves table is intact");
  assert.match(all(app), /Net per year · illustrative/);
  assert.match(all(app), /Money · illustrative/);
});

test("plan (operations-like result): the same wording, and no future DTO field is read", async () => {
  const forbidden = new Set(["not_modelled", "notModelled", "operations_mode", "modelled", "objective"]);
  const result = new Proxy(planResult({ not_modelled: ["freight"], operations_mode: true, objective: { kind: "modelled" } }), {
    get(target, key, recv) {
      if (forbidden.has(key)) throw new Error(`the note read a future DTO field: ${String(key)}`);
      return Reflect.get(target, key, recv);
    },
  });
  const app = await screen(PlanStep, caseJson({ step: "plan", plan: result }));
  const n = notes(app);
  assert.equal(n.length, 1);
  assert.equal(noteText(n[0]), noteText((await screen(PlanStep, caseJson({ step: "plan", plan: planResult() }))).find((e) => e.props?.["data-review-note"])[0]));
});

test("plan: below the bar there is no best-of line; no result, no note", async () => {
  const closest = move({ shown: false, rejection_kind: "below_confidence_bar", verdict: { confidence: 0.8 } });
  const below = await screen(PlanStep, caseJson({ step: "plan", plan: planResult({ plans: [], rejected: [closest] }) }));
  assert.equal(notes(below).length, 1);
  assert.ok(!noteText(notes(below)[0]).includes(R.bestBounded));
  assert.ok(noteText(notes(below)[0]).includes(J.planScope));
  const none = await screen(PlanStep, caseJson({ step: "origin" }));
  assert.equal(notes(none).length, 0, "before a search there is no result to frame");
});

test("plan: a textile / Part 102 / pharma case keeps its own no-plan screen and gets no plan note", async () => {
  const app = await screen(PlanStep, caseJson({ prompt_versions: { engine: "textile-v1" }, triage: { route: "needs_review", reason: "textile rules" }, plan: planResult() }));
  assert.equal(notes(app).length, 0);
  assert.match(all(app), /No plan search for/);
});

// ---- compare --------------------------------------------------------------

const ranked = [
  { rank: 1, target: "Vietnam", status: "plan", moves_searched: 4, best: move() },
  { rank: 2, target: "India", status: "no_plan", moves_searched: 4 },
];

test("compare: the note and the U.S.-origin / Section 232 line show under the table once, and only once there is a ranking", async () => {
  const app = await screen(CompareView, caseJson({ step: "plan", targets: { ranked } }));
  const n = notes(app);
  assert.equal(n.length, 1);
  assert.equal(n[0].props["data-review-note"], "compare");
  for (const s of [R.compare, R.compareBounded, J.compareScope]) assert.ok(noteText(n[0]).includes(s), s);
  assert.doesNotMatch(noteText(n[0]), /optimal|cheapest|guarantee/i);
  assert.ok(app.btn("Compare again"), "compare control intact");
  const empty = await screen(CompareView, caseJson({ step: "plan" }));
  assert.equal(notes(empty).length, 0);
  assert.ok(empty.btn("Compare these countries"));
});

test("compare: a pinned demo's recorded ranking is framed too, and keeps its demo note", async () => {
  const app = await screen(CompareView, caseJson({ pinned: true, targets: { ranked } }));
  assert.equal(notes(app).length, 1);
  assert.match(all(app), /pinned demo: its recorded comparison is shown/);
});

// ---- scenarios --------------------------------------------------------------

test("scenarios: saved scenarios show the scenario note and the scope line once; the edit and run controls stay", async () => {
  const sc = [
    { label: "Move the motor", prediction: { origin_country: "Vietnam", confidence: 0.9 }, comparison: { net_usd: 100 } },
    { label: "Move the PCBA", prediction: { origin_country: "China", confidence: 0.8 }, comparison: { net_usd: 50 } },
  ];
  const app = await screen(ScenariosView, caseJson({ scenarios: sc }));
  const n = notes(app);
  assert.equal(n.length, 1);
  assert.equal(n[0].props["data-review-note"], "scenarios");
  for (const s of [R.scenarios, R.scenariosBounded, J.compareScope]) assert.ok(noteText(n[0]).includes(s), s);
  assert.ok(app.btn("Edit scenarios"));
  assert.match(all(app), /highest net/, "existing chips stay");
});

test("scenarios: no saved scenarios, no note", async () => {
  const app = await screen(ScenariosView, caseJson());
  assert.equal(notes(app).length, 0);
  assert.ok(app.btn("Predict the scenarios"));
});

// ---- chrome ---------------------------------------------------------------

test("no global navigation disclaimer was added to the app chrome", async () => {
  const { readFile } = await import("node:fs/promises");
  const chrome = await readFile(new URL("../src/components/op/chrome.tsx", import.meta.url), "utf8");
  assert.doesNotMatch(chrome, /ReviewNote|review-note|copy\.review|copy\.jurisdiction|Not a ruling\./, "AppBar and chrome carry no new disclaimer");
  assert.equal(copy.landing.footerDisclaimer, "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.", "landing footer line untouched");
});

test("the privacy copy was not touched", async () => {
  assert.match(JSON.stringify(copy.legal.privacy), /op_visitor/);
  assert.doesNotMatch(JSON.stringify(copy.legal), /Marking origin|U\.S\. origin only/);
});

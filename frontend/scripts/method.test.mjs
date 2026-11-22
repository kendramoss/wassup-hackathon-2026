import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { kobalt, kobaltSources, methodExperiments } from "../src/lib/method-inspector.ts";
import { ARMS, BANDS, DECISIONS, LEAK, PLAN_FAMILIES, PLAN_POST, POST_REVIEW, ROUTER_V3, TIMELINE, ghSlug } from "../src/lib/method.ts";

// The /method page links decision entries by their GitHub anchor and quotes
// figures from docs/NUMBERS.md. These tests fail when a heading is renamed,
// a linked file moves, or a figure no longer appears in the registry.
const root = new URL("../../", import.meta.url);
const read = (p) => readFileSync(new URL(p, root), "utf8");
const headings = (p) => new Set(read(p).split("\n").filter((l) => /^#{2,4} /.test(l)).map((l) => ghSlug(l.replace(/^#+ /, ""))));
const numbers = read("docs/NUMBERS.md");
const page = readFileSync(new URL("../src/components/op/screens/method-page.tsx", import.meta.url), "utf8");

test("every decision anchor matches a heading in docs/DECISIONS.md", () => {
  const h = headings("docs/DECISIONS.md");
  for (const [id, slug] of Object.entries(DECISIONS)) {
    assert.ok(h.has(slug), `${id}: no heading with anchor #${slug}`);
    assert.ok(slug.startsWith(`${id.toLowerCase()}--`), `${id}: anchor belongs to another entry`);
  }
});

test("every decision the page and timeline cite has an anchor", () => {
  const cited = new Set([...page.matchAll(/<D id="(D\d+)"/g)].map((m) => m[1]));
  for (const { failedDecision, fixDecision } of methodExperiments) {
    cited.add(failedDecision);
    cited.add(fixDecision);
  }
  for (const s of TIMELINE) s.refs.forEach((r) => cited.add(r));
  for (const g of ARMS) for (const r of g.rows) r.refs.forEach((x) => cited.add(x));
  for (const id of cited) assert.ok(DECISIONS[id], `${id} is cited but has no anchor`);
});

test("linked files exist, and section anchors match their headings", () => {
  const paths = [...page.matchAll(/blob\("([^"]+)"\)/g)].map((m) => m[1]);
  paths.push(...Object.values(kobaltSources));
  for (const s of TIMELINE) for (const f of s.files ?? []) paths.push(f.path);
  for (const p of paths) {
    const [file, anchor] = p.split("#");
    assert.ok(existsSync(new URL(file, root)), `${file} does not exist`);
    if (anchor) assert.ok(headings(file).has(anchor), `${file}#${anchor} is not a heading`);
  }
});

test("every figure on the page is in docs/NUMBERS.md", () => {
  const p1 = (v) => `${(v * 100).toFixed(1)}%`;
  for (const g of ARMS)
    for (const r of g.rows) {
      const want = r.lo !== undefined ? `${p1(r.v)} (${(r.lo * 100).toFixed(1)}–${(r.hi * 100).toFixed(1)})` : p1(r.v);
      assert.ok(numbers.includes(want), `${r.label}: ${want} not in NUMBERS.md`);
      // A row of repeat runs: each run and their mean are registered (D270).
      for (const v of r.runs ?? []) assert.ok(numbers.includes(p1(v)), `${r.label}: run ${p1(v)} not in NUMBERS.md`);
      if (r.vs) {
        const [f, b] = r.vs.match(/\d+/g);
        assert.match(numbers, new RegExp(`fixed ${f}, broke ${b}|${f} / ${b}`), `${r.label}: ${r.vs}`);
      }
    }
  assert.ok(numbers.includes(`${ROUTER_V3.v3}/${ROUTER_V3.n}`) && numbers.includes(`${ROUTER_V3.v2}/${ROUTER_V3.n}`));
  assert.ok(numbers.includes(`${LEAK.heldOutSlots} of ${LEAK.slots.toLocaleString("en-US")}`));
  assert.ok(numbers.includes(`${LEAK.casesTouched} of ${LEAK.cases}`));
  for (const b of BANDS) for (const n of b.n.match(/[\d,]+/g)) assert.ok(numbers.includes(n), `${b.label}: ${n}`);
});

// #480 / D266: the router cue is a protocol shortfall, not a clean example.
// Every surface that publishes the post-cutoff router figures must say so.
test("the evaluation source and accuracy page qualify the router's development split and cue", () => {
  assert.match(page, /blob\("docs\/EVALUATION.md"\)/);
  assert.match(read("docs/EVALUATION.md"), /trade programs/);
  assert.match(read("frontend/src/lib/method.ts"), /729-ruling dev split/);
  const accuracy = read("frontend/src/components/op/screens/accuracy-page.tsx");
  assert.match(accuracy, /\{" "\}Caveat: the &quot;trade programs/);
});

// Judge panel round 4 (D290): the plan search has its own question, its
// figures are registered, and the 21 Nov steps say why they happened then.
test("the plan section's figures are in docs/NUMBERS.md", () => {
  const [given, notGiven] = PLAN_POST;
  const [shown, found, controls] = given.rows;
  assert.ok(numbers.includes(`shown ${shown.n}, right ${shown.k}/${shown.n} (${shown.lo.toFixed(1)}-${shown.hi})`));
  assert.ok(numbers.includes(`recall ${found.k}/${found.n} = 75.0% (${found.lo}-${found.hi})`));
  assert.ok(numbers.includes(`controls shown ${controls.k}/${controls.n} (${controls.lo}-${controls.hi.toFixed(1)})`));
  const [s2, f2] = notGiven.rows;
  assert.ok(numbers.includes(`right ${s2.k}/${s2.n} (${s2.lo}-${s2.hi.toFixed(1)})`));
  assert.ok(numbers.includes(`recall ${f2.k}/${f2.n} = 41.7% (${f2.lo}-${f2.hi})`));
  assert.ok(numbers.includes(`${PLAN_FAMILIES.k} of ${PLAN_FAMILIES.n} = 65.2% (${PLAN_FAMILIES.lo}–${PLAN_FAMILIES.hi})`));
  assert.ok(numbers.includes("moves all 6 parts whose origin CBP's N357771 change switched from China to Thailand"));
  assert.ok(numbers.includes("Also moves 3 parts that change kept in China: the switch, wires and fasteners"));
  assert.doesNotMatch(page, /6 of its 7 parts/);
  assert.match(page, /id="plans"/);
});

test("the 21 Nov steps are labelled as post-review hardening, with exact dates", () => {
  const late = TIMELINE.filter((s) => s.date === "21 Nov 2026");
  assert.ok(late.length > 0 && late.every((s) => s.period === POST_REVIEW));
  assert.match(POST_REVIEW, /external expert review \(21 Nov 2026\)/);
  assert.doesNotMatch(page, /Since 21 Nov every experiment/);
});

// D441, D442: the plan section's rule-check figures are registered.
test("the plan section's per-move rule-check figures are in docs/NUMBERS.md", () => {
  assert.ok(numbers.includes("25 of 57 moves") && numbers.includes("21 of 23 decided moves"));
});

// D500, D502: the plan section's Part 102 search figures are registered.
test("the plan section's Part 102 plan-search figures are in docs/NUMBERS.md", () => {
  assert.ok(numbers.includes("6 of 12 (every move with the motor)") && numbers.includes("| Moves precedent supports while the rule gives another country | 2 "));
});

test("the Method Kobalt excerpts match the committed recordings", () => {
  const prediction = JSON.parse(read(kobaltSources.prediction));
  const plan = JSON.parse(read(kobaltSources.plan));
  const ask = JSON.parse(read(kobaltSources.ask));
  const fixture = JSON.parse(read(kobaltSources.fixture));
  assert.equal(prediction.date, kobalt.cutoff.date);
  assert.equal(fixture.predict.date, kobalt.cutoff.date);
  assert.match(fixture.source, /N328954 \(15 Nov 2022\)/);
  assert.match(fixture.source, /N357771 \(29 Jan 2026\)/);
  assert.equal(prediction.router.route, kobalt.route);
  assert.equal(prediction.router.regime, kobalt.regime);
  assert.equal(prediction.prediction.origin_country, kobalt.origin);
  assert.equal(prediction.prediction.confidence, kobalt.statedConfidence);
  assert.equal(prediction.triage.route, kobalt.triage);
  assert.deepEqual(prediction.prediction.cited_rulings, kobalt.citations);
  assert.ok(prediction.precedents_detail.every((item) => item.date < kobalt.cutoff.date));
  assert.match(prediction.clarify.questions[0], /heating element manufactured in Thailand/);
  assert.equal(plan.target, kobalt.target);
  assert.equal(plan.stats.moves_searched, kobalt.movesSearched);
  assert.equal(plan.plans.length, kobalt.movesShown);
  const shown = plan.plans[0];
  assert.equal(shown.shown, true);
  assert.equal(shown.verdict.supporting_ruling, kobalt.supporting.id);
  assert.equal(shown.verdict.counter_ruling ?? null, kobalt.counterRuling);
  assert.equal(shown.verdict.confidence, kobalt.verifierScore);
  assert.ok(shown.verdict.trail.some(({ tool }) => tool === "search"));
  assert.ok(ask.reply.steps.some(({ tool, arg }) => tool === kobalt.askTool && arg === kobalt.askChange));
  assert.ok(ask.reply.what_ifs.some(({ change, origin, confidence }) => change === kobalt.askChange && origin === kobalt.askOrigin && confidence === kobalt.askStatedConfidence));
});

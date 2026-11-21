import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { ARMS, BANDS, DECISIONS, LEAK, ROUTER_V3, TIMELINE, ghSlug } from "../src/lib/method.ts";

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
  for (const s of TIMELINE) s.refs.forEach((r) => cited.add(r));
  for (const g of ARMS) for (const r of g.rows) r.refs.forEach((x) => cited.add(x));
  for (const id of cited) assert.ok(DECISIONS[id], `${id} is cited but has no anchor`);
});

test("linked files exist, and section anchors match their headings", () => {
  const paths = [...page.matchAll(/blob\("([^"]+)"\)/g)].map((m) => m[1]);
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

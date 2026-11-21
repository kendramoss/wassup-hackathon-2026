import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// #517: on a phone the code samples on /developers scroll sideways. Each one
// that can scroll must be a named region in the tab order, or a keyboard user
// cannot reach the rest of the line (axe scrollable-region-focusable).
const page = readFileSync(new URL("../src/app/developers/page.tsx", import.meta.url), "utf8");

test("every code sample on /developers goes through Code", () => {
  const raw = [...page.matchAll(/<pre\b[^>]*>/g)].map((m) => m[0]);
  assert.equal(raw.length, 2, `expected only the two <pre> inside Code, found: ${raw.join(" | ")}`);
  const uses = [...page.matchAll(/<Code label="([^"]+)"( wraps)?>/g)];
  assert.ok(uses.length >= 7, `found ${uses.length} code samples`);
  const labels = uses.map((m) => m[1]);
  assert.equal(new Set(labels).size, labels.length, "labels must be distinct so each region has its own name");
  assert.ok(uses.some((m) => !m[2]), "some samples scroll");
});

test("a scrolling sample is a focusable, named region; a wrapping one never scrolls", () => {
  const fn = page.slice(page.indexOf("function Code("), page.indexOf("const example"));
  const [wrapping, scrolling] = fn.split("return (").slice(1);
  assert.match(scrolling, /<pre role="region" aria-label=\{label\} tabIndex=\{0\} className=\{pre\}>/);
  assert.match(wrapping, /<pre className=\{pre \+ wrap\}>/);
  assert.match(page, /const wrap = " whitespace-pre-wrap \[overflow-wrap:anywhere\]"/);
});

test("focus is visible: the global :focus-visible outline is not removed for code", () => {
  const css = readFileSync(new URL("../src/app/globals.css", import.meta.url), "utf8");
  assert.match(css, /:focus-visible \{\s*outline: 2px solid var\(--accent\);/);
  assert.doesNotMatch(page, /outline-none/);
});

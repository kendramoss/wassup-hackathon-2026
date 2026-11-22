import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const portal = readFileSync(new URL("../src/app/developers/portal.tsx", import.meta.url), "utf8");

test("all example types share the labeled, wrapping copy block", () => {
  for (const label of ["JSON input", "Structured output · example", "cURL request", "response · example"]) {
    assert.ok(portal.includes(label), label);
  }
  assert.match(portal, /<CopyCode label=\{setup\.label\}/);
  const blocks = [...portal.matchAll(/<pre\b[^>]*>/g)].map(([block]) => block);
  assert.equal(blocks.length, 1);
  assert.match(blocks[0], /whitespace-pre-wrap \[overflow-wrap:anywhere\]/);
});

test("copy controls announce completion and the scrollable tools are keyboard reachable", () => {
  assert.match(portal, /<button\s+type="button"\s+aria-label=\{\x60Copy \$\{label\}\x60\}\s+onClick=\{copyCode\}/);
  assert.match(portal, /role="status"[^>]*>\s*\{feedback\}/);
  assert.match(portal, /role="region" aria-label="Matching tools" tabIndex=\{0\}/);
  const css = readFileSync(new URL("../src/app/globals.css", import.meta.url), "utf8");
  assert.match(css, /:focus-visible \{\s*outline: 2px solid var\(--accent\);/);
  assert.doesNotMatch(portal, /outline-none/);
});

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

// #517: phone users need access to every line without sideways scrolling.
const portal = readFileSync(new URL("../src/app/developers/portal.tsx", import.meta.url), "utf8");

test("each example uses the labeled, wrapping copy block", () => {
  const samples = [...portal.matchAll(/<CopyCode label="([^"]+)"/g)].map(([, label]) => label);
  assert.deepEqual(samples, [
    "Gemini CLI command",
    "POST /v1/cases/predict",
    "Anonymous cURL request",
    "201 response excerpt · illustrative",
  ]);
  const blocks = [...portal.matchAll(/<pre\b[^>]*>/g)].map(([block]) => block);
  assert.equal(blocks.length, 1);
  assert.match(blocks[0], /whitespace-pre-wrap \[overflow-wrap:anywhere\]/);
  assert.doesNotMatch(blocks[0], /overflow-x-(auto|scroll)/);
});

test("copy is a labeled button and announces completion", () => {
  assert.match(portal, /<button\s+type="button"\s+aria-label=\{\x60Copy \$\{label\}\x60\}\s+onClick=\{copyCode\}/);
  assert.match(portal, /role="status"[^>]*>\s*\{feedback\}/);
});

test("focus is visible on interactive controls", () => {
  const css = readFileSync(new URL("../src/app/globals.css", import.meta.url), "utf8");
  assert.match(css, /:focus-visible \{\s*outline: 2px solid var\(--accent\);/);
  assert.doesNotMatch(portal, /outline-none/);
});

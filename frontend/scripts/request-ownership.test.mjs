import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { requestOwner } from "../src/lib/request-ownership.ts";

test("the newest request owns completion in either order", () => {
  const owner = requestOwner();
  const a = owner.next();
  const b = owner.next();
  assert.equal(owner.isCurrent(a), false);
  assert.equal(owner.isCurrent(b), true);
  const c = owner.next();
  assert.equal(owner.isCurrent(c), true);
  assert.equal(owner.isCurrent(b), false);
});

test("upload and family-check callbacks ignore superseded completions", async () => {
  // Source contract for the two React call sites (#346, #351): the pure owner
  // above is exercised directly; this checks that the async branches consult
  // it before applying a result, an error or busy state.
  const src = await readFile(new URL("../src/components/op/screens/product-step.tsx", import.meta.url), "utf8");
  assert.match(src, /const owner = useRef\(requestOwner\(\)\)\.current/);
  assert.equal((src.match(/if \(owner\.isCurrent\(id\)\) onImported\(r\);/g) ?? []).length, 2); // PDF and CSV
  assert.match(src, /if \(!owner\.isCurrent\(id\)\) return;[\s\S]*?setError\(e\);/);
  assert.match(src, /if \(owner\.isCurrent\(id\)\) setBusy\(false\);/);
  assert.match(src, /const famOwner = useRef\(requestOwner\(\)\)\.current/);
  assert.match(src, /if \(!famOwner\.isCurrent\(id\)\) return;/);
});

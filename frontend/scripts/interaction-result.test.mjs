import assert from "node:assert/strict";
import { test } from "node:test";
import { reportInteractionResult } from "./lib/interaction-result.mjs";

test("a clean audit reports no issues", () => {
  const lines = [];
  const count = reportInteractionResult([], (line) => lines.push(line));
  assert.equal(count, 0);
  assert.deepEqual(lines, ["No interaction issues."]);
});

test("detected issues are deduplicated, printed and counted", () => {
  const lines = [];
  const count = reportInteractionResult(
    ["desktop /cases: focus hidden", "mobile /cases: sticky bar", "desktop /cases: focus hidden"],
    (line) => lines.push(line),
  );
  assert.equal(count, 2);
  assert.deepEqual(lines, ["desktop /cases: focus hidden", "mobile /cases: sticky bar", "2 issue(s)"]);
});

import assert from "node:assert/strict";
import { test } from "node:test";
import { draftsFromScenarios, hasRecordedSetup } from "../src/lib/scenario-drafts.ts";

const current = {
  facts: "Current facts",
  product: {
    components: [
      { group: "pcba", plant: "cn" },
      { group: "housing", plant: "th" },
    ],
    operations: [{ group: "final_assembly", plant: "th" }],
    plants: [{ id: "cn" }, { id: "th" }],
  },
};

test("persisted facts scenario keeps its label and text", () => {
  const drafts = draftsFromScenarios({
    ...current,
    scenarios: [{ label: "Ask-created what-if", facts: "Changed facts from Ask" }],
  });
  assert.deepEqual(drafts, [{ label: "Ask-created what-if", mode: "facts", groups: [], plant: "", facts: "Changed facts from Ask" }]);
});

test("persisted move scenario keeps its label, groups and destination", () => {
  const drafts = draftsFromScenarios({
    ...current,
    scenarios: [
      {
        label: "Custom move",
        product: {
          ...current.product,
          components: [
            { group: "pcba", plant: "th" },
            { group: "housing", plant: "th" },
          ],
        },
        comparison: { moved: [{ group: "pcba", to_plant: "th" }] },
      },
    ],
  });
  assert.deepEqual(drafts, [{ label: "Custom move", mode: "move", groups: ["pcba"], plant: "th", facts: "" }]);
});

test("money and scenario failures keep their own retry action", async () => {
  // #350: the shared error box must invoke the operation that failed.
  const { readFile } = await import("node:fs/promises");
  const src = await readFile(new URL("../src/components/op/screens/scenarios-view.tsx", import.meta.url), "utf8");
  assert.match(src, /setError\(\{ error: err, retry: \(\) => void saveMoney\(e\) \}\)/);
  assert.match(src, /setError\(\{ error: e, retry: \(\) => void run\(\) \}\)/);
  assert.match(src, /<ErrorBox error=\{error\.error\} onRetry=\{error\.retry\}/);
  assert.doesNotMatch(src, /onRetry=\{run\}/);
});

test("moves are still recovered when comparison data was cleared by a money edit", () => {
  const drafts = draftsFromScenarios({
    ...current,
    scenarios: [
      {
        label: "Custom move",
        product: {
          ...current.product,
          components: [
            { group: "pcba", plant: "th" },
            { group: "housing", plant: "th" },
          ],
        },
      },
    ],
  });
  assert.deepEqual(drafts[0].groups, ["pcba"]);
  assert.equal(drafts[0].plant, "th");
  assert.equal(hasRecordedSetup({
    ...current,
    scenarios: [{ label: "Custom move", product: current.product }],
  }), true, "a saved product is still a recorded setup after its comparison is cleared");
  assert.equal(hasRecordedSetup({ ...current, scenarios: [{ label: "Empty" }] }), false);
});

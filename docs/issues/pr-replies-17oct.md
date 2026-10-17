# Replies to paste on the PRs (17 Oct)

## #126 (flaky TestListCases, fake Ask answer)
Thanks, both checked.

**TestListCases:** fixed on `fix/origin-proof-list-order-and-demo-notes` (b9a2360). The cause is Windows' wall clock rather than second precision. The store and the sort use the full `time.Time` (only the JSON row is formatted to the second, and the test compares IDs, not that string). On Windows `time.Now()` ticks every ~15.6 ms, so the test's 2 ms sleep left both cases (and the export after them) on the same timestamp, and the sort fell back to its tiebreak on the random case ID: a coin flip, which matches your 7 of 15. The test now waits 20 ms between steps. Passes 300 of 300 locally. I left the sort as is: equal timestamps can't happen within one product action, and the ID tiebreak keeps the order stable between calls.

**Fake Ask answer:** looks good. It keys on "trade-compliance lead", which is in the case agent's system prompt (`caseagent.System`) and in no other prompt, so the engine, router, verifier and clarify still get the JSON. Vertex is untouched. Keep it.

## #58 (closest move in DEMO.md)
You're right. "Closest" means what the Plan screen shows: the below-bar move with the highest precedent check. In that Vertex seed that's motor-fan assembly + PCBA in Thailand at 0.90 (net $320k/yr), not PCBA + switch at 0.85. The seed's summary line had picked the first below-bar row. Fixed on the same branch (b9a2360):
- `cmd/demo` now picks the highest check, with ties in list order, the same as `plan-step.tsx`. There's a test against the recorded Vertex plan.
- The `docs/DEMO.md` Vertex row is corrected, with a note on what "closest" means. `eval/results/demo-seed-vertex.json` keeps the old line as recorded.
- The LiDAR plan wasn't dumped, so its row may name a weaker move than the screen. Check its Plan page before scripting it.

## #105 (quiet Draft visits)
Checked, all good. `preview: true` returns the draft and skips `ExportedAt`, the history event and the save. D56/D57 on main add the same no-write for visitors on pinned cases, so the two fit together.

## #108 (read but not cited)
Checked, good. It only lists `Precedents` when nothing was cited (no supporting, counter or cited ruling). Each line is marked "read by Origin Proof, not cited; [CONFIRM RELEVANCE]", so the broker sees they're unverified. The test covers it.

## #55
Noted, thanks. Nothing needed from the backend.

## #128
Covered: #111, #112, #115 and #118 were handled last turn.

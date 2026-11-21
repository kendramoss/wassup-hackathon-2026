# Claims reconciliation (issue #286)

This document reconciles the benchmark and architecture claims in the two audit reports (`unde_labs_technical_audit.md` and `unde_labs_technical_operational_audit.md`, kept unchanged as historical records) and in the project documents with what the committed artifacts and code support. It is an evidence ledger, not a new benchmark: nothing here reruns a model, calls a network service or adds data.

Every number in the figure table below is recomputed from the committed files by `claimscheck`, and the command fails if the document, the artifacts or their checksums disagree:

```
cd backend && go run ./cmd/claimscheck
```

The command is read-only, uses only the Go standard library, and needs no credentials or network. Exit status 0 means every check passed, 1 means at least one failed, 2 means a bad command line. `go run ./cmd/claimscheck -print` prints the figure table in this document's format and `-print-manifest` prints the checksum manifest; neither writes a file. Figures are never updated automatically: when an artifact changes, a person decides whether the document or the artifact is wrong.

## 1. How to read the evidence states

| State | Meaning |
|---|---|
| MEASURED | Recomputed here from committed result rows, splits or corpora (the figure table). |
| TRACED | The code and the tests that exercise it were located, and `claimscheck` confirms each cited file, function and test still exists. It does not run the tests or judge whether they are sufficient; `go test ./...` does the first. |
| REPORTED | Stated in a committed document (`eval/RESULTS.md`, `docs/NUMBERS.md`) and not recomputed here, for example confidence intervals. |
| UNVERIFIABLE | Cannot be derived from anything in the repository. Kept, but labelled. |
| UNSUPPORTED | The claim goes beyond what the evidence shows. A correction is given. |
| INCORRECT | The claim contradicts the code or the artifacts. A correction is given. |
| INTENT | A workflow or configuration file says so; the live system was not inspected. |

A source claim that cannot be checked (UNVERIFIABLE) is a different finding from a claim shown to be false (INCORRECT). Section 3 keeps them apart.

## 2. Recomputed figures

Source files are listed in the checksum manifest (section 9). "Held-out" is the 377-ruling final split; "development" is the 469-ruling dev split. The official run is the frozen Vertex AI run of engine-v1 (BM25 retrieval, K = 5, router-v2), scored once, timestamped in `eval/results/RUNLOG.jsonl`. Scoring adjustments (`eval/data/scoring_adjustments.json`) exclude no held-out rows and 6 development rows, and accept either of two countries for one development ruling.

| figure | value | meaning |
|---|---|---|
| `splits.final` | 377 | held-out ruling ids in the split file |
| `splits.dev` | 469 | development ruling ids in the split file |
| `splits.pilot` | 12 | pilot ruling ids (in neither scored set) |
| `final.rows` | 377 | official held-out result rows (Vertex AI, engine-v1) |
| `final.excluded` | 0 | rows dropped by scoring adjustments |
| `final.scored` | 377 | rows scored |
| `final.raw_correct` | 340 | rows whose own `correct` flag is true |
| `final.correct` | 340 | correct after scoring adjustments |
| `final.accuracy_pct` | 90.2 | accuracy, percent |
| `final.error_rows` | 0 | rows with an error string |
| `final.rows_missing_scope` | 0 | result rows with no router-scope row |
| `final.rows_not_bm25` | 0 | rows whose retrieval field is not bm25 |
| `final.label_method_regex` | 355 | held-out labels extracted by regex |
| `final.label_method_manual` | 22 | held-out labels set by hand |
| `final.out_of_scope` | 11 | rows the router sent out of scope |
| `final.date_min` | 2026-01-02 | earliest held-out ruling date |
| `final.date_max` | 2026-09-25 | latest held-out ruling date |
| `final.fast.n` | 300 | fast path: confidence >= 0.92 and router in scope |
| `final.fast.correct` | 288 | fast path correct |
| `final.fast.coverage_pct` | 79.6 | fast path n / scored, percent |
| `final.fast.accuracy_pct` | 96.0 | fast path accuracy, percent |
| `final.fast_conf_only.n` | 306 | confidence >= 0.92, router scope ignored |
| `final.fast_conf_only.correct` | 294 | of those, correct |
| `final.fast_conf_only.coverage_pct` | 81.2 | coverage, percent |
| `final.fast_conf_only.accuracy_pct` | 96.1 | accuracy, percent |
| `final.band_ge95.n` | 271 | confidence >= 0.95 |
| `final.band_ge95.correct` | 265 | of those, correct |
| `final.band_ge95.accuracy_pct` | 97.8 | accuracy, percent |
| `final.band_90_95.n` | 64 | confidence 0.90 to below 0.95 |
| `final.band_90_95.correct` | 50 | of those, correct |
| `final.band_90_95.accuracy_pct` | 78.1 | accuracy, percent |
| `final.band_80_90.n` | 31 | confidence 0.80 to below 0.90 |
| `final.band_80_90.correct` | 18 | of those, correct |
| `final.band_80_90.accuracy_pct` | 58.1 | accuracy, percent |
| `final.band_lt80.n` | 11 | confidence below 0.80 |
| `final.band_lt80.correct` | 7 | of those, correct |
| `final.band_lt80.accuracy_pct` | 63.6 | accuracy, percent |
| `dev.rows` | 469 | development result rows |
| `dev.excluded` | 6 | rows dropped by scoring adjustments |
| `dev.scored` | 463 | rows scored |
| `dev.raw_correct` | 422 | rows whose own `correct` flag is true |
| `dev.correct` | 422 | correct after scoring adjustments |
| `dev.accuracy_pct` | 91.1 | accuracy, percent (development set) |
| `dev.error_rows` | 0 | rows with an error string |
| `dev.rows_missing_scope` | 0 | result rows with no router-scope row |
| `dev.rows_not_bm25` | 0 | rows whose retrieval field is not bm25 |
| `dev.label_method_regex` | 448 | development labels by regex |
| `dev.label_method_manual` | 21 | development labels by hand |
| `dev.out_of_scope` | 8 | rows the router sent out of scope |
| `dev.date_min` | 2025-07-01 | earliest development ruling date |
| `dev.date_max` | 2025-12-31 | latest development ruling date |
| `dev.fast.n` | 368 | fast path n (development set) |
| `dev.fast.correct` | 355 | fast path correct |
| `dev.fast.coverage_pct` | 79.5 | fast path coverage, percent |
| `dev.fast.accuracy_pct` | 96.5 | fast path accuracy, percent |
| `temporal.final_starts_after_dev_ends` | true | first held-out date is after the last development date |
| `labels.rows` | 2790 | labelled corpus rows (eval/data/labels.jsonl) |
| `labels.date_min` | 2023-01-10 | earliest labelled ruling date |
| `labels.date_max` | 2026-09-25 | latest labelled ruling date |
| `labels.with_primary_origin` | 2553 | rows with a single primary origin |
| `labels.rows_after_cutoff` | 0 | rows dated after 2026-09-25 |
| `usmca.rows` | 473 | Part 102 rows (eval/data/labels_usmca.jsonl) |
| `usmca.date_min` | 2015-01-05 | earliest Part 102 ruling date |
| `usmca.date_max` | 2026-09-22 | latest Part 102 ruling date |
| `usmca.with_primary_origin` | 356 | Part 102 rows with a primary origin |
| `final.precedent_slots` | 1885 | retrieved precedent slots over the 377 held-out cases (K = 5) |
| `final.precedent_slots_not_before_case` | 0 | slots dated on or after their case (must be 0) |
| `final.precedent_slots_undated` | 0 | slots whose ruling has no date in the labelled corpora |
| `final.precedent_slots_in_final_split` | 414 | slots that are themselves held-out rulings |
| `final.cases_with_final_split_precedent` | 209 | cases with at least one such slot |
| `usmca.overlap_with_labels` | 182 | Part 102 rulings also in the labelled corpus |
| `usmca.not_in_labels` | 291 | Part 102 rulings absent from the labelled corpus |
| `pool.union` | 3081 | distinct rulings in labelled + Part 102 files |
| `facts.files` | 3081 | fact files in backend/data/facts |
| `runlog.final.time` | 2026-10-15T18:29:23Z | RUNLOG timestamp of the held-out run (UTC) |
| `slice.baseline.source` | dev-gemini-4-argon-prec-engine-v1-vertex.jsonl | run the 30-case slice baseline was cut from |
| `slice.baseline.n` | 30 | slice baseline cases |
| `slice.baseline.correct` | 27 | slice baseline correct |
| `slice.baseline.band95_n` | 22 | slice baseline cases at confidence >= 0.95 |
| `slice.baseline.band95_correct` | 21 | of those, correct |
| `planner_post.pairs` | 15 | frozen post-cutoff flip pairs (D170) |
| `planner_post.real_changes` | 11 | pairs where CBP's two holdings differ |
| `planner_post.controls` | 4 | pairs where work moved and the origin did not |
| `planner_post.core_pairs` | 10 | pairs tiered core |
| `planner_post.known_a.error_rows` | 0 | precedent-check rows with an error |
| `planner_post.known_a.shown` | 8 | moves the check supported at the 0.92 plan bar |
| `planner_post.known_a.shown_right` | 8 | of those, real changes with CBP's later origin named |
| `planner_post.known_a.controls_shown` | 0 | no-change pairs supported at the bar |
| `planner_post.known_a.recall_pct` | 72.7 | real changes shown and right, percent |
| `cal.version` | cal-v2 | calibration map version (backend/internal/calibrate/map.json, D160) |
| `cal.bar` | 0.9 | auto-accept bar on calibrated probability |
| `final.cal.fast.n` | 300 | auto-accepted at the calibrated bar and in scope |
| `final.cal.fast.correct` | 288 | of those, correct |
| `final.cal.fast.accuracy_pct` | 96.0 | accuracy of auto-accepted, percent |
| `final.cal.mean_stated_pct` | 94.7 | mean stated confidence, percent |
| `final.cal.mean_calibrated_pct` | 90.2 | mean calibrated probability, percent |
| `final.cal.ece_stated` | 0.0585 | ECE of stated confidence, one bin per value |
| `final.cal.ece_calibrated` | 0.0285 | ECE of calibrated probability, one bin per value |
| `dev.cal.fast.n` | 368 | auto-accepted at the calibrated bar and in scope (development set; the bar was fitted here) |
| `dev.cal.fast.correct` | 355 | of those, correct |
| `dev.cal.fast.accuracy_pct` | 96.5 | accuracy of auto-accepted, percent |
| `dev.cal.mean_stated_pct` | 94.9 | mean stated confidence, percent |
| `dev.cal.mean_calibrated_pct` | 90.8 | mean calibrated probability, percent (in sample) |
| `dev.cal.ece_stated` | 0.0498 | ECE of stated confidence |
| `dev.cal.ece_calibrated` | 0.0031 | ECE of calibrated probability (in sample: the map was fitted here) |

Two further consistency checks run on every invocation: the held-out and development result ids must equal the ids in the split file, the `global` policy counts in `eval/results/calibration-by-family.json` must equal the fast-path counts recomputed from the rows (a separate program produced that file), and the calibrated bar in `backend/internal/calibrate/map.json` must accept exactly the cases the stated 0.92 bar accepts, on both splits (D160; the `cal.` rows are recomputed in Go from that map, independently of the Python build that wrote it). The baseline and lift figures discussed in 3.3 are deliberately not in this table: they cannot be recomputed from the repository.

## 3. Benchmark claims

### 3.1 Accuracy (cohort, date, configuration, uncertainty)

| Claim (source) | State | Evidence and correction |
|---|---|---|
| 90.2% on 377 held-out 2026 rulings (both audits, README, NUMBERS) | MEASURED | 340 of 377 correct (`final.correct`, `final.scored`). No rows excluded, no error rows. Ruling dates 2026-01-02 to 2026-09-25. The labels are regex-extracted for 355 rulings and set by hand for 22. The population is the cleaned split recorded in `eval/splits/splits_final.json` (its own `rule` field: single-origin substantial-transformation rulings, 2026, pilot cases removed), not every CBP origin ruling. The 95% interval 86.8 to 92.8 is REPORTED in `eval/RESULTS.md`. |
| Held-out set is "unseen" | MEASURED, see 4.2 | The held-out cases were not used to tune prompts (REPORTED, `docs/DETAILS.md`). They are not absent from the retrieval pool: 414 of 1,885 retrieved precedent slots are earlier held-out rulings. |
| "Strict temporal split" between retrieval corpus and test set (both audits) | INCORRECT as worded | The split between development and held-out is by date, and every retrieved precedent is dated strictly before its case (4.1). The retrieval corpus is not limited to rulings before 2026: it runs to 2026-09-25 and is filtered per case. |
| Development set 91.1% | MEASURED | 422 of 463 after 6 exclusions (`dev.correct`, `dev.scored`); 90.0% unadjusted (422 of 469). Development numbers carry the label "development set" because the fast-path bar was chosen on them. |

### 3.2 High-confidence tier

| Claim (source) | State | Evidence and correction |
|---|---|---|
| 96.0% in the high-confidence tier (both audits) | MEASURED | One scored run, so no run-to-run variance is measured; `eval/RESULTS.md` reports that first-call confidence varied between proxy runs enough to move fast-path coverage by about 3 points (REPORTED, development set). The tier is exactly: stated confidence at least 0.92 AND the router places the ruling in scope. It covers 300 of 377 cases (`final.fast.n`) and 288 are correct (`final.fast.correct`), 96.0%. 12 wrong answers are auto-accepted. The 95% interval 93.1 to 97.7 is REPORTED. |
| "approximately 80%" / "~80% confident subset" (operational audit) | MEASURED | Coverage is 79.6% (`final.fast.coverage_pct`). Without the router-scope condition the confidence bar alone selects 306 cases (81.2%) at 96.1%. The two definitions differ; cite the first. |
| Separate "confidence at least 0.95" band | MEASURED | 265 of 271 (97.8%). This is a different tier from the 0.92 fast path and must not be mixed with the 96.0%. |

### 3.3 Baseline and lifts

| Claim (source) | State | Evidence and correction |
|---|---|---|
| Heuristic baseline "most-mentioned country" is 60.5% on all 377 cases | UNVERIFIABLE | The only trace in the repository is a literal in `backend/cmd/eval/main.go:1353#baseline_most_mentioned` and its copy in `backend/internal/evalsummary/summary.json#baseline_most_mentioned`, each citing a research results file that is not committed. No baseline predictions are committed and no code computes the baseline, so neither the value, its interval, nor its exact definition can be reproduced here. It stays in the documents as reported, unverified, and must not be described as verified against raw data. `claimscheck` fails if that research file ever appears under `eval/` or `docs/`, because the finding would then be stale. |
| "+29.7" lift of all-case accuracy over the baseline | UNVERIFIABLE | Arithmetic on the unverified baseline: 90.2 minus the reported figure. It is a difference of percentages, so the unit is percentage points, not "%". It is the only all-case lift that can be stated, and only as relative to the unverified figure. |
| "+35.5" lift of the high-confidence tier over the baseline | UNSUPPORTED | It subtracts an all-377 baseline from the accuracy of a 300-case subset that the model's own confidence selected. A like-for-like lift needs the baseline scored on the same 300 cases. That is not committed, so the lift cannot be stated. |
| Heuristics "fail nearly 40% of the time ... the engine weighs process complexity over raw component count" (technical audit) | UNVERIFIABLE | The 40% is 100 minus the unverified baseline. The second sentence is an interpretation: no committed ablation separates process weighting from other engine behaviour. |

### 3.4 Calibration

| Claim (source) | State | Evidence and correction |
|---|---|---|
| The 5.8% jump from 90.2% to 96.0% "demonstrates" / "confirms" reliable confidence calibration (both audits) | INCORRECT inference | 96.0 minus 90.2 is 5.8 percentage points. Any subset selected by a confidence score with some signal scores higher than the whole set, so the jump shows the score carries information, not that it matches observed frequency. A percentage-point lift does not prove calibration. |
| Confidence bands on the held-out set | MEASURED | At least 0.95: 265 of 271 (97.8%). 0.90 to 0.95: 50 of 64 (78.1%, interval 66.6 to 86.5 REPORTED). 0.80 to 0.90: 18 of 31 (58.1%, 40.8 to 73.6 REPORTED). Below 0.80: 7 of 11 (63.6%, 35.4 to 84.8 REPORTED). Read as probabilities, the two middle bands are overconfident: their intervals lie below their stated ranges. The lowest band scores higher than the one above it, on 11 cases, which supports no conclusion. |
| No calibration statistic exists in the repository | SUPERSEDED (21 Nov) | True at the base commit for `backend/`. Since then `eval/charts/build_evaluation.py` computes ECE, Brier and a reliability diagram offline from the committed rows: held-out mean confidence 94.7% against accuracy 90.2%, ECE 0.059 (0.041 to 0.091), Brier 0.081 against 0.089 for a constant forecast (`eval/results/evaluation-metrics.json`; docs/EVALUATION.md §4.2). The confidence is overconfident on average; it is still a self-report used against a bar, not a calibrated probability. |
| Per-family bars (`eval/results/calibration-by-family.json`) | REPORTED | Fitted on development only and applied once to the held-out set: 287 of 377 auto-accepted at 94.1% against 300 at 96.0% for the global bar. Not adopted (`FAST_PATH_BY_FAMILY` stays off). Reported in `eval/RESULTS.md`. |
| Review routing "reliably flags ambiguous edge cases" | MEASURED, narrower | Of the 377 cases, 77 were not auto-accepted and 52 of those were nevertheless correct (derived: 377 minus 300, 340 minus 288). Of the 37 wrong answers, 25 were routed to review and 12 were auto-accepted. That is a routing outcome on this set, not evidence of reliability on new rulings. |

### 3.5 Corpus and retrieval

| Claim (source) | State | Evidence and correction |
|---|---|---|
| "2,790+ historical CBP rulings", "prior to 2026" (both audits) | MEASURED, wording corrected | The labelled corpus has 2,790 rulings dated 2023-01-10 to 2026-09-25, of which 2,553 have a single primary origin (`labels.*`). It does not stop at 2026. The facts directory holds 3,081 files, which equal the 2,790 labelled rulings plus 291 Part 102 rulings absent from them (`usmca.not_in_labels`); the Part 102 file has 473 rows, 182 of them also labelled, dated 2015-01-05 to 2026-09-22 (`usmca.*`, `pool.union`, `facts.files`). The general BM25 index is built from the labelled file and the Part 102 engine indexes the Part 102 file (`backend/cmd/server/main.go`). The copies the backend ships are byte-identical to the checksum-pinned `eval/` copies. `docs/DETAILS.md` and the README say "2,790"; the labelled count is right and the facts pool is larger. |
| "Firestore vector search" / "vector-indexed corpus" (both audits) | INCORRECT | Production retrieval is an in-process BM25 index (`backend/cmd/server/main.go:731#evidenceBM25`). No vector or nearest-neighbour query to Firestore exists. Embedding re-ranking (`retrieval.Hybrid`) is used only by the offline `cmd/eval` experiments; on the development set (proxy runs) the hybrid scored 88.9% and pure embedding retrieval 89.8% against 90.0% for BM25, neither difference significant, and neither was adopted (REPORTED, `eval/RESULTS.md`). All 377 held-out rows record BM25 retrieval (`final.rows_not_bm25`). Firestore stores cases, caches, rate-limit counters and API keys. |

## 4. Temporal controls, and planner evidence kept apart from accuracy

### 4.1 Controls that exist (TRACED and MEASURED)

- Date split: development rulings end 2025-12-31 and held-out rulings start 2026-01-02 (`temporal.final_starts_after_dev_ends`); no labelled ruling is dated after 2026-09-25 (`labels.rows_after_cutoff`).
- Per-case precedent filter: a precedent must be strictly earlier than the case and not the case itself or an excluded ruling (`backend/internal/retrieval/bm25.go:152#PrecedentFilter`, `backend/internal/domain/ruling.go:47#UsableAsPrecedentFor`). Measured on the official run: none of the 1,885 retrieved precedent slots is dated on or after its case, and none is undated (`final.precedent_slots_not_before_case`, `final.precedent_slots_undated`; the command fails if the first is not 0).
- Verifier date bound: a verdict may cite only rulings dated strictly before the case (`backend/internal/verifier/verifier.go:656#inBound`, `backend/internal/verifier/verifier.go:619#eligibleHits`).

### 4.2 Limits of those controls

- The filter is by date, not by split, so earlier held-out rulings and their holdings are retrievable for later held-out cases: 414 of 1,885 slots, in 209 of 377 cases (`final.precedent_slots_in_final_split`, `final.cases_with_final_split_precedent`). That mirrors a deployed system that always has earlier rulings; it does mean the held-out set is not disjoint from the retrieval pool. No committed run measures the effect of removing those slots.
- Model pre-training exposure to rulings is not excluded (REPORTED, `docs/DETAILS.md`).
- The production verifier searches the live CROSS database with a date bound and its tool path varies between runs (code comment in `backend/internal/verifier/cache.go`), so verifier outcomes are not reproducible offline; a cache replays recorded verdicts.

### 4.3 Planner evidence is not origin-prediction accuracy

- Latest evaluation AVAILABILITY: the latest official origin-engine run is the held-out run at the timestamp in `runlog.final.time`; the latest ruling in any committed evaluation artifact is dated 2026-09-25. Since D170 one committed artifact does: the production precedent check, scored once on 15 frozen flip pairs with at least one ruling published after 2026-09-25 (`eval/data/flips_post.jsonl`, rows `eval/results/flips-post-verifier-v2-known-a-litellm.jsonl`, figures `planner_post.*`). It is a small set (11 real changes, 4 controls), so it is evidence about the check, not a planner accuracy figure. The README still says the planner is the least tested part.
- ACCURACY: no planner accuracy figure exists in the repository and none is claimed here. The heat-gun and LED tub-light plans in `docs/NUMBERS.md` and `docs/DETAILS.md` are single demonstration runs; the verifier has development-set pair evidence reported in `docs/NUMBERS.md` (development set, proxy runs, not recomputed here). Neither is origin-prediction accuracy, and neither is a held-out planner result.
- A pull-request eval slice (`.github/workflows/eval-slice.yml`, baseline `eval/results/slice30-baseline.json`: 27 of 30 correct, 21 of 22 at confidence 0.95 or higher, cut from the development run, `slice.baseline.*`) is a regression smoke test. Its latest observed run (Actions run 44677154743, reported from the run logs, not reproducible from the repository) was incomplete: 9 of 30 rows were Vertex HTTP 504 errors, 21 were answered and 17 of those correct. Three of the four wrong answers were already wrong in the committed baseline. An incomplete run on a provider outage supports no overall accuracy claim and no new-regression claim, and its 0.95-band check ran on a truncated sample.

## 5. Capability trace

Format: `file:line#symbol` (the line is a hint; `claimscheck` fails on a missing file or symbol and only notes a moved line) and `test-file#TestName`.

| Capability | Code | Tests | Scope caveat |
|---|---|---|---|
| Corpus ingestion | `backend/internal/corpus/store.go:35#Load` | `backend/internal/corpus/store_test.go#TestLoadStore`, `backend/internal/corpus/store_test.go#TestLoadSplitsAndLabelsFromRepo` | Raw ruling text is not committed; fetching needs the network and is not rerun here. |
| BOM ingestion (CSV, Excel) | `backend/internal/bom/bom.go:53#ReadRows`, `backend/internal/bom/bom.go:182#Parse` | `backend/internal/bom/bom_test.go#TestParseCSVToPlannableProduct`, `backend/internal/bom/bom_test.go#TestParseExcel`, `backend/internal/bom/bom_test.go#TestParseErrorsAndWarnings` | Normalises into the product model; free-text narratives are not parsed here. |
| BM25 retrieval (official) | `backend/internal/retrieval/bm25.go:74#NewBM25`, `backend/internal/engine/engine.go:79#Predict` | `backend/internal/retrieval/bm25_test.go#TestSearchRanksAndFiltersByDate`, `backend/internal/retrieval/bm25_test.go#TestSearchExcludesTheCaseItself`, `backend/internal/retrieval/bm25_test.go#TestSearchHonoursExcludeList`, `backend/internal/retrieval/parity_test.go#TestResearchParity` | The index is in process memory, rebuilt at startup. |
| Vector search (experimental, not official, not in production) | `backend/internal/retrieval/vector.go:147#Hybrid`, `backend/internal/retrieval/vector.go:107#LoadVectors` | `backend/internal/retrieval/vector_test.go#TestVectorsSaveLoad`, `backend/internal/retrieval/vector_test.go#TestHybridReranksByEmbedding` | Re-ranks BM25 candidates; used only by `cmd/eval`, not by the server (`backend/cmd/server/main.go:731#evidenceBM25`). No Firestore vector query exists. |
| Origin inference | `backend/internal/engine/engine.go:79#Predict`, `backend/internal/router/router.go:127#Classify`, `backend/internal/triage/triage.go:18#Decide` | `backend/internal/engine/engine_test.go#TestPredict`, `backend/internal/engine/engine_test.go#TestVotingPicksMajorityOverRotations`, `backend/internal/router/router_test.go#TestClassify`, `backend/internal/triage/triage_test.go#TestDecide` | "Confidence" is the model's self-report compared with a bar; see 3.4. |
| Relocation search | `backend/internal/planner/plan.go:185#PlanWithProgress`, `backend/internal/planner/enumerate.go:68#EnumerateUpTo`, `backend/internal/planner/plan.go:65#DefaultMaxMoves` | `backend/internal/planner/plan_test.go#TestPlanShowsSupportedMoveAndRejectsTheRest`, `backend/internal/planner/plan_test.go#TestPlanNeedsVerifierConfidence`, `backend/internal/planner/planner_core_test.go#TestEnumerateUpToCapsAndPrefersCostlyGroups` | Searches a bounded set of template moves to listed plants (at most `DefaultMaxMoves`, 12). Not an optimiser; see section 6. |
| Verifier (move check, not a repair loop) | `backend/internal/verifier/verifier.go:397#Verify`, `backend/internal/verifier/verifier.go:288#originViolation` | `backend/internal/verifier/verifier_test.go#TestVerifySupported`, `backend/internal/verifier/verifier_test.go#TestVerifyEnforcesToolBudget`, `backend/internal/verifier/excluded_support_test.go#TestRulingOutsideDateBoundIsNotSupport`, `backend/internal/verifier/excluded_support_test.go#TestExcludedRulingDirectlyRetrievedIsRejectedAtAcceptance` | It accepts or rejects a proposed move with a supporting or counter ruling. Rejected moves are not automatically revised; near-miss advice is a separate, optional step (`backend/internal/planner/plan.go:441#adviseNearMisses`). "Precedent-supported" is not a ruling. |
| Caching | `backend/internal/engine/cache.go:29#Cache`, `backend/internal/verifier/cache.go:31#Cache` | `backend/internal/engine/cache_test.go#TestCacheReplaysPredictions`, `backend/internal/engine/cache_test.go#TestCacheEvidenceRevision`, `backend/internal/verifier/cache_test.go#TestCacheReturnsTheSameVerdict`, `backend/internal/httpapi/cases_test.go#TestRulingIsProxiedAndCached` | Successful ruling lookups are cached. The audit's "failed CROSS lookups are not cached" has no test in the traced set and is not claimed here. |
| Input bounds | `backend/internal/bom/bom.go:33#MaxRows`, `backend/internal/bom/bom.go:42#maxSheetRows`, `backend/internal/bom/bom.go:43#maxSheetCols`, `backend/internal/bom/bom.go:45#maxFieldLen`, `backend/internal/httpapi/products.go#maxUpload`, `backend/internal/httpapi/cases.go:224#maxBody` | `backend/internal/bom/xlsx_bomb_test.go#TestExcelUnzipBombIsRefused`, `backend/internal/bom/xlsx_bomb_test.go#TestExcelWideAndLongSheetsAreBounded`, `backend/internal/bom/bom_test.go#TestReadRowsCSVCaps`, `backend/internal/httpapi/cases_test.go#TestBodyLimit`, `backend/internal/httpapi/economics_validation_test.go#TestEconomicsInvalidRefusedBeforeWork` | The "1,000 rows by 64 columns" in the audit is the read cap for a sheet or a CSV (CSV since D137, with a 2,000-character cell cap). A BOM itself is capped at 500 rows, uploads at 5 MiB and JSON bodies at 256 KiB. |
| Rate limits | `backend/internal/httpapi/limit.go:48#DefaultLimits`, `backend/internal/httpapi/limit.go:191#ipKey`, `backend/internal/httpapi/limit.go:175#clientKey` | `backend/internal/httpapi/manage_test.go#TestIPv6GroupedBySlash64`, `backend/internal/httpapi/limit_shared_test.go#TestSharedLimiterTwoInstancesOneLimit`, `backend/internal/httpapi/limit_shared_test.go#TestSharedLimiterFailsOpenToLocalLimits`, `backend/internal/httpapi/hardening_test.go#TestDailyBudgetRefusesModelCalls` | Defaults are in code (20 light, 4 heavy, 120 global per minute); deployed values come from configuration and were not inspected. The client address is taken from a header the frontend proxy sets, which is sound only if the backend is reachable through that proxy alone (INTENT, deploy comment). The shared limiter fails open to local limits when its store errors. |
| Persistence | `backend/internal/store/firestore.go:28#Firestore`, `backend/internal/store/store.go:369#Memory` | `backend/internal/store/store_test.go#TestStoresListNewestFirstAndFilePersists`, `backend/internal/store/firestore_test.go#TestFirestoreEmulator` | The Firestore test skips unless an emulator is configured, so the default `go test` does not exercise Firestore. Live persistence was not inspected. |
| Testing in CI | `.github/workflows/ci.yml#staticcheck`, `.github/workflows/ci.yml#ci-ok` | `go test ./...` in the backend job of the workflow | CI runs gofmt, build, vet, staticcheck, `go test ./...` without `-race`, frontend lint and build. The audit's "Go race tests" and "Playwright/axe checks" are not run by the workflows (a search of `.github/` and `Makefile` finds no `-race`, and no workflow runs `npm test` or `ui-check`). The tooling exists in the repository. |
| Deployment safeguards | `.github/workflows/deploy.yml#update-traffic`, `.github/workflows/deploy.yml#--min-instances`, `backend/internal/httpapi/health.go:19#HealthCheck` | `backend/internal/httpapi/health_probe_test.go#TestDeepHealthFailingStoreStill503` | INTENT only; the live service was not inspected. The workflow deploys after CI, then a verify job fails if the latest ready revision is not the serving one. It contains no explicit zero-traffic gate on `/health` and no rollback step; any such behaviour is the platform's and unverified here. The audit's "automatic rollback" is therefore UNSUPPORTED by the workflow file. |

## 6. Legal and product scope

- Part 102 marking support is not USMCA regional value content. The Part 102 engine applies the general marking rules and tariff shifts of 19 CFR 102.11 and 102.20 to Canada- and Mexico-related goods (`backend/internal/engine/usmca.go:12#USMCASystem`, version `usmca-v1`); textile goods use 102.21. The word "USMCA" in file names refers to that marking use. No regional-value-content or preferential-origin qualification is implemented (a search of the Go code, the frontend and the documents finds none; the term appears only in the subjects of corpus rulings). Drafts keep Section 301 distinct from marking origin (`backend/internal/export/regime.go:24#section301Separate`, `backend/internal/export/regime_test.go#TestPart102DoesNotSettleSection301`).
- Part 177 is a ruling-request procedure, not an origin rule. `backend/internal/export/cfr177.go:18#CFR177` drafts a request under 19 CFR 177.1 and 177.2 (`backend/internal/export/export_test.go#TestCFR177Contents`). The audit line "19 CFR Part 102 / Part 177" mislabels it; origin rules are 102.11 and 102.21 and the substantial-transformation standard cited at 134.1(b), with the textile and pharmaceutical engines.
- EU Binding Origin Information (and any other non-US origin source) is unsupported: the corpus is U.S. CBP rulings only. The audits already say so; this is the same limit stated again.
- The full U.S. scope, the configured regimes, the unsupported use cases and the EU BOI and USMCA RVC deferral are in [docs/scope-and-jurisdiction.md](scope-and-jurisdiction.md). It states no benchmark numbers.
- "Binding" citations. The audits describe engine output as citing "binding CBP rulings". The product cites earlier rulings as precedent. Under 19 CFR 177.9 a CBP ruling letter is the agency's position on the transaction and requester described, and other parties should not assume it applies to a different transaction; it can also be modified or revoked. Legal statement from the regulation, not testable from the repository; confirm with counsel. The README already says "confirm with a licensed customs broker or a CBP binding ruling" and the drafts ask CBP for a binding ruling about the requester's own transaction.
- "Cost-optimal" footprints. The planner ranks searched moves by modelled duty effect minus user-entered added cost, and labels those inputs illustrative (`backend/internal/planner/doc.go`, `backend/internal/planner/plan.go:495#economics`). It does not model capital expenditure, re-tooling lead time, minimum order quantities or labour and logistics (both audits say this too), searches at most 12 moves (single, double and capped triple moves) and relies on precedent support. "Best of the searched moves under the entered costs" is supportable; "cost-optimal" is not.

## 7. Wording corrections (proposed; source documents are not rewritten)

| Where | Original wording | State | Proposed wording |
|---|---|---|---|
| Both audits | "Cloud Firestore vector search", "vector-indexed corpus" | INCORRECT | "an in-process BM25 index over the labelled rulings; embedding re-ranking was an offline experiment that was not adopted" |
| Both audits | "binding CBP rulings" | UNSUPPORTED | "earlier CBP rulings used as precedent; a ruling binds CBP and the requester for the transaction it describes" |
| Both audits | "cost-optimal manufacturing footprints" | UNSUPPORTED | "the best of the searched moves under the entered, illustrative costs" |
| Both audits | "Delta ... +35.5%" | UNSUPPORTED, not like-for-like | remove; it compares a 300-case subset with an all-case baseline |
| Both audits | "+29.7%" | UNVERIFIABLE | "29.7 percentage points above a reported, unverified 60.5% baseline" |
| Both audits | "5.8% jump ... confirms calibration" | INCORRECT | "96.0% against 90.2% overall, a 5.8 point difference; the tier is selected by the model's own confidence and calibration was not measured" |
| Both audits | "Historical CBP rulings issued prior to 2026" | INCORRECT | "labelled rulings dated 2023-01-10 to 2026-09-25, each case seeing only earlier ones" |
| Both audits | "19 CFR Part 102 / Part 177" | INCORRECT | "Part 102 origin rules for marking; Part 177 governs ruling requests" |
| Operational audit | "~80% confident subset" | MEASURED | "79.6% of cases (300 of 377): confidence at least 0.92 and in router scope" |
| Operational audit | "Go race tests", "Playwright/axe checks" | UNSUPPORTED in CI | "Go tests, vet and staticcheck run in CI; race and browser checks are not part of the workflows" |
| Operational audit | "zero traffic until /health passes with automatic rollback" | UNSUPPORTED by the workflow | describe the verify job; do not claim rollback without evidence |
| `docs/NUMBERS.md` | baseline row status "Showable" | UNVERIFIABLE | "Reported, unverified (source not committed)" |

The same proposed replacements, as exact strings for the integrator, are in the unit report.

## 8. What this document does not establish

- No live system, deployed configuration or cloud resource was inspected; every INTENT row is workflow text.
- Confidence intervals, label counts by method, cost and latency figures are REPORTED from `eval/RESULTS.md`, not recomputed.
- The 60.5% baseline is unverifiable, and so are the facts-only and research-run comparisons, and the change-detection and verifier-pair results in `docs/NUMBERS.md` cannot be rederived from committed rows.
- Tracing a test shows it exists, not that it is sufficient; `go test ./...` is the evidence that it passes.
- No model, provider, network or paid call was made to produce or check this document, and no accuracy claim is made beyond the held-out and development numbers above. No privacy, certification or production-readiness claim is made or implied.

## 9. Checksum manifest

`eval/SHA256SUMS` already pins the labelled corpora and the splits, and `claimscheck` verifies it. The block below pins the other files the check reads (the facts directory is pinned by the digest of its sorted file-name list instead of repeating 3,081 hashes). A changed file fails the check until a person reviews it and refreshes the line. Regenerate with `cd backend && go run ./cmd/claimscheck -print-manifest`.

```
56b9ce96830561d4cc481efb40517d3bb5594847b6df389b2c450dc53a1fe244  eval/SHA256SUMS
727918a055ad3bb10d13392c7859da004f2d8cea2b90fd4a564e71a326d326ad  eval/data/scoring_adjustments.json
2d8a357af6eeb134485c05d60af08eb15f75c37b39ae49b9fc2a14203c34ec81  eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl
f5c24fac4e27a88941c981c7ff042b8e4094d55441c7a6bf67280c342c164551  eval/results/scope-final-router-v2-vertex.jsonl
c9e79d732fa25ecedd4213daeaf0b9d62ce86138a639d05ffde9697e156f0ba6  eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl
48e86727d5bbc59c415c8b7ae84325a86ce008492f8c7a8711077718d9948f9d  eval/results/scope-dev-router-v2-vertex.jsonl
33a6d3188fb0b04f367b95707306c1ff15a7519359faa72ad3c45c6237f41653  eval/results/RUNLOG.jsonl
38bdf2db05f4f56f0b13450ec4a4531940932d675bfc88a2288995b25ae06a47  eval/results/calibration-by-family.json
705d40de8a3cf285471ba23589401c77504900eab36abd5c6db53efaf5083cb9  eval/results/slice30-baseline.json
777614596d93d6c0e755809811a160b2dbbdc3a1f529f62eca6010e434c80057  eval/results/flips-post-verifier-v2-known-a-litellm.jsonl
7fba593939b585c06e05ba9ee46db38c5fff0896aa8104427e71c273890a0f4b  backend/internal/calibrate/map.json
dc7e82fe63a2b4de2c77984169577387dd8816d73f66b6b43575d5b74f159b85  backend/data/labels.jsonl
6040e8508af239d3d23fc360b2b5a8d06742125d3547ef298e35f8e71b600f9d  backend/data/labels_usmca.jsonl
ad7227af446b0412395673de95749d0da1ab2ab54de3bd99c63e49084e4d7d08  names:backend/data/facts
```

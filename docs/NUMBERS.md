# On-screen numbers and their sources

Every number that may appear in the demo, deck or results page. **Status** says whether it can be shown today. The held-out engine, fast path, cost and latency rows below are the Vertex AI re-run (D39). "Proxy" = produced through the LiteLLM proxy: development only, and not a substitute for those rows. "Gateway" rows ran through an OpenAI-compatible LiteLLM gateway to Gemini. Gateway and Vertex AI agree on the development set (same engine and model, exact McNemar p = 0.58, D151); the gateway runs were not repeated on Vertex AI.

## Evaluation (results page — served by `GET /v1/eval/summary`)

| Number | Value | Source | Status |
|---|---|---|---|
| Held-out origin accuracy (377 rulings, Jan–Sep 2026) | **90.2% (86.8–92.8)**, Vertex AI, frozen engine-v1. Proxy re-run 89.9% (86.5–92.6); research run 89.1% (85.6–91.9) | `eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl` (D39). Proxy file kept for the comparison | Official. Proxy and research figures stay labelled as such |
| Best simple baseline (first country named, recomputed; D155) | **63.4% (58.4–68.1)**; most-mentioned country 60.7% (55.7–65.5) | `eval/results/baselines-final.json` (`go run ./cmd/eval baselines`, no model); row "Rule baselines, recomputed" | Measured. Use these. Never show a lift between a baseline and the 96.0% fast-path row: the fast path is a 300-case subset, the baselines are all 377 |
| History: research baseline (most-mentioned country) | 60.5% (55.5–65.3) | research `RESULTS-v2.md` (no model involved); not committed, so not reproducible here | Reported, unverified, superseded by the recomputed 60.7% (D155). Kept as history; do not show |
| Other baselines | final-assembly 40.6%, last-mentioned 39.8%, majority 12.2% (majority since recomputed exactly, D155) | research `RESULTS-v2.md`; not committed, so not reproducible here | Reported, source not in this repository, unverified. Show only with that label, like the 60.5% row |
| Fast path (calibrated confidence ≥ 0.90, the same cases as stated confidence ≥ 0.92, and in scope; D160), held-out | 79.6% of cases (300/377) at 96.0% (93.1–97.7), Vertex AI, router-v2. Proxy was 78.5% at 96.6% (93.9–98.2). At the old 0.95 bar, proxy coverage was 70.3% at 97.7% | final Vertex rows + `eval/results/scope-final-router-v2-vertex.jsonl` | Official. The proxy figure stays labelled proxy. 288 of 300; the subset is selected by the model's own confidence and router scope |
| Origin outcome matrix, first held-out test set | 340/377 right overall. Auto-accepted: 288 right, 12 wrong. Human review: 77/377 (20.4%), 52 right and 25 wrong. | `frontend/src/lib/failure-cases.json` (from `eval/charts/build_trust_data.py . --check`); cross-checked against final accuracy, fast-path accuracy and coverage in `backend/internal/evalsummary/summary.json` | Official, Vertex AI. Review includes cases outside the fast path. Correctness is measured against CBP before any human review. |
| Confidence ≥ 0.95 alone, held-out | 71.9% at 97.8% (95.3–99.0) | final rows | Proxy. Do **not** show the research "65% at 98.0%": it describes a different routed pipeline |
| Change detection from scratch, held-out 2026 pairs | 68% (47–84), n=22; always-change baseline 63.6%; balanced accuracy 61.6% | research `RESULTS-v2.md` | Showable, always with both baselines; label "weak" |
| Move verifier (verifier-v2), development set (54 pre-2026 pairs) | precision 32/32 (89–100), recall 69.6% (55.2–80.9, n=46), specificity 8/8; with CBP's current origin given: 97.3% of 37 (86.2–99.5), 78.3%, 7/8 | `eval/results/flips-dev-verifier-v2-{unknown,known}-a-litellm.jsonl` (served by `/v1/eval/summary`). The earlier 34/34 and 74% were verifier-v1 | Proxy; must say **"development set"** |
| Development engine accuracy (Jul–Dec 2025) | Unadjusted, 469 rulings: 422/469 = 90.0% (86.9–92.4); fast path 371/469 = 79.1% at 355/371 = 95.7% (93.1–97.3). After scoring adjustments (D23), 463 rulings: 422/463 = 91.1% (88.2–93.4); fast path 368/463 = 79.5% at 355/368 = 96.5% (94.1–97.9). Always say which | `eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl` + `eval/results/scope-dev-router-v2-vertex.jsonl` + `eval/data/scoring_adjustments.json`; adjusted figures recomputed in `eval/results/evaluation-metrics.json` | Showable; must say **"development set"**. `GET /v1/eval/summary` embeds the unadjusted figures; `eval/RESULTS.md`, `docs/claims-reconciliation.md` and docs/EVALUATION.md use the adjusted ones |
| Calibration, held-out (377) | mean stated confidence 94.7% vs accuracy 90.2%; ECE 0.059 (0.041–0.091) with one bin per emitted confidence value, 0.046 (0.022–0.074) with 10 equal-width bins; Brier 0.081 (0.059–0.105) against 0.089 for a constant forecast. Bootstrap 95% intervals, 2,000 resamples, seed 20261017 | `eval/results/evaluation-metrics.json`, built by `python3 -I eval/charts/build_evaluation.py .` from the final Vertex rows | Measured offline, no model calls. Say "overconfident by about 4.5 points on average"; never call the stated confidence a calibrated probability (the calibrated one is the D160 row below) |
| Calibration, development set (463, adjusted) | mean confidence 94.9% vs accuracy 91.1%; ECE 0.050 (0.033–0.078) per emitted value, 0.038 (0.016–0.063) 10-bin; Brier 0.072 (0.051–0.094) against 0.081 constant | same file | Measured offline; must say **"development set"** |
| Split exclusions | rulings where CBP held two or more countries: 29 of 498 in the dev window, 7 of 396 in 2026 (then 12 pilot cases removed: 389 → 377) | `eval/results/evaluation-metrics.json` (`funnel`), reconstructed from `eval/data/labels.jsonl` | Measured; the original cleaning script is not committed, so this is a reconstruction that reproduces the counts exactly |
| Textile rules, held-out (71 rulings, 2026, textile-v1) | 85.9% (76.0–92.2); 95.2% (84.2–98.7) when auto-accepted at confidence ≥ 0.95, on 59.2% of cases. Production prompt textile-v2 scored 86.9% (78.8–92.2) on 99 development rulings. 27 of the 71 are also in the Part 102 split (D210) | `eval/results/textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl`; dev: `eval/results/textile-dev-…-vertex.jsonl` | Official (owner, 18 Oct); the v2 figure says "development set" |
| Pharmaceutical rules, held-out (56 rulings, 2026, pharma-v1) | 89.3% (78.5–95.0); 100% (92.7–100) at confidence ≥ 0.95, on 87.5% of cases | `eval/results/pharma-final-gemini-4-argon-prec-pharma-v1-litellm.jsonl` | Official (owner, 18 Oct). LiteLLM proxy run, not re-run on Vertex |
| Part 102 marking-origin rules (the `usmca_part102` engine; not USMCA qualification), held-out (32 rulings, 2026, usmca-v1) | 78.1% (61.2–89.0); 100% (79.6–100) at confidence ≥ 0.95, on 46.9% of cases | `eval/results/usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl` | Official (owner, 18 Oct). LiteLLM proxy run; small n, always with the interval |
| Router, development window (Jul–Dec 2025, router-v2) | pharmaceutical and USMCA kept out of the substantial-transformation engine 88.4% (75.5–94.9, n=43); textile sent to the textile engine 87.9% (72.7–95.2, n=33); ordinary rulings wrongly sent out of scope 5% (1.4–16.5, n=40) | `eval/results/scope-window-dev-router-v2-vertex.jsonl` | Superseded (D233): router-v2, no longer the production router; most of the 43 are pharmaceutical, so it overstated Part 102 routing. Say **"development set"** and **router-v2** if shown |
| Second held-out split (83 rulings, 28 Sep–13 Nov 2026; D150, D154) | 77/83 = 92.8% (85.1–96.6); fast path 75/83 = 90.4% at 97.3% (90.8–99.3); $0.0098 per case (gateway-reported); p50 7.5 s / p95 12.3 s | `eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl`; `eval/results/accuracy-v2-metrics.json` (`heldout.final2`) | Measured, scored once, engine-v1 on Argon through the gateway (not Vertex AI). Always with n and the interval; say it confirms the engine after its cutoff, not that it improved |
| Both held-out splits pooled (460) | 417/460 = 90.7% (87.6–93.0); fast path 375/460 = 81.5% at 96.3% (93.8–97.8) | `eval/results/accuracy-v2-metrics.json` (`heldout.pooled_final_final2`) | Measured; mixes the Vertex run (377) and the gateway run (83). Say so wherever it is shown |
| All regimes, first held-out window, pooled | 476/536 = 88.8% (85.9–91.2) adds the four splits: substantial transformation 377, textile 71, pharmaceutical 56, Part 102 32. It counts 27 rulings twice (in both the textile and the Part 102 split; D210). Each ruling once: 451/509 = 88.6% (85.6–91.1) | `eval/results/accuracy-v2-metrics.json` (`heldout_all_regimes`, `heldout_all_regimes_by_window`); `claimscheck` (`regimes.w1.*`) | Measured; pharmaceutical and Part 102 are proxy runs. Prefer the each-ruling-once figure |
| Other regimes, post-cutoff held-out (28 Sep–13 Nov 2026; D210) | textile 15/17 = 88.2% (65.7–96.7), textile-v2; pharmaceutical 4/6 = 66.7% (30.0–90.3), pharma-v1; Part 102 marking 11/13 = 84.6% (57.8–95.7), usmca-v1, with only the 15 window-2 rulings as precedents (D612). $0.41 for the three runs | `eval/results/{textile,pharma,usmca}-final2-*-router.jsonl`; splits `eval/splits/splits_{textile,pharma,usmca}_final2.json`; `claimscheck` (`regimes.w2.*`); served as `textile_final2_accuracy`, `pharma_final2_accuracy`, `usmca_final2_accuracy` | Measured, frozen first, scored once, gateway (not Vertex AI). Small: always with n and the interval. Pharmaceutical is under 10; do not read anything into it. Each regime's own engine (the best case); router-v2 sent 10 of the 24 rulings to the general engine. The routed path is scored in the D233 row |
| Router, post-cutoff (D211) | pharmaceutical and Part 102 rulings kept out of the substantial-transformation engine: 11/19 = 57.9% (36.3–76.9), against 88.4% on the development window | `router_regime` on `eval/results/{pharma,usmca}-final2-*-router.jsonl`; served as `router_final2_out_of_scope_recall` | Measured once, gateway, router-v2 on Gemini 3.8 Flash. Small. The router it describes was replaced (D233); shown on /accuracy as the router before |
| Router routing, router dev split (before 2026, 725 rulings with a gold engine; D230–D232) | router-v2 677/725 = 93.4% (91.3–95.0), Part 102 36/70; router-v3 695/725 = 95.9% (general 528/547, failed the 1-point limit); router-v4 697/725 = 96.1% (94.5–97.3), specialist 163/178 = 91.6%, Part 102 57/70, general 534/547 = 97.6%; router-v4 vs v2 exact McNemar p = 0.0005 | `eval/results/route-dev-router-{v2,v3,v4}-gemini-3.8-flash-litellm.jsonl`; splits `eval/splits/splits_router.json`; `claimscheck` (`router.dev.*`); served as `router_dev_routing`, `router_dev_specialist_routing` (router-v4) | Showable; must say **"development set"**. Gateway, not Vertex AI |
| Router routing, held-out (D233) | Jan–Sep 2026, 505: router-v4 490 = 97.0% (95.2–98.2), specialist 125/128 = 97.7% (93.3–99.2); router-v2 488 = 96.6%. 28 Sep–13 Nov 2026, 105: router-v4 101 = 96.2% (90.6–98.5), specialist 19/22 = 86.4% (66.7–95.3); router-v2 96 = 91.4%, specialist 13/22 | `eval/results/route-final{,2}-router-v4-gemini-3.8-flash-litellm.jsonl`, `route-final-router-v2-gemini-3.8-flash-litellm.jsonl`, `route-final2-router-v2-d211.jsonl`; `claimscheck` (`router.final.*`, `router.final2.*`); served as `router_final_routing`, `router_final_specialist_routing`, `router_final2_routing`, `router_final2_specialist_routing` | Measured, scored once per router, gateway. router-v2 window-1 labels mix Vertex AI (377) and gateway (132). Small post-cutoff specialist n: always with n and interval. **The "trade programs" cue came from post-cutoff error analysis (1/70 dev Part 102 rulings use it); final2 is not clean held-out evidence for router-v3/v4. Excluding the 6 cue rulings: specialist 14/17 vs 12/17 (fixed 2, broke 0), recomputed as `router.final2.nocue.*`** |
| Routed path, all regimes, post-cutoff (107 rulings, each once; D231, D233) | router-v4 95/107 = 88.8% (81.4–93.5); router-v2 94/107 = 87.9% (80.3–92.8); each ruling to its own engine 96/107 = 89.7% (82.5–94.2) | `eval routed` over `eval/results/route-final2-*.jsonl` + the D211 rows + `eval/results/*-final2-*-routed-v{2,4}.jsonl`; `claimscheck` (`router.w2.routed.*`); served as `all_regimes_final2_routed_accuracy`, `all_regimes_final2_routed_accuracy_router_v2` | Measured, gateway, each (ruling, engine) pair run once. The product's real pooled figure on these rulings. The two routers differ on 3 rulings; not a significant difference. **Not clean held-out evidence for router-v3/v4: the "trade programs" cue came from reading this window's misses. Without the 6 cue rulings, 90/101 (v4) vs 91/101 (v2), recomputed as `router.w2.routed.nocue.*`** |
| All regimes, all held-out windows (Jan–Nov 2026; D210) | each ruling once: **547/616 = 88.8% (86.1–91.1)**; second window alone 96/107 = 89.7% (82.5–94.2). Splits added: 583/655 = 89.0% (86.4–91.2) | `eval/results/accuracy-v2-metrics.json` (`heldout_all_regimes_by_window`); `claimscheck` (`regimes.all.*`); served as `all_regimes_heldout_accuracy` and shown on /accuracy | Measured; pools Vertex AI, proxy and gateway runs, one run each; intervals do not include run-to-run or label uncertainty. Say "each ruling counted once" and "mixed paths" wherever shown |
| Model sweep, development set (463 adjusted; D153) | Argon 91.1% on Vertex, 91.8% through the gateway (p = 0.58); Carbon 88.1% (27 failed calls); 3.1 Pro preview 87.3% (10 failed calls); majority of three 91.6% | `eval/results/accuracy-v2-metrics.json` (`dev`) | Measured; must say **"development set"**; gateway runs |
| Calibration layer (isotonic, fitted on dev; D156) | held-out 377: ECE 0.059 → 0.030 (0.020–0.059; 11 → 6 bins, stated on those bins 0.0528, ~20% of the drop), Brier 0.081 → 0.072 (0.056–0.092), mean stated 94.7% → 90.4% (accuracy 90.2%); second split: ECE 0.067 → 0.038 (9 → 6; same-bin 0.0614, ~18%), Brier 0.060 → 0.051; pooled 460: ECE 0.055 → 0.027 (11 → 6; same-bin 0.0500, ~17%) | `eval/results/accuracy-v2-metrics.json` (`calibration_layer_engine_v1`, `heldout.final2.calibration_layer`, `heldout.pooled_final_final2.calibration_layer`) | Measured offline. cal-v1, eval only; the product uses cal-v2 (next row) |
| Calibrated confidence in the product (cal-v2, fitted on dev; D160) | held-out 377: mean stated 94.7% → calibrated 90.2% (accuracy 90.2%), ECE 0.059 → 0.028 (0.020–0.060; 11 → 6 bins, stated on those bins 0.0528, ~19%), Brier 0.081 → 0.072 (0.056–0.092); final2 (83): 96.1% → 93.8% (accuracy 92.8%), ECE 0.067 → 0.037 (0.030–0.099; 9 → 6, same-bin 0.0614, ~18%), Brier 0.060 → 0.051; both (460): ECE 0.055 → 0.026 (0.018–0.053; 11 → 6, same-bin 0.0500, ~16%) | `eval/results/accuracy-v2-metrics.json` (`calibration_v2`); map `backend/internal/calibrate/map.json`; 377 figures also recomputed in Go by `go run ./cmd/claimscheck` (`final.cal.*`) and served by `GET /v1/eval/summary` (`final_calibration_*`, including `final_calibration_ece_stated_same_bins`) | Built: the API returns `calibrated_confidence` for engine-v1 on Gemini 4 Argon; the Result screens and MCP tools show it. The ECE pairs use different bins; /accuracy shows the same-bin stated figure beside them. Other engines and the fallback model: stated confidence only |
| Calibrated auto-accept bar (D160) | 0.90 on calibrated confidence: the lowest map step with in-scope dev accuracy ≥ 96% on ≥ 100 cases (dev 368/463 = 79.5% at 96.5%). It accepts exactly the cases stated ≥ 0.92 did, so the held-out fast-path figures are unchanged (377: 300 at 96.0%; final2: 75 at 97.3%) | `eval/results/accuracy-v2-metrics.json` (`calibration_v2.dev_curve`, `.final.at_bar`, `.final2.at_bar`); `config.DefaultCalibratedFastPath` | Built; `CALIBRATED_FAST_PATH` default, `CALIBRATION=off` restores the stated 0.92 bar |
| China-cluster adjudicator, development set (463; D163, D164) — not adopted | against engine-v1 on Vertex AI (91.1%): narrow 90.1% (fixed 2, broke 7, p = 0.18); symmetric 90.9% (9 / 10, p = 1.0); symmetric keeping only moves away from China, chosen after the fact, 92.0% (7 / 3, p = 0.34). Cost 1.24× to 1.26× per case. None met the pre-registered rule; no held-out split scored | `eval/results/adjudicator-metrics.json` (`python3 -I eval/charts/build_adjudicator.py .`); rows `eval/results/dev-…+adjudicate-v1-*.jsonl` | Measured; must say **"development set"** and "not adopted". A negative result |
| Component checklist for the general engine, development set (463; D650 to D653) — not adopted | against engine-v1 on the same gateway path (91.8%; China-held 66 of 78, other rulings 359 of 385; $0.0079 per case; p50 / p95 6.8 s / 10.8 s): st-checklist-v1 91.1% (88.2–93.4), fixed 5, broke 8, p = 0.58, China-held 67, other 355, $0.0109 (1.38 ×), 9.5 s / 15.4 s; st-checklist-v2 91.4% (88.4–93.6), fixed 5, broke 7, p = 0.77, China-held 66, other 357, $0.0119 (1.51 ×), 10.2 s / 20.4 s. Both failed the pre-registered rule; no held-out split scored | `eval/results/st-checklist-metrics.json` (`python3 -I eval/charts/build_st_checklist.py .`); rows `eval/results/dev-gemini-4-argon-prec-st-checklist-v1-litellm-d650.jsonl`, `...-v2-litellm-d650.jsonl` | Measured; gateway; must say **"development set"** and "not adopted". A negative result |
| Rule baselines, recomputed (no model; D155) | first held-out split: first country mentioned 63.4% (58.4–68.1), most-mentioned 60.7% (55.7–65.5), last 23.6%, majority (Vietnam) 12.2%; second split: most-mentioned 71.1% (60.6–79.7), first 60.2% | `eval/results/baselines-final.json`, `eval/results/baselines-final2.json` (`go run ./cmd/eval baselines`) | Measured. The best simple rule on the 377 is the first country mentioned, 63.4% |
| Cost per origin prediction | $0.0078 (promo price) / $0.0157 (list), Vertex AI held-out | final Vertex rows × Gemini Developer API prices verified 2026-10-14 | Token counts are Vertex. The Vertex price sheet was not re-checked; this is the same promo/list schedule as the proxy runs |
| Cost per verified move | $0.031–0.032 (promo) | verifier dev rows | Proxy |
| Full plan, heat gun | $0.22 promo / $0.45 list, 33 s, 7 engine calls + 6 verifier runs | `eval/results/plan-heatgun-2022-local-litellm.json` | Proxy; demo fixture. An earlier run of 6 moves, before three-group moves (D84); it showed motor-fan + PCBA at 0.95, not the production 0.96 plan. /method says so beside the figure (D310) |
| Latency per prediction | p50 4.6 s / p95 5.7 s | final Vertex rows | Official Vertex AI. The proxy run was p50 5.0 s / p95 6.4 s |

### Repeatability (D190)

| Number | Value | Source | Status |
|---|---|---|---|
| Development accuracy over three runs (463, adjusted) | 91.8%, 91.1%, 91.1%; mean 91.3%, spread 0.7 points | `eval/results/repeatability-dev-litellm.json`; recomputed by `claimscheck` (`repeat.*`) | Measured, development set, gateway (not Vertex AI). Say "development set" |
| Rulings whose answer changes between runs | 4 of 463 (0.9%) | same | Measured |
| Fast path coverage over three runs | 81.0%, 81.4%, 80.6% (fixed scope file) at 95.5-95.7%; 8 rulings move in or out | same | Measured, development set |
| Calibrated confidence over three runs | mean 91.2%, 91.2%, 91.0%; ECE 0.022, 0.020, 0.017; changes on 50 rulings, 0.005 on average | same | Measured, development set |

### Plan check run again (D321, D322)

| Number | Value | Source | Status |
|---|---|---|---|
| Heat-gun plan, three runs, production verifier settings | a plan shown in 3 of 3 runs; the same 3 moves each time; the three-group move checked 0.95, 0.96, 0.96; motor-fan + PCBA 0.92, 0.92, 0.94; motor-fan + switch, cord and minor parts 0.92 three times; motor-fan alone 0.90, 0.91, 0.91 (not shown) | `eval/results/plan-stability-heatgun.json` (`d321`), rows `eval/results/plan-heatgun-2022-relaxed-verifier-v2-stability-run{1,2,3}-litellm.json`; served as `plan_stability_heatgun_*` | Measured, pre-registered (D321), gateway, demonstration not test. Two shown moves sit exactly on the 0.92 bar |
| Earlier recorded heat-gun plan runs (15–18 Oct) | 13 runs with older settings; a plan shown in 6; best checks 0.75 to 0.96 | same file (`recorded_before_d321`, `runs`) | Recorded; settings differ run to run, so not a stability measure |
| Cost of the three runs | $1.32 gateway-reported | same file (`d321.gateway_cost_usd`) | Measured |

### Plan after the split (D330, D331, D332)

| Number | Value | Source | Status |
|---|---|---|---|
| Heat-gun moves searched after the split | 10 singles and pairs plus 2 of 4 triples (12, `PLAN_MAX_MOVES`); CBP's move (motor-fan, PCBA, power cord, knobs and covers) is the 12th | `backend/internal/planner/split_switch_test.go`, `planner_core_test.go` | Offline, built |
| CBP's own 2026 move, two runs, production verifier settings | shown in 2 of 2 runs; check 0.94 both times, supporting N347421; ranked 5 of 7 and 4 of 6 by net a year (6 of 7 and 1 of 6 by tier, D341) | `eval/results/plan-stability-heatgun.json` (`d331`), rows `eval/results/plan-heatgun-2022-relaxed-verifier-v2-split-run{1,2}-litellm.json` | Measured, 2 runs. Not a blind test: the split was made after seeing N357771 (D330) |
| The four-group move in the same runs | check 0.96 both times, followed from near-miss advice; net $285k/yr, ranked last | same files | Measured, 2 runs |
| Top-ranked plan after the split | run 1 motor-fan alone, moves 2 of the 6 changed parts; run 2 motor-fan + power cord, knobs and covers, moves 5 of 6 and nothing CBP kept | same files (`backtest`) | Measured, 2 runs. The product's backtest reads this plan, so it did not get closer to CBP's change |
| Cost of the two runs | $1.78 gateway-reported | same file (`d331.gateway_cost_usd`) | Measured |
| Heat-gun group costs after the split | switch, wiring and minor parts $40k China / $60k Thailand a year; power cord, knobs and covers $30k / $45k (was one group, $70k / $105k) | `backend/testdata/heatgun_2022.json`, `backend/internal/demos/fixtures/heatgun_2022.json` (`group_costs`) | Illustrative, editable; the split is not sourced |

### Plan ranking by tier (D340, D341)

Computed by `python3 -I -B eval/planner/plan_rank.py .` (`--check` in `make reproduce`). No model calls.

| Number | Value | Source | Status |
|---|---|---|---|
| Clear margin | 0.01: the clear-margin tier starts at a check of 0.93, with no counter-ruling | `eval/results/plan-rank-analysis.json` (`dev.chosen_margin`, `dev.tier1_from`); `planner.DefaultClearMargin` | Chosen on development rows only by the D340 rule; the need was noticed on the heat gun |
| Development verdicts that held on another run | clear-margin tier 644 of 644 (100%); every shown verdict 906 of 994 (91.1%); at the bar 262 of 350 (74.9%) | same file (`dev.by_margin`, `dev.tier2_at_chosen`) | Measured offline, eight known-a dev runs with differing settings |
| Committed plan runs re-ranked | 44 runs, 30 with a shown plan, 13 with two or more; the top plan changes in 6; in 2 of those 6 the new top backtests worse (D332 run 1, the post-cutoff stove heater) | same file (`totals`, `rerank`) | Offline re-rank of committed rows; not a test |
| CBP's heat-gun move after the split, ranked by tier | 6th of 7 in run 1 (its check named counter-ruling N345859), 1st of 6 in run 2 | same file (`rerank`, `cbp_move`) | Offline re-rank of D332's rows; the rule was set after seeing them |

### Repeatability on held-out (D240, D241)

| Number | Value | Source | Status |
|---|---|---|---|
| First held-out split (377): official run and two repeats | 90.2% official (Vertex AI); repeats 89.9% and 90.2%, mean 90.05%; official 0.15 points above the repeat mean; spread 0.3 points | `eval/results/repeatability-heldout-litellm.json`; recomputed by `claimscheck` (`repeat_heldout.final.*`) | Measured. Repeats through the gateway, not Vertex AI; the official run predates the fixed seed |
| First held-out split: answer changes | 7 of 377 (1.9%) over the three runs; 2 between the two repeats | same | Measured |
| First held-out split: fast path over three runs | coverage 79.6%, 78.8%, 77.7% at 96.0%, 96.0%, 96.6% (fixed Vertex scope file) | same | Measured |
| First held-out split: calibrated ECE over three runs | 0.028, 0.030, 0.028 | same | Measured |
| Second held-out split (83): official run and two repeats | 92.8% official (D154); repeats 92.8% and 95.2%, mean 94.0%; official 1.2 points below the repeat mean; spread 2.4 points | same (`repeat_heldout.final2.*`) | Measured, gateway, small n |
| Second held-out split: answer changes | 2 of 83 over the three runs | same | Measured |
| Second held-out split: fast path over three runs | coverage 90.4%, 92.8%, 89.2% at 97.3%, 96.1%, 98.6% (each run's own router call) | same | Measured, small n |
| Second held-out split: calibrated ECE over three runs | 0.037, 0.040, 0.066 | same | Measured, small n |
| Cost of the four repeat runs | $7.47 gateway-reported, with resumes | same (`gateway_cost_usd`) | Measured |

### Rule check, 19 CFR 102.20 (D360-D362, D460, D461, D560)

Recomputed by `cd backend && go run ./cmd/tariffshift eval -check` (in `make reproduce`) from the committed extractions; no model calls at scoring.

| Number | Value | Source | Status |
|---|---|---|---|
| Rule rows encoded | 446 rows of 19 CFR 102.20 in chapters 15, 21, 39, 40, 44, 48, 71, 73, 76, 82, 83, 84, 85, 87, 90, 94, 95; 408 of 633 alternatives encoded, 225 written by goods description or with a content proviso and never decided by code | `backend/internal/tariffshift/rules.json` (eCFR, 18 Nov 2026) | Built; a test re-parses the committed text. Before D460: 360 rows in 7 chapters, 332 of 515 (unchanged by the D412 fix) |
| Rows and alternatives added by D460 | 86 rows, 76 of 118 alternatives encoded: 15: 6 of 7, 21: 10 of 16, 39: 3 of 7, 40: 13 of 14, 44: 2 of 10, 48: 11 of 20, 71: 12 of 13, 76: 4 of 4, 83: 9 of 9, 95: 6 of 18 | same | Built |
| Goods' chapters behind the chapter choice (D560, corrects D460) | 153 development and earlier Part 102 goods with a code (69 development, 84 of 239 labelled Part 102 rulings dated before 2024 and in no dev or held-out split), 75 in the 7 chapters; picked: every other chapter with at least 4 (44: 11, 39: 9, 48: 8, 83: 5, 76: 4). Kept from D460, not picked by this rule: 15, 21, 40, 71, 95 | `eval/tariffshift/chapter-counts.json` (`python3 -I -B eval/tariffshift/chapter_counts.py . --check`) | Measured; no held-out rows read. The D460 count (195 goods) read the 72 development rulings twice and the 32 held-out `final` rulings (#619) |
| Held-out rule-check figures without the five chapters kept from D460 (D560) | full text final 4 of 32 (4 agree; out of scope 12); facts final 1 of 32, final2 0 of 13; CBP's good code with top part suggestions final 12 of 29 (11 agree), top suggestion for everything 13 (12 agree), CBP's stated codes 4 (4); without the inputs that carried a code 9 of 24 (8 agree); same answer under every suggested code 11 of 12 (suggested good: 11 of 13), without those inputs 8 of 9 and 7 of 9. Decisions that go: N357770 (95), N361217 (15), both agreed with CBP | `eval/tariffshift/d560/summary.json`, `eval/tariffshift/d560/parts/summary.json` (`go run ./cmd/tariffshift eval -without 15,21,40,71,95 -out ../eval/tariffshift/d560 -check`, same for `parts-eval`) | Measured offline, no model calls, computed once; the five chapters stay encoded in the product |
| Coverage from the facts the engine reads | 1 of 117 Part 102 rulings decided (dev 0/72, prompt v1; final 1/32, H355020, every material Canadian, agrees; final2 0/13); 68 of 117 give no code for the good | `eval/tariffshift/summary.json` | Measured; the honest headline for the product. Say "decides almost none". 7 chapters (D362): 0 of 117 |
| Coverage from the full ruling text (CBP's own codes) | dev 8 of 72 (8 agree with CBP, 67.6-100%), final 4 of 32 (4 agree, 51.0-100%); differs from the model on 2 dev rulings, agreeing with CBP on both (N341152, N352059) | same | Encoding check, not product accuracy. Always with n; never as an accuracy figure. 7 chapters (D362): dev 2 of 72, final 2 of 32, all agree; same before and after the D412 fix. Without the five chapters kept from D460 (D560): final still 4 of 32 |
| Extraction cost | $2.58 gateway-reported, seven runs | `eval/results/RUNLOG.jsonl` (`tariffshift-extract-*`) | Measured |

### Canada marking rules, SOR/94-23 (D720-D725)

Recomputed by `cd backend && go test ./internal/tariffshift -run 'RulesCAJSON|Parity'` (in `make reproduce`); no model calls. A rules check only: no accuracy figure exists (D725).

| Number | Value | Source | Status |
|---|---|---|---|
| Rule rows and alternatives parsed | 572 rows (569 code rows, whole-chapter rows for 61 and 62, a note-only row for 63.09); 548 of 596 alternatives encoded (91.9%); 544 rows fully, 4 partly, 24 not; 40 written by goods description and 8 with a content or process proviso, never decided by code | `backend/internal/tariffshift/rules_ca.json` (Justice Laws XML, retrieved 22 Nov 2026), `TestRulesCAJSONMatchesParser` | Built; a test re-parses the committed text |
| Alternatives allowing a change within the good's own subheading or heading (s. 12(2)) | 102 (88 encoded) | same | Built; a difference from 19 CFR 102.20 (D722) |
| Rows whose note can set origin by a process (D723) | 57 | same | Built |
| Parity with the U.S. check on identical rows | 116 rows with the same text; same answer on 2,094 of 2,094 inputs (1,029 decided by the U.S. rules) | `TestParityOnIdenticalRows` | Built, synthetic inputs; not an accuracy figure |
| Published Canadian rulings to test against | about 11 origin and 3 marking rulings, none negative (CBSA list archived 2 Jul 2025) | day-0 check for #663, D725 | Too few for an accuracy figure; none shown |

### EU and UK non-preferential origin list rules (D760-D766)

Recomputed by `cd backend && go test ./internal/euorigin ./internal/ukorigin -run 'JSON|Coverage|Parity'` (in `make reproduce`); no model calls. Rules checks only: no accuracy figure exists (D766).

| Number | Value | Source | Status |
|---|---|---|---|
| EU chapters, rows and alternatives | 37 chapters; 446 rows; 542 alternatives: 199 decided from codes and shares, 338 process rules decided only from an answer, 5 not read; rows: 182 by code alone, 259 with a process rule, 5 with an unread rule | `backend/internal/euorigin/rules_eu.json` (CELEX 02015R2446-20261101, retrieved 24 Nov 2026), `TestCoverageEU` | Built; a test re-parses the committed text |
| UK chapters, rows and alternatives | 37 chapters; 452 rows; 552 alternatives: 205 from codes and shares, 342 process, 5 not read; rows: 184, 263, 5 | `backend/internal/ukorigin/rules_uk.json` (reference document v1.2, retrieved 24 Nov 2026), `TestCoverageUK` | Built; a test re-parses the committed text |
| EU and UK parity on rows with the same text | 444 rows with the same text; 441 compared (3 value rules skipped); same status and origin on 1,764 of 1,764 synthetic inputs per market | `TestParity` | Built, synthetic inputs; not an accuracy figure |
| Published origin decisions with facts to test against | EU: none (the public BOI list has outcomes only); UK: none (HMRC publishes no Advance Origin Rulings) | D766 | No accuracy figure; none shown |

### Per-part HTS suggestions (D380-D382)

Recomputed by `cd backend && go run ./cmd/tariffshift parts-eval -check` (in `make reproduce`) from the committed rows; no model calls at scoring.

| Number | Value | Source | Status |
|---|---|---|---|
| Part index | 1,159 classifying sentences in 562 of 3,112 corpus rulings | `backend/data/part_hts_index.json` (`go run ./cmd/corpus part-hts-index`) | Built |
| Limits per request | up to 20 parts, up to 3 codes each, one model call, answers cached a day (at most 2,000) | `backend/internal/hts/parts.go` | Built |
| Parts with a suggestion | dev 389 of 393 (Part 102), 200 of 200 (families); final 225 of 245 | `eval/tariffshift/parts/summary.json` | Measured |
| Top suggestion matches CBP's stated part code | dev 20 of 27 (55.3-86.8%), top 3: 22 of 27; families 2 of 2; final (held out) 2 of 3 (20.8-93.9%) | same | Measured, small n; always with n. Post hoc, 4 dev gold codes look wrong; without them 19 of 23 |
| Good's code, top suggestion matches CBP's | dev 36 of 68, final 14 of 27 | same | Measured |
| Rule check decided with suggested part codes (CBP's good code) | dev 16 of 69 (16 agree with CBP), final 14 of 29 (13 agree; N358639 differs); with CBP's stated codes only: 8 and 4. Suggested good code too: dev 17 (16 agree), final 15 (14 agree) | same | Measured. Not product accuracy. Give per split, never pooled (D413). 7 chapters (D382): dev 9 (9), final 7 (6); stated codes 2 and 2; suggested good 10 (9) and 8 (7). Held out without the five chapters kept from D460 (D560): 12 (11) and 13 (12) |
| Inputs that carried an HTS number past the v1 stripper (#576) | final 5 of 29 (N363600 with the importer's full code), dev 4 of 69 | `eval/tariffshift/parts/summary.json` (`input_codes`) | Measured, post-freeze correction (D413) |
| Held-out figures without those inputs | every code suggested: 11 of 24 decided, 10 agree; CBP's good code: 11 of 24, 10 agree; good top-1 / top-3: 12 / 12 of 23 (14 / 15 of 27 as run) | same (`without_input_codes`) | Measured, post-freeze correction (D413); not re-run. 7 chapters: 7 of 24, 6 agree, both rows. Without the five chapters kept from D460 (D560): 9 of 24, 8 agree |
| Rule-check decisions that hold under every suggested part code (D430) | CBP's good code and top part suggestions: dev 15 of 16 decided (15 agree with CBP), final 13 of 14 (13 agree; the one disagreeing, N358639, depends on a part's code). Suggested good code: dev 16 of 17 (15 agree), final 13 of 15 (13 agree). Without the inputs that carried a code: dev 13 of 14 and 15 of 16, final 10 of 11 and 9 of 11. 7 chapters: dev 8 of 9, final 6 of 7; suggested good 9 of 10 and 6 of 8. Held out without the five chapters kept from D460 (D560): 11 of 12 and 11 of 13 | `eval/tariffshift/parts/summary.json` (`robustness`) | Measured offline, no model calls, committed suggestions; added after the freeze. Each part's top-3 codes; past 81 combinations a deterministic sample of 81 (4 dev, 3 final decided rulings) |
| Cost per product | $0.0103 (dev Part 102), $0.0139 (families), $0.0123 (final); $1.42 for all runs | same; `eval/results/RUNLOG.jsonl` (`parts-hts-*`) | Measured, gateway-reported |
| Codes the plausibility check marks "check this code" (D471) | dev 0 of 1,060 (680 Part 102, 380 families), after changes made on dev (its first version marked 7 dev codes, all plausible on reading); final (held out, measured once) 5 of 424, all in N359001 ("nuts" as 1704.90.10 and 2008.11.60, "temperature transmitters" as 2106.90), none on a part with a gold code, so 0 checked against gold either way; wrong dev codes it leaves unmarked: 15 of 15 | `eval/parts-check/summary.json` (`go test ./internal/hts`) | Measured offline on the committed suggestion rows; no model calls. A flag, not a filter: no code is removed. Do not present it as an accuracy gain |

### The rule check on each plan move (D440-D442)

Recomputed by `cd backend && go run ./cmd/tariffshift plans-eval -check` (in `make reproduce`) from committed plan runs, fixtures and the development suggestions; no model calls.

| Number | Value | Source | Status |
|---|---|---|---|
| Committed plan products with every code as committed; Part 102 plan searches | 0 of 43; 0 | `eval/tariffshift/plans/summary.json` | Measured. The reason the count below uses suggested codes |
| Per-family plan moves decided (development suggestions) | 25 of 57 moves (22 shift to the target, 3 give another country), in 14 of 23 searches | same | Measured, development and train fixtures. Informational: every search is decided under substantial transformation. 9 of the 22 shifts are moves that put every part in the target. Unchanged with the 17 chapters of D460 |
| Agreement with the precedent check | 21 of 23 decided moves with a check; 2 flagged (precedent supports, rule gives China, N354220, both below the bar); shown plans 15 of 19 decided, all 15 shift | same; `eval/tariffshift/plans/moves-dev.jsonl` | Measured. Not accuracy: Part 102 is not the test for these cases |
| Current setup | 11 of 23 decided; 4 give CBP's held origin | same | Measured. The difference between the two tests, not an error rate |
| Moves compared with a later CBP ruling | 0 | same | No committed move has both codes and a later ruling |

### Scored Part 102 plan searches (D501, D502)

Three development rulings, one plan search each through an OpenAI-compatible LiteLLM gateway to Gemini (no cache), scored offline by `cd backend && go run ./cmd/tariffshift plans102-eval -check` (in `make reproduce`). Part codes are unverified suggestions; plant capabilities and money are illustrative.

| Number | Value | Source | Status |
|---|---|---|---|
| Part 102 plan searches with codes | 3 (N348465 to Canada, N344002 and N350133 to China) | `eval/tariffshift/plans102/summary.json` | Measured, development. Picked to exercise the rule (D501), not a sample |
| Actuator (N348465) moves the rule says shift; shown | 6 of 12 (every move with the motor); 1 shown, and the rule says it shifts | same; `eval/tariffshift/plans102/moves-dev.jsonl` | Measured. Precedent supports all 6; 5 are below the bar, including the motor alone at 0.82 |
| Moves precedent supports while the rule gives another country | 2 (the compressor and lawn-mower moves to China, both below the bar) | same | Measured. Flagged; neither shown, so the D500 order had nothing to act on |
| D500 order changes on these searches | 0 | same | Measured. No search showed more than one plan |
| Current setup: rule gives CBP's holding | 2 of 3 (the actuator is undetermined: the motor does not shift unless de minimis) | same | Measured |
| Later ruling, rule on its setup (N354024) | Mexico, as CBP held | same | Measured. A later product, not a plan move: no plan backtest (0) |
| Cost | $0.4333 for the 3 searches (actuator $0.3542, 79 s) | same; `eval/results/RUNLOG.jsonl` (`planner-part102-d501-litellm`) | Measured, gateway-reported |
| Engine predicts the target on actuator moves the rule says do not shift | 3 (gearbox and controls 0.85; gearbox 0.75; gearbox and housings 0.75) | `eval/tariffshift/plans102/moves-dev.jsonl`; on the recorded page as `engine_origin_d501` in `frontend/src/lib/part102-plan-example.json` (`go run ./cmd/tariffshift plans102-example -check`; D532, D563) | Measured, from the same run. The recorded page shows each |

### Part 102 plan searches with the Part 102 engine (D540, D541)

The same three searches run again with the Part 102 engine predicting each move, scored by `cd backend && go run ./cmd/tariffshift plans102-eval -series d540 -check` (in `make reproduce`). Seen development products, one run each: a re-run, not new evidence of accuracy.

| Number | Value | Source | Status |
|---|---|---|---|
| Engine agrees with the 102.20 rule, decided moves | 7 of 8 (general engine, D502: 6 of 8) | `eval/tariffshift/plans102/summary-d540.json`; `summary.json` for D502 | Measured, development |
| Engine agrees with the rule's lean, moves undetermined only for de minimis | 6 of 6, all on the actuator (D502: 3 of 6) | same; `moves-dev-d540.jsonl` | Measured, development |
| Moves sent to the precedent check | 7 (D502: 11) | same | Measured |
| Actuator plans shown | 2, both moves the rule says shift (D502: 1) | same | Measured. The precedent check varies run to run |
| Moves precedent supports while the rule gives another country | 1, the compressors (D502: 2) | same | Measured. The Part 102 engine still predicts China there |
| Cost | $0.3416 for the 3 searches (actuator $0.2770, 37 s); $0.1724 more for an invalid first attempt with no local corpus, not scored | same; `eval/results/RUNLOG.jsonl` (`planner-part102-d540-litellm`, `planner-part102-d540-nocorpus-litellm`) | Measured, gateway-reported |
| Recorded actuator page: Part 102 engine agrees with the rule (D563, re-recorded in D631) | `usmca-v5` (D630 run): 12 of 12 moves (6 the rule decides, 6 open only on de minimis); `usmca-v1` in the D541 run: 12 of 12; the general engine in the D501 run: 9 of 12. 2 shown plans, both moves the rule says shift (not the same two as in the D541 run). Search $0.2923, 47 s | `frontend/src/lib/part102-plan-example.json` (`engine_agrees_with_rule`, `engine_agrees_with_rule_d541`, `engine_agrees_with_rule_d501`) from `go run ./cmd/tariffshift plans102-example -check`; `eval/tariffshift/plans102/runs/plan-p102-n348465-local-verifier-v2-d630-litellm.json` | Measured, from the committed D630, D541 and D501 runs; one seen development product, not an accuracy figure |

### Part 102 engine with the 102.20 rule text quoted (D550, D551)

Development only, one run each through an OpenAI-compatible LiteLLM gateway to Gemini, scored by `cd backend && go run ./cmd/tariffshift ground102-eval -check` (in `make reproduce`). Did not pass the D550 rule; not adopted; held out not scored.

| Number | Value | Source | Status |
|---|---|---|---|
| Development Part 102 rulings with a quoted 102.20 row | 29 of 72 | `eval/tariffshift/ground102/summary-d551.json` | Computed, no model |
| Correct, usmca-v1 re-run (B) | 53 of 72 | same; `eval/tariffshift/ground102/runs/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm-d550.jsonl` | Measured, development |
| Correct, usmca-v2 (V) | 50 of 72 | same; `runs/usmca-dev-gemini-4-argon-prec-usmca-v2-litellm-d550.jsonl` | Measured, development. Negative result |
| Discordant rulings, exact McNemar | B only 5, V only 2, p = 0.45 | same | Measured |
| Contradicts the 102.20 check, of the 10 it decides | B 2, V 2 (N341152, N352059) | same; `eval/tariffshift/results-dev-full_text.jsonl` | Measured |
| Cost | B $0.6881, V $0.7114 (1.03 ×); $1.3995 in all | same; `eval/results/RUNLOG.jsonl` (`usmca-dev-gemini-4-argon-prec-usmca-v{1,2}-litellm-d550`) | Measured, gateway-reported |

### Held-out Part 102 plan set (D570, D571)

The first held-out evaluation of the Part 102 plan pipeline: held-out Part 102 rulings frozen by a fixed rule before any run, then the production family check, fixtures built by fixed rules and one plan search each with the Part 102 engine. Scored by `cd backend && go run ./cmd/tariffshift plans102-eval -series heldout -check`; the set by `python3 -I eval/planner/part102_heldout.py freeze . --check` (both in `make reproduce`).

| Number | Value | Source | Status |
|---|---|---|---|
| Held-out Part 102 rulings considered; frozen | 45 (window 1, 32; window 2, 13); 18, all window 1 | `eval/tariffshift/plans102/heldout/frozen.json` | Measured, held out. Frozen before any run |
| Frozen rulings with a plan template (family check) | 6 of 18 (33%, 95% CI 16–56%): 3 furniture, 3 vehicle parts | `eval/results/families-coverage-final-all-litellm-p102-heldout.jsonl`; `summary-heldout.json` | Measured, held out |
| Plan searches run; failed | 6; 0 (none re-run) | `eval/tariffshift/plans102/summary-heldout.json` | Measured, held out |
| Moves; decided by the 102.20 rule | 6 in 4 searches; 6 (3 shift, 3 another country) | same; `moves-heldout.jsonl` | Measured, held out |
| Part 102 engine agrees with the rule, decided moves | 3 of 6 (95% CI 19–81%) | same | Measured, held out. Moves in a search are not independent |
| Shown plans; the rule says they shift | 1; 1 (N358639, pipes and flange to Mexico) | same | Measured, held out |
| Moves precedent supports while the rule gives another country | 0 | same | Measured, held out |
| Current setup: rule gives CBP's holding; engine gives CBP's holding | 5 of 6 decided (95% CI 44–97%); 5 of 6 (44–97%). N358639 is the miss for both | same | Measured, held out |
| Backtests on a later CBP ruling | 0 (no later ruling names a frozen one) | `frozen.json` (`later_rulings`) | Measured |
| Cost | $0.3016 in all: plan searches $0.2195 (7 to 35 s each), family check $0.0404, group call $0.0245, development group check $0.0172 | `eval/results/RUNLOG.jsonl` (`planner-part102-heldout-litellm`, `families-coverage-final-all-litellm-p102-heldout`, `part102-heldout-groups`, `part102-heldout-groups-dev`) | Measured, gateway-reported |

### Held-out Part 102 plan set, second pass with `usmca-v5` and 20 families (D630, D631)

The same 18 frozen rulings, scored a second time: pre-registered in D630 before any held-out call, the 20-family check of D581, D571's fixtures where the family is unchanged, and one plan search each with the production Part 102 engine (`usmca-v5`, the product's codes in the facts). Scored by `cd backend && go run ./cmd/tariffshift plans102-eval -series heldout2 -check`; fixtures by `python3 -I eval/planner/part102_heldout.py build2 . --check` (both in `make reproduce`). The D571 figures above stand as the first scoring. This pass is a candidate comparison, not new held-out evidence: all 8 planned rulings are window-1 rulings that `usmca-v5` was scored on in D601 before it was adopted (D602), and its current-setup result here is that D601 result (D670).

| Number | Value | Source | Status |
|---|---|---|---|
| Frozen rulings planned end to end | 8 of 18 (44%, 95% CI 25–66%; D571: 6 of 18) | `eval/tariffshift/plans102/summary-heldout2.json` | Measured on held-out rulings, second scoring of the set; a candidate comparison, not new held-out evidence (window-1 rulings scored in D601, D670) |
| Fixtures reused from D571; built new | 4 (N357951, N359145, N359690, N362414, byte for byte); 4 (N358639 now metal products, N360891, N362347, N363381) | `backend/testdata/planner_part102_heldout2/`; `eval/tariffshift/plans102/heldout/groups-f20.jsonl` | Built by the D570 rules |
| Plan searches run; failed | 8; 0 (none re-run) | `summary-heldout2.json` | Measured, held out |
| Moves; decided by the 102.20 rule | 7 in 7 searches; 6 (1 shifts, 5 another country; 1 undetermined, chapter 96 not encoded) | same; `moves-heldout2.jsonl` | Measured, held out |
| Part 102 engine agrees with the rule, decided moves | 6 of 6 (95% CI 61–100%; D571 `usmca-v1`: 3 of 6, 19–81%). On the four fixtures reused from D571: 3 of 3 (D571: 1 of 3) | same (`pooled`, `pooled_same_fixture_as_d571`) | Measured, held out; a candidate comparison (D670). Moves in a search are not independent; 5 of the 6 are moves where the rule keeps CBP's holding |
| Shown plans; the rule says they shift | 1; 1 (N358639, pipes, flange and brackets to Mexico, precedent check 0.95, N353124) | same | Measured, held out. The 0.95 verdict was made with the codes paragraph then also in the precedent check's facts (D640) |
| Moves precedent supports while the rule gives another country; rows marked for review | 0; 0 (D571's run, scored by the same code: 0; 1) | same; `moves-heldout.jsonl` (`review`) | Measured, held out |
| Current setup: rule decides; gives CBP's holding; engine gives CBP's holding | 7 of 8; 6 of 7 (95% CI 49–97%); 6 of 8 (41–93%). Misses: N358639 (both), N363381 (engine United States, CBP Canada) | `summary-heldout2.json` | Measured, held out. The engine figure repeats D601's window-1 rows ruling for ruling, with the same two misses; N363381 is not a new miss (D670) |
| Search wall time, median / 95th percentile | 12.5 s / 40.2 s (D571: 20.0 s / 35.0 s) | same (`wall_seconds`) | Measured, gateway |
| Cost | $0.1883: plan searches $0.1757 ($0.022 a search; D571 $0.037), group calls $0.0126 | `eval/results/RUNLOG.jsonl` (`planner-part102-heldout2-litellm`, `part102-heldout-groups-f20`) | Measured, gateway-reported |

### Part 102 plan templates (D580, D581)

Six templates written from Part 102 marking rulings dated before 2024 and in no development or held-out split (the D560 pool), chosen by counting the pool rulings with no template. Coverage is scope, not accuracy: a match means a template exists.

| Number | Value | Source | Status |
|---|---|---|---|
| Part 102 selection pool; covered with 14 families | 239 rulings; 83 (34.7%), 2 failed checks | `eval/families/part102-pool.json`; `eval/results/families-coverage-train-all-litellm-p102pool-f14.jsonl` | Measured, pool (selection data) |
| Pool rulings with no template, by category | food and drink 53, metal products 13, plastic articles 11, wood articles 11, chemicals and pharmaceuticals 8, paper 7, machinery parts 7, electrical apparatus 7 | `eval/results/families102-selection.json` | Measured, pool. Model-labelled categories |
| Citations checked; supported | 151 first pass, 81 supported; 23 re-checked after 3 merges, 6 supported; 78 citations of 56 rulings kept | `eval/results/families102-citation-checks.jsonl`; templates | Built |
| Part 102 development (72) with a plan template | 49 (68.1%, 95% CI 56.6–77.7) with 20 families against 20 (27.8%, 18.8–39.0) with 14; on the 71 both answered 48 against 20 (28 gained, 0 lost) | `eval/results/families-coverage-dev-all-litellm-p102-f20.jsonl`, `…-p102-f14.jsonl` | Measured, development |
| General development (469) with 20 families | 317 of 465 answered (68.2%) against 265 of 469 with 14; on the 456 both answered 309 against 264 (48 gained, 3 lost, 9 changed family) | `eval/results/families-coverage-dev-all-litellm-f20.jsonl`, `…-f14-cap2048.jsonl` | Measured, development. Regression check |
| Frozen held-out Part 102 rulings with a plan template | 8 of 18 (44.4%, 95% CI 24.6–66.3) with 20 families against 6 of 18 with 14; 1 failed check counted as none; on the 17 both answered 8 against 5 | `eval/results/families-coverage-final-all-litellm-p102-heldout-f20.jsonl` | Measured, held out. Pre-registered in D580, run once |
| Cost | $3.01: family checks $2.25, pool labels $0.28, drafts $0.22, citation checks $0.25, notes $0.01 | `eval/results/RUNLOG.jsonl`; `families102-*` files (`cost_usd`) | Measured, gateway-reported or token-priced at promo rates |

### Third round of plan templates (D660, D661)

Six templates written from substantial-transformation rulings dated before 2026 and in no development, held-out or tune-check split, chosen by counting the development and pool rulings of both regimes that the production family check gives no template. Coverage is scope, not accuracy: a template exists, not that a plan will be found.

| Number | Value | Source | Status |
|---|---|---|---|
| Selection set; with a template with 20 families | 1,436 rulings counted once (general development 469, a new pool 661, Part 102 development 72, the D580 pool 239); 996 with a template, 440 without | `eval/results/families-r3-selection.json`; `eval/families/r3-st-pool.json`; `families-coverage-train-all-litellm-{r3stpool,p102pool}-f20.jsonl` | Measured, development and pool data |
| Rulings with no template, by product type | paper and tissue 73, car filters 23 (no pool ruling), off-road vehicles and machinery 20, semiconductors 16, bicycles 13 (4 pool rulings), golf and sporting goods 13, toys and build kits 12, HVAC 11 | `eval/results/families-r3-selection.json` (`types`) | Measured; model-labelled nouns grouped by fixed patterns written after a first count by category |
| Citations kept | 123 citations of 59 rulings; 187 checks, 160 supported (28 of them on a first semiconductor draft that was replaced) | templates; `eval/results/families-r3-citation-checks.jsonl` | Built; checked by gemini-3.8-flash and read by the agent, not a customs expert |
| General development (469) with a plan template | 349 (74.4%, 95% CI 70.3–78.2) with 26 against 317 (67.6%, 63.2–71.7) with 20; on the 458 both answered 348 against 313 (37 gained, 2 lost, 3 moved) | `eval/results/families-r3-coverage.json` (`st_dev`) | Measured, development; the selection counted these rulings |
| Part 102 development (72) with a plan template | 50 (69.4%, 58.0–78.9) with 26 against 49 (68.1%) with 20; 3 gained, 2 lost | same (`p102_dev`) | Measured, development |
| First held-out split (377) with a plan template | 298 (79.0%, 74.7–82.9) with 26 against 224 (59.4%, 54.4–64.3) with 14; on the 369 both answered 294 against 223 (71 gained, 0 lost, 6 moved); the six new families matched 28 | same (`st_final`); `eval/results/families-coverage-final-all-litellm-f26.jsonl` | Measured once, held out, pre-registered in D660. No 20-family run exists, so the rise mixes D580's six and D660's six |
| Frozen held-out Part 102 rulings with a plan template | 8 of 18 (44.4%, 24.6–66.3) with 26, as with 20; on the 17 both answered 7 against 8 (1 lost, 1 moved to HVAC) | same (`p102_frozen`); `…-p102-heldout-f26.jsonl` | Measured once, held out, pre-registered in D660 |
| Live plan checks, one per new family | 5 of 6 show a precedent-supported plan, 1 goes to a broker (the plush toy: CBP's own move found, check 0.90, below the bar); 4 of 6 are reverse probes; $0.5709, median 49 s, slowest 101 s | `eval/results/families-r3-live-checks.json`, `eval/results/plan-*-d660-litellm.json` | Development checks on pool-ruling fixtures, one run each; not an evaluation |
| Cost | $6.78: family checks $5.34, labels $0.51, drafts $0.19, citation checks $0.17, notes $0.01, live checks $0.57 | `eval/results/RUNLOG.jsonl`; `families-r3-*` files (`cost_usd`) | Measured, gateway-reported or token-priced at promo rates |

### Part 102 engine against the 102.20 rule: development error analysis and two fixes (D590, D591)

Development only, through an OpenAI-compatible LiteLLM gateway to Gemini, one run per series. Scored by `cd backend && go run ./cmd/tariffshift p102err-single -check` and `p102err-moves -check` (in `make reproduce`). Neither fix passed the D590 rule; not adopted; held out not scored. Agreement with the rule is not accuracy.

| Number | Value | Source | Status |
|---|---|---|---|
| Development Part 102 rulings the 102.20 check decides / leans on (three committed inputs) | 20 / 8 of 72; the check gives CBP's holding on 26 of the 28 | `eval/tariffshift/p102err/summary-single.json` | Computed, no model |
| Engine agrees with the check on single setups, decided / leans | October usmca-v1 14 of 20 / 6 of 8; D550 B 15 / 6; D550 V 14 / 7 | same | Computed from committed rows |
| Development plan fixtures built by the fixed D570 rules | 13 (46 development rulings pass the D570 steps; the family check finds a template for 13) | `eval/tariffshift/p102err/dev-set.json`, `eval/results/families-coverage-dev-all-litellm-p102-dev.jsonl`, `backend/testdata/planner_part102_dev/` | Measured, development |
| Engine calls per series (current setups and moves the check decides or leans on) | 97 | `eval/tariffshift/p102err/summary-moves.json` | Computed |
| B (production): agrees with the check on decided rows / leans / CBP's holding on Part 102 current setups | 43 of 57 / 17 of 23 / 8 of 16 | same; `runs/moves-d590-b-*.jsonl` | Measured, development |
| Disagreement categories, plan rows (B) | 20: classification 15, assembly 3, invented exception 1, rule input 1; 12 also cite simple assembly or 102.17 | `eval/tariffshift/p102err/categories-dev.json` | Hand-assigned before any variant ran |
| Disagreement categories, single setups | 8: classification 4, assembly 1, invented exception 1, rule input 2 | same | Hand-assigned |
| Va (product's codes in the plan facts) | 46 of 57 / 23 of 23 / 11 of 16; won 4, lost 1, exact McNemar p = 0.375; cost 1.03 × B | `summary-moves.json`; `runs/moves-d590-va.jsonl` | Measured, development. Did not pass (net gain under 6) |
| Vb (codes and the usmca-v3 checklist) | 54 of 57 / 21 of 23 / 9 of 16; won 11, lost 0, p = 0.001; cost 1.78 × B; 84,147 output tokens against B's 28,462 | `summary-moves.json`; `runs/moves-d590-vb.jsonl` | Measured, development. Did not pass (cost over 1.5 × B) |
| Safety flag S: decided development rows it flags under B | 9 of 57 | `summary-moves.json` (`safety_flags_s`) | Computed. Ships; changes no answer |
| Cost of this item | $3.4375 (family check $0.0908, group call $0.0457, B $0.8661, Va $0.8941, Vb $1.5408) | `eval/results/RUNLOG.jsonl` (`families-coverage-dev-all-litellm-p102-dev`, `part102-dev-groups-d590`, `p102moves-d590-*`) | Measured, gateway-reported |

### Part 102 engine: a short 102.11 checklist, production since 22 Nov (D600, D601, D602)

Through an OpenAI-compatible LiteLLM gateway to Gemini, one run per series. Scored by `cd backend && go run ./cmd/tariffshift p102err-moves -check` and `p102ck-eval -check` (in `make reproduce`). Cost and latency are reported, not gating (owner decision, D601).

| Number | Value | Source | Status |
|---|---|---|---|
| Vc (`usmca-v4` compact checklist, codes in plan facts): decided plan rows / leans / CBP's holding on current setups | 54 of 57 / 22 of 23 / 10 of 16; won 11, lost 0, p = 0.001; cost 1.37 × B; 45,681 output tokens | `eval/tariffshift/p102err/summary-moves.json`; `runs/moves-d600-vc.jsonl` | Measured, development |
| Vd (`usmca-v5` failing materials only, codes in plan facts) | 56 of 57 / 23 of 23 / 9 of 16; won 13, lost 0, p = 0.0002; cost 1.11 × B; 25,157 output tokens | same; `runs/moves-d600-vd.jsonl` | Measured, development. Selected (D602) |
| Correct on the 72 development Part 102 rulings: B / Vb / Vc / Vd | 53 / 54 / 56 / 53 | `eval/tariffshift/p102ck/summary-d602.json` | Measured, development |
| Median per-call latency on the 72: B / Vb / Vc / Vd | 8.4 s / 13.2 s / 9.3 s / 7.9 s | same (`latency_p50_ms`) | Measured, development |
| Vd held out, window 1 / window 2 / pooled | 29 of 32 / 11 of 13 / 40 of 45 (usmca-v1 25 / 11 / 36); window 2 with only the 15 window-2 rulings as precedents (D612) | same; `eval/tariffshift/p102ck/runs/usmca-final*-usmca-v5-*-d601.jsonl` | Measured, held out, scored once; windows scored before with usmca-v1 (candidate comparison) |
| Held-out answers at confidence ≥ 0.95, right: usmca-v5 / usmca-v1 | 16 of 17 / 21 of 22 | same run rows (`confidence`, `correct`) | Computed from committed rows |
| `usmca-v5` held out at stated confidence ≥ 0.95, by window: accepted, right | window 1: 11 of 32 (34.4%), 11 right; window 2: 6 of 13 (46.2%), 5 right (`usmca-v1` window 1: 15 of 32, 46.9%, 15 right) | `eval/tariffshift/p102ck/runs/usmca-final*-usmca-v5-*-d601.jsonl` (`confidence`, `correct`); served as `usmca_v5_final_conf95_accuracy`, `usmca_v5_final_conf95_coverage`, `usmca_v5_final2_conf95_*` (D620) | Computed from committed held-out rows; the /accuracy Part 102 card gives these as the current engine's, and `usmca-v1`'s 46.9% as the earlier engine's |
| Safety flag S under Vd: decided development rows flagged | 0 of 57 | `summary-moves.json` (`safety_flags_s`) | Computed |
| Cost of D600 to D602 | $5.2524 | `eval/results/RUNLOG.jsonl` (`p102moves-d600-*`, `usmca-*-d600`, `usmca-*-d601`) | Measured, gateway-reported |

### Part 102 engine: auto-accept bar and latency (D610 to D614)

From committed rows (`cd backend && go run ./cmd/tariffshift p102bar -check`, in `make reproduce`; `eval/tariffshift/p102bar/summary.json`), plus one development re-run through an OpenAI-compatible LiteLLM gateway to Gemini.

| Number | Value | Source | Status |
|---|---|---|---|
| `usmca-v5` development rows at stated ≥ 0.95: accepted, right | 38 of 72, 36 right (94.7%, 95% CI 82.7–98.5); `usmca-v1` 38, 36 | `summary.json` (`d610_bar.dev_stated_curve`, `dev_usmca_v1_stated_curve`) | Computed, development |
| `cal-p102-v1` map steps (development) | 0.45–0.90 → 0.4483 (29, 13 right); 0.92–0.98 → 0.9302 (43, 40 right); no bar | same (`map_cal_p102_v1`) | Computed. Not a candidate, not shipped |
| Held out at stated ≥ 0.95, pooled 45: `usmca-v5` / `usmca-v1` | 17 accepted, 16 right (94.1%, 95% CI 73.0–99.0) / 22, 21 (95.5%) | same (`held_out_stated_table`) | Computed from committed held-out rows; bar unchanged |
| Held out at stated ≥ 0.92, pooled: `usmca-v5` / `usmca-v1` | 19, 18 / 28, 26 | same | Computed, reported only |
| Held-out ECE, `usmca-v5`, stated / map | 0.0916 / 0.2371 | same (`held_out_ece_stated`, `held_out_ece_map`) | Computed once |
| Per-call latency median / 95th percentile, `usmca-v1` → `usmca-v5`: development 72 / window 1 / window 2 | 8.4 / 21.8 s → 7.9 / 18.3 s; 7.1 / 9.2 s → 7.7 / 27.8 s; 8.5 / 16.4 s → 7.4 / 17.5 s | same (`d611_latency.runs`) | Computed from committed rows |
| Window 1's two slowest `usmca-v5` calls: latency, output tokens (run median) | 28.4 s, 200; 27.8 s, 274 (256) | same (`slowest`, `output_tokens_p50`) | Computed |
| D611 re-run, arm A / arm B (hedge after 12 s): wall time median / 95th percentile | 13.1 / 51.4 s; 13.0 / 40.2 s | `summary.json` (`arm_a_no_hedge`, `arm_b_hedge_12s`); `eval/tariffshift/p102bar/runs/` | Measured, development, gateway |
| D611 re-run, arm A / arm B: correct of 72; gateway-reported cost | 57, $0.5834 / 54, $0.7079 (hedges sent on 38 of 72; with cancelled hedges counted about $1.0815) | same | Measured, development, gateway |
| Arm A latency against D600's run of the same prompt, per ruling (rank correlation) | −0.07 | same (`spearman_arm_a_vs_d600`) | Computed |
| Window-2 Part 102 runs: rulings with facts in the precedent pool | 15 of 488 labelled (the facts path named a missing directory, D612) | `eval/data/labels_usmca*.jsonl`, `eval/data/facts_final2/` | Computed; window 2 not re-scored |
| Cost of D611 | $1.6649 (A $0.5834, B $0.7079, cancelled hedges estimated $0.3736) | `eval/results/RUNLOG.jsonl` (`usmca-dev-*-d611-a`, `-d611-b`); `summary.json` | Measured, gateway-reported, plus estimate |

### Method page (/method): negative results, splits and leakage, already in the decision record

These rows register, for the /method page, figures that are already committed in `docs/EVALUATION.md` and `docs/DECISIONS.md`. No new run produced them.

| Number | Value | Source | Status |
|---|---|---|---|
| Fine-tuned Gemini 3.5 Flash, development set (463 adjusted; D73) — not adopted | 87.9% (84.6–90.6) against Argon on Vertex AI 91.1% (88.2–93.4); fixed 15, broke 30, exact McNemar p = 0.036; fast path at 0.92: 94.0% of cases at 89.4%. Held-out not run; the tuned endpoints were deleted | `eval/results/dev-gemini-3.5-flash-tuned-prec-engine-v1-vertex.jsonl`; docs/EVALUATION.md 3.3 | Measured, Vertex AI. Must say **"development set"** and "dropped". A negative result |
| Untuned Gemini 3.5 Flash and Gemini 3.8 Flash, development set (D44, D73) | 3.5 Flash 90.5% (87.5–92.8), fixed 16, broke 19, p = 0.736; 3.8 Flash 90.1% (87.0–92.5), fixed 13, broke 18, p = 0.473 (both against Argon on Vertex AI) | `eval/results/dev-gemini-3.5-flash-prec-engine-v1-vertex.jsonl`, `eval/results/dev-gemini-3.8-flash-prec-engine-v1-vertex.jsonl`; docs/EVALUATION.md 3.3 | Measured; must say **"development set"** |
| Model sweep intervals, development set (D153) | Argon gateway 91.8% (88.9–94.0), fixed 8, broke 5, p = 0.58; Carbon 88.1% (84.9–90.8), fixed 6, broke 20, p = 0.009; 3.1 Pro preview 87.3% (83.9–90.0), fixed 12, broke 30, p = 0.008; majority of three 91.6% (88.7–93.8), fixed 8, broke 6, p = 0.79 (each against Argon on Vertex AI) | `eval/results/accuracy-v2-metrics.json` (`dev`); D153 table | Measured; gateway runs; must say **"development set"** |
| rules-v2, development set (D157) — not adopted | 90.9% (88.0–93.2) against 91.8% for engine-v1 on the same gateway path; fixed 9, broke 13, p = 0.52 | `eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-rules-v2.jsonl` | Measured; gateway; must say **"development set"** |
| router-v3, router development split (D231) — not adopted | general-engine routing 528/547 = 96.5% against router-v2 536/547 = 98.0%, 1.5 points lower, over the 1.0-point limit of the D230 rule | `eval/results/route-dev-router-v3-gemini-3.8-flash-litellm.jsonl` | Measured; gateway; must say **"development set"** |
| Fine-tuning split (D38, D73) | 699 rulings dated before 2025-07-01: train 632, tune_check 67; used only by the fine-tuning experiment | `eval/splits/splits_tune.json`; docs/EVALUATION.md 2.1 | Built |
| Held-out precedents that are earlier held-out rulings (first held-out split, official run) | 414 of 1,885 retrieved precedent slots, in 209 of 377 cases; 0 slots dated on or after their case | `claimscheck` (`final.precedent_slots_in_final_split`, `final.cases_with_final_split_precedent`, `final.precedent_slots_not_before_case`); docs/claims-reconciliation.md 4.2 | Measured. A stated leak path, not an estimate of its effect: no committed run removes those slots |

## Textile rules, 19 CFR 102.21 (D951, D952; flag `TEXTILE_RULES`, off; Accuracy page)

| Number | Value | Source | Status |
|---|---|---|---|
| Textile rule check agrees with CBP, held out (facts plus suggested subheading; `final` + `final2`) | 13 of 14 decided rulings (92.9%, 95% Wilson interval 68.5–98.7). 14 of 41 in-scope rulings decided (34%); 88 rulings, 47 out of scope. Fewer than the 15 decided that D951 required, so the flag stays off | D952; `eval/textile102/summary.json` (`go run ./cmd/textilerules eval -check`) | Showable. Held out of the rule code, not new to the project (the 88 scored the model in D51). Same answer as the model on every decided ruling |
| Textile rule check, full CROSS text (encoding check) | 18 of 18 decided (82.4–100), 44% of in-scope; with CBP's code in view | D952; same summary | Showable only as an encoding check |
| Textile rule check, facts alone | 0 of 88 decided: the texts rarely state the good's code | D952; same summary | Showable |
| Textile rule check, development | 32 of 32 decided agree (89.3–100) with a suggested subheading | D952; same summary | Not independent: the code was tuned on these 99 rulings |
| Textile rule check, cost | $7.28 of a $25 sub-cap (extraction through the gateway) | `eval/results/RUNLOG.jsonl` | Showable |

## Tariff watch (D941, backend behind `TARIFF_WATCH`; Accuracy page)

| Number | Value | Source | Status |
|---|---|---|---|
| Tariff watch, affected lines found, HTS Rev 22 to 23, entry day 10 Jan 2027 | recall **120/121 = 99.2% (95.5–99.9)**; precision 120/120 (96.9–100); no line affected in the Rev 21 to 22 pair; new additional-duty rate exactly right on 4 of 4 maritime lines (and on all 20,048 line-days scored, nearly all with no change). Entry day 27 Nov 2026: recall 116/117, precision 116/116 | `eval/tariffwatch/summary.json` (`go run ./cmd/tariffwatch eval`, no model, no network; recomputed by a test); truth from Federal Register 91 FR 72610 and 91 FR 72608 with the exclusion lists of 89 FR 46948 and 89 FR 76581 and the maritime lines of 90 FR 48320 | Measured. Show with n and the interval and the limits: not a blind test, one revision pair, 4 rate lines, detects changed terms not dollars, truth for exclusions read from scanned pages by OCR (three readings voted). First run, before the OCR rule was fixed: precision 0.950 (114/120), 6 false alarms, 1 miss (D941) |

## API limits (developers page, `docs/api/SECURITY.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Requests a minute per client, routes without a model call | 120 | `RATE_BASIC_PER_MIN` default, `backend/internal/config/config.go` (D137) | Built; default, configurable |
| Light / heavy model requests a minute per client | 20 / 4 | `RATE_LIGHT_PER_MIN`, `RATE_HEAVY_PER_MIN` defaults, same file (D45) | Built; default, configurable |
| Model-calling requests running at once per client | 4 | `RATE_MAX_IN_FLIGHT` default (D137) | Built; per instance |
| Invalid API keys a minute per address | 10 | `RATE_AUTH_FAILURES_PER_MIN` default (D137) | Built; per instance |
| One client's share of the daily model budget | a tenth (0.1) | `DAILY_SPEND_CLIENT_SHARE` default (D137) | Built; applies only with `DAILY_SPEND_CAP_USD` set |
| Broker review link life | 14 days by default, 30 at most (1 to 30 accepted) | `reviewDefaultDays`, `reviewMaxDays` in `backend/internal/httpapi/reviews.go` (D200) | Built; checked by the server on every read and answer |
| Broker comment length | 2,000 characters at most | `MaxReviewComment`, same file (D200) | Built |
| Open broker review links per case | 5 | `maxActiveReviews`, same file (D200); 20 links kept per case, 20 answers per link | Built |
| Broker review requests a minute per client | 30 | `RATE_REVIEW_PER_MIN` default, `backend/internal/config/config.go` (D200) | Built; default, configurable; per instance |
| Supplier declaration link life | 14 days by default, 60 at most | `DefaultDays`, `MaxDays` in `backend/internal/supplierdecl/supplierdecl.go` (D937) | Built; behind `SUPPLIER_DECLARATIONS` (off) |
| Open supplier requests per case | 5 (20 kept) | `MaxActive`, `MaxRequests`, same file (D937) | Built; behind the flag |
| Supplier declaration requests a minute per client | 30 | `RATE_DECLARATION_PER_MIN` default, `backend/internal/config/config.go` (D937) | Built; default, configurable; per instance |
| Attached supplier document | 128 KB at most, one per answer | `MaxDocBytes`, same file (D937) | Built; type and pages checked as uploads are |

## Demo story

| Number / claim | Value | Source | Status |
|---|---|---|---|
| Heat gun 2022 origin | CBP held China | CBP ruling N328954 (15 Nov 2022) | Showable |
| Heat gun 2026 origin after the change | CBP held Thailand | CBP ruling N357771 (29 Jan 2026) | Showable |
| Duty difference China vs Thailand, heat gun | ≈25 points (≈41.7% vs ≈16.7% ad valorem) | `docs/DUTY-SOURCES.md` (HTS 8419.89.95 inferred; Section 301 List 2) | Showable with "product-specific; classification to be confirmed by a broker" |
| Plan shown | Make the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand: check 0.96 (at or above 0.92), supporting N345938 (hair clipper kit, 6 Mar 2025), no counter-ruling, engine Thailand 1.00, net $285k/yr (illustrative economics). 1 of 7 moves cleared the bar; the closest single move, motor-fan assembly alone, is 0.90 on N325810. Backtest against N357771: of the 6 parts whose origin CBP's change switched from China to Thailand, the plan moves all 6 (motor, fan, PCBA, knob, power cord, back cover); it also moves the switch, wires and fasteners, which the change kept in China. The outer shells, also listed as Thai in N357771, were already Thai in 2022 and are not counted (D310) | Production run, backend-00051-kmk, case_878d76540b91, cached for everyone | One production run, after three-group moves (D84) and the relaxed precedent search (D89). Earlier run (backend-00038-zw2): no plan cleared 0.92, closest move motor-fan assembly 0.90 |
| Backtest of the plan shown, heat gun | moves all 6 parts whose origin CBP's N357771 change switched from China to Thailand (electric motor, fan, PCBA, knob, power cord, back cover). Also moves 3 parts that change kept in China: the switch, wires and fasteners. Not counted: the outer shells, which N357771 lists as Thai but were already Thai in 2022 (N328954 does not list them among the Chinese parts) | Production case `case_878d76540b91`, `plan.backtest` (`GET /v1/cases/case_878d76540b91`, read 21 Nov 2026); N357771 and N328954 facts (`backend/data/facts/`); `actual_change` in `backend/internal/demos/fixtures/heatgun_2022.json` (`already_in_target`, `still_from_china`, `still_from_china_as`); same plan as the "Plan shown" row | One production run. Not a blind test: three-group moves were added after the team saw this change (D84); the date cutoff holds. Shown on the plan step's header band and backtest section, the landing, and /method (D290, D310). Replaces "6 of the 7 parts", which counted the already-Thai shells as missed. Made with the template before the split (D330); stays as stored until an owner re-pins the demo |
| Duty effect / added cost on the plan shown | $500k/yr duty saved and $215k/yr added cost, net $285k/yr | Fixture `group_costs` in `backend/testdata/heatgun_2022.json`: Thailand minus China for the three moved groups, 120k + 60k + 35k = 215k (derivation in the "Impact story" table below); duty saved from `duty_rates` | Show only as "illustrative — editable". The $180k/yr that earlier drafts and the `/business` page show is a different move, motor-fan assembly plus PCBA only (120k + 60k, net $320k; D331 ranking). It is not the plan shown. The `/business` page labels its money as example figures |

## Known failure cases and corpus counts (Accuracy page, MODEL-CARD.md, DATASHEET.md)

Built by `python3 -I eval/charts/build_trust_data.py .` from committed files; `--check` fails if the outputs are stale. No model calls.

| Number | Value | Source | Status |
|---|---|---|---|
| Held-out misses listed | all 37 of 377; 12 auto-accepted (confidence ≥ 0.92 and in scope), 25 sent to review | `frontend/src/lib/failure-cases.json`, from the final Vertex rows + `eval/results/scope-final-router-v2-vertex.jsonl` | Measured. Every miss is listed, none selected |
| Accuracy by origin CBP held, held-out | Thailand 60/66 = 90.9% (81.6–95.8); China 44/57 = 77.2% (64.8–86.2); United States 51/53 = 96.2% (87.3–99.0); Vietnam 43/46 = 93.5% (82.5–97.8); Malaysia 34/35 = 97.1% (85.5–99.5); South Korea 20/22 = 90.9% (72.2–97.5); Taiwan 18/19 = 94.7% (75.4–99.1); 26 other countries 70/79 = 88.6% (79.8–93.9). Wilson 95% | same file, `by_origin` | Measured. Small n per origin: always with the interval |
| Direction of the misses | CBP held China, Unde said another: 13 (7 auto-accepted). Unde said China, CBP held another: 18 (4 auto-accepted). Neither China: 6 | same file, `china` | Measured |
| Held-out accuracy by collection | NY 328/364; HQ 12/13 | same file, `by_collection` | Measured. HQ n = 13 is too small to compare |
| Corpus | 2,790 rulings, 2023-01-10 to 2026-09-25; NY 2,619 / HQ 171; by year 354 / 425 / 1,358 / 653; regex 2,696 / manual 94; 3,081 facts files | `eval/results/corpus-stats.json` | Measured |
| Broker draft facts cleaning, committed facts (D250) | of 3,081 facts files, 2,613 carry a CBP reply phrase; after cleaning none keeps one and no paragraph starts with a fragment; 32,973 of 33,046 reference sentences kept (99.78%; D180 version 98.04%); test bar 99.5% | `backend/internal/export/facts_test.go` (`TestCleanFactsCorpus`) over `backend/data/facts` | Measured, no model |
| Substantial-transformation pool | 1,703 rulings, 1,593 with a single held origin, 93 countries; China + ASEAN 62.8% of those 1,593 | same file | Measured |

### Revoked and modified precedents (D320)

Built by `go run ./cmd/corpus supersession` (from `backend/`; `-check` verifies) and `python3 -I -B eval/supersession/impact.py . --check`. No model calls.

| Number | Value | Source | Status |
|---|---|---|---|
| Supersession map | 144 edges (supersession-v2): 35 from 3,275 corpus rulings read (3,112 with full text, 163 by subject only) and 109 from Customs Bulletin notices only; 143 rulings revoked or modified (70 revoked, 74 modified edges), by 62 later rulings; 14 of the 143 are in a precedent pool (4 general, 10 Part 102) | `backend/internal/domain/supersession.json` (`counts`) | Measured. Only revocations whose letter is in the corpus, that a later ruling in it mentions, or that a fetched bulletin published |
| Committed rows that showed or cited a superseded precedent | of 14,160 rows with rulings in 158 files: 23 showed one revoked or modified before the case date, 8 cited one, 1 supported verifier verdict cited one (a rejected heat-gun move at 0.90, N322364) | `eval/results/supersession-impact.json` (`totals`, `files_with_rulings`) | Measured offline; nothing re-scored. D370: a change dated on the case date does not count (it was 43 and 10 under D320's first rule, mostly the revoking case H325607 itself) |
| Customs Bulletin notices read (D400, D401) | 28 bulletins (vol. 57 no. 32, 6 Sep 2023, to vol. 60 no. 30, 30 Sep 2026); 81 ruling-letter notices: 38 final, 42 proposed, 1 withdrawal; 3 flagged by the parse checks and read once by a model (2 rulings added, 0 dropped, 0 failed reads); 117 final changes, of which 109 new edges and 8 confirming a corpus edge; 78 proposals pending (0 in a precedent pool) | `eval/data/bulletins/index.jsonl`, `notices.jsonl`, `model_reads.jsonl`; `supersession.json` (`counts`, `proposed`) | Measured. A sample of the weekly bulletin chosen by web search, not every issue |
| Customs Bulletin model reads | $0.0403 gateway-reported for 3 calls (gemini-3.5-flash) | `eval/data/bulletins/model_reads.jsonl` (`cost_usd`) | Measured |
| Committed rows naming a ruling changed only in the Customs Bulletin | 6 (N326416 in textile development rows, case date 2025-12-11; modified effective 2026-10-18), 0 of them before the change | `eval/results/supersession-impact.json` (`bulletin_rulings_named`, `totals`) | Measured offline; the 23 showed / 8 cited figures are unchanged |
| H325607 (the letter that revoked N331974) | wrong in 28 of 28 development engine runs (27 Spain, 1 China), each of which showed N331974 (20 of 20 before the two #815 runs of D857, 22 of 22 before the two #831 runs of D861 and 24 of 24 before the two runs of D905 and 26 of 26 before the two runs of D908, all eight also Spain) | `eval/results/dev-*-engine-v1*.jsonl`, rows with `ruling` H325607 (one per file; `correct`, `pred`, `precedents`); recomputed by `TestH325607DevRuns` in `backend/cmd/claimscheck`. Since D370 these rows are no longer in `supersession-impact.json` (#566) | Measured. Not re-run with the guard, so no effect is claimed |
| First held-out split rows that showed N331974 | 8 over 4 runs (N361172, N364229), all correct | same file | Measured. Held-out figures unchanged |
| Revocation alerts on committed results (D480, D481; recounted in D490) | of 19,159 result rows naming rulings: 3 rely on a ruling changed on or after their case date (N356467 citing N344694, modified by H344999, in 2 development runs; training ruling N349019 citing N344694 in the china-v1 check run of D861), 7 on one changed before it (7 of the 8 above; the eighth also cites the revoker), 0 on a proposed change only; 99 only read a changed ruling (95 and 18,222 rows before D908, which added the 937 rows of its two development runs; 91 and 17,285 rows before D905, which added the 937 rows of its two development runs; 17,253 rows before D869, which added the 32 rows of #837; 72 before D861, which added 2,184 rows in four runs; 68 before D857; the four added and the rows total, 14,102 before, include the two #815 development runs; 15,039 before D859, which added the two food-fresh runs). Before D490 the count was 4, 8 and 85: a revoking ruling (H325607) was alerted on the ruling it revokes. Stored case documents: 0 of 7; demo fixtures: 0 of 16 | `eval/supersession/alerts.json` (`rows`, `row_examples`, `cases`, `demo_fixtures`) from `go run ./cmd/corpus alerts`; `TestCommittedAlertCount` | Measured offline as of 22 Nov 2026; no model call |
| Revocation alerts by relevance to the answer (D531; per role since D561, recount unchanged) | of the 9 relied-on final changes in committed rows (2 after the case date, 7 before): 2 do not touch the answer (N344694, China to Taiwan; the answer was Vietnam), 3 keep the same held origin (N345891 and H346161, both Taiwan by the labels), 4 not known (the one plan-support change, N322364 in the heat-gun plan, among them: its held origin is not in the labels); 0 touch the answer | `eval/supersession/alerts.json` (`relevance_final_relied`, `row_examples`) from `go run ./cmd/corpus alerts` | Measured offline as of 22 Nov 2026; no model call. Held origins from the shipped labels |
| Held origins known for rulings in the revocation map (D531) | 37 of 308 rulings (changed or changing) have a label; 30 give one origin. H325607 is left unknown: its label was read from its account of N331974 | `backend/internal/domain/ruling_holdings.json` from `go run ./cmd/corpus holdings` | Measured. Most changed rulings are classification letters with no origin label |

## Japan Customs advance rulings on origin (D710, D711; eval only, not in the product)

Built by `eval/jp/*.py` and `go run ./cmd/jpeval` (from `backend/`); `make reproduce` checks the split, labels, translation check, index stats and baseline metrics. These are phase-one development figures. D714 records the phase-two attempt.

| Number | Value | Source | Status |
|---|---|---|---|
| Rulings in the snapshot (22 Nov 2026) | 3,606 (the search's own count); by year 2022 1, 2023 264, 2024 1,274, 2025 1,434, 2026 633; 65 tariff chapters, chapters 84, 85, 87 and 90 together 4 | `data/jp/snapshot.json`, `eval/jp/index-stats.json` | Measured. A rolling window: the site keeps about three years |
| Rulings by agreement | Japan-EU EPA 1,305; CPTPP 670; AJCEP 633; RCEP 402; Japan-US 234; GSP 168; 12 others 194 (non-preferential 6) | `eval/jp/index-stats.json` (`by_agreement`) | Measured |
| Split (D710) | development 2,973 (before 2026), held-out 633 (from 2026), `dev_eval` 400 | `data/jp/splits.json` | Frozen before any label was read |
| Published negative answers | 0 of 2,973 development rulings | `data/jp/labels-dev.jsonl` (`outcome`) | Measured. So no real-ruling figure can test a refusal |
| Translation spend | $6.98 for 3,606 rulings (gemini-3.8-flash, gateway-reported) plus a $0.05 pilot | `data/jp/translations-en.jsonl` (`cost_usd`) | Measured |
| Translation check, development | tariff numbers kept 2,973 of 2,973; quoted rule codes 2,969 of 2,973; named origin 1,155 of 1,155 | `eval/jp/translation-check.json` | Measured, mechanical. Not a review by a Japanese customs specialist |
| Japan baseline, criterion match, development (`dev_eval`, D711) | **370 of 400 = 92.5% (89.5–94.7)**; majority (always CC) 36.0%; 368 (92.0%) with the labels before D711's two disclosed pattern changes | `eval/jp/baseline-dev-metrics.json`; rows `eval/jp/runs/jp-dev_eval-gemini-4-argon-jp-baseline-v1-litellm-d711.jsonl` | Measured once, gateway; must say **"development set"**. All answers are positive, so it measures the criterion, not refusals |
| Japan baseline cost and latency | $0.0073 a case ($2.93 for 400); p50 6.7 s, p95 11.1 s | same file | Measured, gateway |

## Planner product scope (D138)

| Number | Value | Source | Status |
|---|---|---|---|
| Product families with a plan template | 26 (3 before D138, 10 added in D138, 1 in D212, 6 in D580, 6 in D660) | `backend/internal/planner/templates/*.json`; pinned by `TestTemplateSetIsPinned` | Built |
| Selection pool | 845 substantial-transformation rulings outside the dev, held-out and pilot splits; 840 labelled | `eval/results/families-selection.json` | Measured (model labels, gemini-3.8-flash) |
| Assembled pool rulings per new family | vehicle parts 88, PCBA-based devices 86, hand tools 67, cables and harnesses 59, medical devices 26, power supplies 23, furniture 15, display and cameras 14, water filters 12, gas grills 11 | `eval/results/families-selection.json` (`chosen`) | Measured (model labels) |
| Rulings cited by the new templates | 156 distinct; 267 of 326 citation checks supported, the rest dropped | `backend/internal/planner/templates/*.json` (`sources`), `eval/results/families-citation-checks.jsonl` | Measured; each citation checked by gemini-3.8-flash against the ruling's facts and holding |
| Coverage, held-out (377, 2026) | 16 (4.2%) before, 215 (57.0%) after; failed checks counted as not covered. On the 368 checks that answered: 58.4% (53.3–63.3) | `eval/results/families-coverage-final-{legacy,all}-litellm.jsonl` | Measured once per set (LiteLLM gateway, not Vertex). Say "matched to a family with a plan template", never "planned" or "accurate" |
| Coverage, development (469, Jul–Dec 2025) | 23 (4.9%) before, 258 (55.0%) after; on the 452 that answered: 57.1% (52.5–61.6) | `eval/results/families-coverage-dev-{legacy,all}-litellm.jsonl` | Measured; say "development set" |
| Family 14, electric motors, pumps and compressors (D212) | written from the 10 assembled pool rulings D138 named; 18 of 30 citation checks supported, 7 distinct rulings cited | `backend/internal/planner/templates/motors_pumps_compressors.json`; `eval/results/families14-citation-checks.jsonl`, `eval/results/families14-draft.json` | Built; checked by gemini-3.8-flash and read by the agent, not a customs expert |
| Coverage with 14 families, held-out (377, scored once after the template was committed) | 224 (59.4%, 54.4–64.3) against 215 (57.0%) with 13; failed checks counted as not covered. On the 367 rulings both runs answered: 219 against 214 (+1.4 points; 6 gained, 1 lost, exact McNemar p = 0.125). The new family matched 8, 3 of them previously matched by another template | `eval/results/families-coverage-final-all-litellm-f14.jsonl` vs `…-final-all-litellm.jsonl` | Measured once, gateway. Scope, not accuracy. Most of the all-in gain is fewer failed checks (9 to 4) after the 2,048-token cap |
| Coverage with 14 families, development (469) | 265 (56.5%, 52.0–60.9) against 258 (55.0%) with 13; on the 446 both answered: 258 against 253 (+1.1 points; 6 gained, 1 lost). At the old 1,024-token cap the 14-family run failed 35 checks (17 with 13 families) | `eval/results/families-coverage-dev-all-litellm-f14-cap2048.jsonl`, `…-f14.jsonl` | Measured; say "development set" |
| Live plan check, family 14 | condensate pump (N347899, CBP held China): engine China 0.90; 4 plans shown, best "make the motor stator and rotor in Thailand", supported by N344081 at 0.94; $0.24 promo | `eval/results/families-plan-condensate-pump-2025-litellm.json` | Development check on a pool-ruling fixture, one run, gateway; not an evaluation |
| Family-check failures | 13 families: 17 of 469 dev, 9 of 377 held-out at 1,024 output tokens; 31 of 469 dev at the old 400. 14 families: 35 of 469 dev at 1,024; 9 of 469 dev and 4 of 377 held-out at 2,048, the cap now in use (D212) | coverage rows above | Measured |
| Live plan checks, new families | 7 of 10 fixtures show a precedent-supported plan, 3 go to a broker; $0.97 promo for the ten | `eval/results/families-live-checks.json` | Development checks on pool-ruling fixtures, one run each; not an evaluation |
| Gas-grill demo, development check | engine China 0.95 (CBP held China, N335085 scenario 1); 7 moves searched, 1 shown: lid and firebox forming, burners and gas train, and cooking surface, panels and frame in Indonesia, check 0.96, supporting N333420; backtest: moves both parts CBP's scenario 2 moved, plus more. Duty rates, import value and costs are illustrative placeholders | `eval/results/families-plan-gas-grill-2023-litellm.json`; demo `backend/internal/demos/fixtures/gas_grill_2023.json` | One run through the LiteLLM gateway, not production. The demo runs live when loaded, so the site's answer may differ; quote the screen, not this row |

## Planner evaluation (D170, D171)

| Number | Value | Source | Status |
|---|---|---|---|
| Post-cutoff flip pairs | 15 frozen pairs from CBP rulings, at least one dated after 25 Sep 2026: 11 real origin changes (6 core, 5 edge), 4 controls; 6 with a plan template | `eval/data/flips_post.jsonl`, `eval/data/flips_post_meta.json`, `eval/results/planner-post-familycheck-litellm.jsonl` | Built; each pair read by hand by one reader (not a customs expert) |
| Precedent check on them, CBP's starting origin given | shown at the 0.92 bar: 8 of 8 right (67.6–100); real changes shown 8 of 11 = 72.7% (43.4–90.3); core 5 of 6; controls shown 0 of 4; $0.58 for 15 checks (promo) | `eval/results/flips-post-verifier-v2-known-a-litellm.jsonl`, `eval/results/planner-post-summary.json`; recomputed by `claimscheck` (`planner_post.*`) and served as `planner_post_known-a_*` | Measured, run 1 of 3: runs 2 and 3 gave the same figures (D354, `eval/results/planner-post-repeats-summary.json`); gateway (not Vertex AI), local corpus search. Always with n and the interval; say "precedent check", not "plan accuracy" |
| Precedent check on them, starting origin not given | shown 5, right 4 (37.6–96.4): the fracking unit was shown at 0.92 with the wrong later origin; real changes shown 4 of 11 = 36.4% (15.2–64.6); controls shown 0 of 4 | `eval/results/flips-post-verifier-v2-unknown-a-litellm.jsonl`, `eval/results/planner-post-summary.json` | Measured, run 1 of 3: runs 2 and 3 showed 5 and 4, right 4 each time (D354, `eval/results/flips-post-verifier-v2-run{2,3}-unknown-a-litellm.jsonl`); gateway. Show beside the known-a row, never alone |
| Cited ruling supports the shown move (hand read) | 5 of 8 support, 3 partial (known-a) | `eval/planner/spot_checks.json` | One reader |
| Plan search on the post-cutoff pairs | strut: CBP's move is the top plan (0.96); stove heater: CBP's move shown (0.92) but ranked below a cheaper subset CBP never ruled on; three air-purifier reverse probes: no plan shown (two had moves supported at 0.85, below the bar; D301 correction); $0.76 | `eval/results/planner-post-plan-*-litellm.json` | Measured, once each, gateway; n = 5 |
| Per-family plan checks | 23 products (two per family where eligible; gas grills one, LiDAR none): 15 of 23 = 65.2% (44.9–81.2) show a precedent-supported plan, 8 go to a broker; China-held 10 of 15, reverse probes 5 of 8; cited ruling supports the move in 9 of 15, partial 4, does not 2; $1.90 promo, $0.083 a search | `eval/results/planner-families-summary.json`, `eval/results/planner-families-*-litellm.json` | Development checks on development and train rulings, one run each, illustrative economics; not an evaluation of plan quality |
| Plan bar re-examined on committed rows (D300, D301) | dev rule (9 verifier-v2 runs, 485 verdicts) picks **0.92**: 159 shown, 0 wrong (97.6–100); 0.90 adds 2 wrong, 0.85 shows 256 with 3 wrong; real-change pairs shown in every run 13/46 at 0.92, 19/46 at 0.85. Wrong supported dev verdicts 3 of 287, under the 10 the rule asks for a change. At 0.85 on held-out rows: post-cutoff known-a unchanged (9/9, recall 9/12); unknown-a 8/9, recall 8/12 (5/6, 5/12 at 0.92); 2026 pairs 36/40 with two controls shown (21/23 at 0.92); plan searches: 3 more moves on the purifier probes (one contradicted by CBP), per-family plans on 21 of 23 instead of 15 | `eval/results/plan-bar-analysis.json` (`python3 -I -B eval/planner/plan_bar.py . --check`, in `make reproduce`) | Computed offline, no model calls; the bar was chosen on dev only and the held-out rows reported after. Not a blind test: the 0.92 held-out figures were published before. Production keeps 0.92 |

### Post-cutoff flip-pair addendum (D192)

| Number | Value | Source | Status |
|---|---|---|---|
| Addendum pairs | 3 frozen pairs (1 real change, 2 controls, all edge tier); re-probe of 1,284 ruling numbers found 0 new rulings | `eval/data/flips_post_addendum.jsonl`, `eval/data/flips_post_addendum_meta.json`, `eval/data/addendum_probe.jsonl` | Built; one reader |
| Precedent check, D170 + addendum (18 pairs), starting origin given | shown 9, right 9/9 (70.1-100); recall 9/12 = 75.0% (46.8-91.1); controls shown 0/6 (0-39.0) | `eval/results/planner-post-addendum-summary.json` | Measured, gateway, local corpus search; run 1 of 3, runs 2 and 3 gave the same figures (next row). Always with n and the interval |
| Same, starting origin not given | shown 6, right 5/6 (43.7-97.0); recall 5/12 = 41.7% (19.3-68.1); controls shown 0/6, one control run failed | same | Measured, gateway. Show beside the known-a row |
| Same 18 pairs, 3 runs (D353, D354) | starting origin given: shown 9, right 9, found 9/12, controls 0/6 in each of 3 runs; pooled 27/27 shown right (87.5-100), 27/36 found, 0/18 controls. Not given: shown 6, 6, 5; right 5 each run; found 5/12 each run; controls 0/6 each run | `eval/results/planner-post-repeats-summary.json` | Measured, gateway, same pairs each run, so the pooled interval is about repeat runs, not new pairs |
| Precedent check, real-change pairs shown in every run (D86 pool) | 17 of 59 = about 29% (development 13/46, 2026 pairs 4/13; 9 and 4 verifier-v2 runs, with and without the starting origin) | `backend/internal/evalsummary/summary.json`, `verifier_calibration.splits[].pair_level` (`shown_every_run`, `real_change_pairs`) | Measured, gateway. A stricter count than the 75% (9/12) above: earlier pairs, and a pair counts only if shown in every run. Both are labelled on /accuracy and the landing (D310) |
| Precedent check, one frozen pair: plywood core, Malaysia (N365961, 16 Oct 2026) to Tanzania (N366717, 12 Nov 2026) | shown at 0.96 (bar 0.92), supported by N347656, named Tanzania; same in all 6 runs (3 runs, with and without the starting origin); the check read only rulings dated before 16 Oct; the check was given CBP's later setup as the move, no plan search | `eval/data/flips_post.jsonl` (`SP-N365961-N366717`), `eval/results/flips-post-verifier-v2-*known-a-litellm.jsonl` | Measured, one pair of the 18 (edge tier); served on `/examples/blind-plan`. Never as a rate: n = 1 |

### New-family demos (D191)

| New-family demos with a recorded run | 11 (gas grill and four D138 families, plus one each for the six D660 families: paper/tissue, off-road vehicles, semiconductors, golf/sporting goods, toys/build kits, heating) | `backend/internal/demos/fixtures/*.json` (`recorded`), checked against the result files by `TestRecordedRunsMatchResultFiles` | Built. Development runs through the gateway, one each; not runs on this site until seeded |
| Recorded runs that disagree with CBP | 2 of 11 (aluminum wheel N349871: engine Thailand, stated 0.85, calibrated 0.60; roll AGV N336445: engine South Korea, stated 0.82; CBP held China in both) | `eval/results/planner-families-n349871-litellm.json`; `eval/results/families-r3-live-checks.json` | Measured, one run each. Show beside CBP's holding |
| Engine disagreed with CBP on the starting setup, per-family checks | 8 of 29: 7 of 23 (D171) and 1 of 6 in the D661 live checks (the roll AGV: South Korea, CBP China); 5 of 8 in the D138 live checks | `eval/results/planner-families-summary.json` (`engine_current` vs `cbp_held`); `eval/results/families-r3-live-checks.json` (`engine_current_origin` vs `cbp_current_origin`); `eval/results/families-live-checks.json` | Measured, development checks, one run each |

## Claude connector (MCP; built, see `docs/MCP.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| MCP tools | 16 in the hosted catalog on 23 Nov 2026 (13 after D200, 11 before D200) | `docs/MCP.md` (Tools table), `backend/internal/mcpserver` (registry), `frontend/src/app/developers/catalog.ts`; checked against hosted `tools/list` | Built. Tool availability follows the server configuration. |
| ADK agent full-flow run, first (22 Nov 2026) | 1 run, 1 case; 7 agent model calls, gateway-reported $0.0465; about 477 s, of which a plan call timed out at ADK's default 300 s and was retried (second call 103 s); predicted China (calibrated 90%, stated 95%), 1 supported plan to Thailand, 7 rejected moves | `docs/mcp-transcripts/adk-full-flow.md` ("The first run") and that file's git history (D391) | Run once, Gemini 3.8 Flash through a gateway; shows the flow works, not accuracy |
| ADK agent full-flow run, second (22 Nov 2026) | 1 run, 1 case; 7 agent model calls, gateway-reported $0.0446; about 400 s; the first plan call failed on the server's side at 300 s and the retry with the same idempotency key answered 29 s later (one search, joined, D402); predicted China (calibrated 90%, stated 95%), 2 supported plans to Thailand, 6 rejected moves | `docs/mcp-transcripts/adk-full-flow.md`, `docs/mcp-transcripts/adk-full-flow.replay.txt` (D422) | Run once, same model and gateway; the join is inferred from timings, not server logs |
| ADK agent evaluation, 12 products (22 Nov 2026; D450, D451) | 12 runs, one per product, rules written first: 10/12 passed every check; tool order 11/12, arguments 11/12 (both misses are the rules as written: a predict call refused by input validation and retried, and a current-setup draft after no plan was found); origin as `predict_origin` gave it 12/12; cites a tool-returned ruling 12/12; rulings named that no tool returned 0/12; "Not a ruling" line 12/12; target plant added 4/4. Agent model cost $0.4722 gateway-reported ($0.0394 a run); wall time median 313 s (113 to 396 s) | `eval/agent/results/live-gemini-3.8-flash-undelabs.jsonl`; `eval/agent/summary.json` (`python3 -I -B eval/agent/score.py . --check`, in `make reproduce`); docs/EVALUATION.md §12 | Measured once per product, Gemini 3.8 Flash through a gateway, against undelabs.dev. Measures the agent's tool use, not origin accuracy: predict_origin agreed with CBP 12/12 only because each product's own ruling is in the precedent pool. Do not show the 12/12 as accuracy |
| ADK agent fix check (22 Nov 2026; D470, D472) | a03, a04, a06, a10, a12 once each against a local server with the D470 changes: a04 drafted once by `plan_id`; a03 and a06 found no plan and claimed no working move; a06 called `predict_origin` once; a10 not tested (local spend cap), a12 not tested (no implausible code this time). Agent cost $0.0973 (first attempt, stopped by the local cap) and $0.1751 gateway-reported | `eval/agent/results/fixcheck-d470-local*.jsonl`; `eval/agent/summary.json` | Measured once, on dev products already seen, local server with Gemini 3.8 Flash as engine. A check that issues are gone, not a score: never pool with D451 or show its pass count as a rate |
| ADK agent evaluation v2, run 1 (22 Nov 2026; D510, D511) | 12 runs against undelabs.dev, checks written first (D450 plus D470: one draft, a `plan_id` the search returned, no claimed move after no plan): 9/12 passed every check (8/12 under D450's rules); a01-a08, not cut, 8/8; a09-a12 cut by the server's anonymous per-client budget (429), a09 still passes; D451's run rescored under the same rules 8/12, on a01-a08 5/8. Agent model cost $0.4189 gateway-reported; ADK trajectory metric replayed offline D451 12/12, run 1 8/12 | `eval/agent/results/live-v2-run1.jsonl`; `eval/agent/summary.json` (`series`); `eval/agent/adk/replay.json`; docs/EVALUATION.md §12 | Run once per product, Gemini 3.8 Flash through a gateway. Run 2 remains blocked by production sign-in after the budget reset (D731). Repeatability is unmeasured. Tool use, not origin accuracy |
| ADK authenticated live run (25 Nov 2026, D732, D906) | 10/12 passed every D510 check; a06 agent schema error, a07 production 502 | `eval/agent/results/live-v2-authenticated-run1.jsonl`, `eval/agent/SUMMARY.md` | Measured, live, signed in with an evaluator key; development products; not an accuracy figure (D450) |
| ADK v2 run 2, local server (25 Nov 2026, D866, D867) | 12/12 passed every D510 check (D450 rules 10/12); with run 1, 21/24; same pass or fail 9/12, same predicted origin 10/12 | `eval/agent/results/local-v2-run2.jsonl`, `eval/agent/SUMMARY.md` | Measured, development products; local server on the gateway, not the live site; not an accuracy figure (D450) |
| ADK v2 run 2 preflight (23 Nov 2026, D731) | HTTP 401 `unauthenticated` on anonymous case creation. Zero scored product sessions and $0 model spend | `eval/agent/adk/live-v2-run2-preflight.json`, `eval/results/RUNLOG.jsonl` | Operational preflight after the reset. Repeatability is unmeasured |

## Image and scan import (built; see `docs/MULTIMODAL.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Upload limits | 1–4 files, 8 MB together, 10 PDF pages together, 12,000 px a side, 40 megapixels | `backend/internal/media/media.go` constants | Showable; these are settings, not results |
| Browser downscale | photos over 3 MB or 4,096 px re-encoded at 3,072 px longest side | `frontend/src/lib/media-upload.ts` | Showable; a setting |
| Field accuracy reading BOM images (synthetic) | 20 synthetic images, 170 parts: names read 151 (88.8%), countries right 151 (88.8%); every matched name had the right country; 15 of 20 images fully read. Set 1 (5 plain tables) 45/48; set 2 (15 varied layouts, photos, scans) 106/122. The 19 misses are exact-name mismatches: part numbers appended (13), a multi-part row split (3, one cut short in the image), one row split into two (1), plant rows counted as parts (2). Gateway cost $0.252 | `eval/results/media-v1-gemini-4-argon-litellm.json`, `eval/results/media-varied-gemini-4-argon-litellm.json`, `eval/results/RUNLOG.jsonl` (D167, D168) | Measured once, gateway (not Vertex AI), Gemini 4 Argon. Shown on /accuracy under "Import from a photo or scan" (D180). Always say "synthetic images, illustrative" and give the counts. Not a measure of accuracy on real photos or scans; do not show it as one |

## Market and time (from `hackathon-research/evidence/fees/LEDGER.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Preparation per ruling request | 20 hours | CBP, 90 FR 54349, 26 Nov 2025 (OMB 1651-0085) — all ruling subjects, not only origin | Showable with that qualifier |
| Ruling responses per year | 3,628 regional (NCSD) | same notice | Showable; "all ruling subjects" |
| Price paid for US-origin advice | median KRW 5.0M ≈ US$3,600 (23 purchases, 2024–26) | KOTRA export-voucher marketplace purchase records | Showable; "Korean exporters, subsidised vouchers; team-compiled", and that it is the only public price for origin advice we found (not ASEAN, not a U.S. broker's fee) (D180) |
| CBP turnaround, Asian-origin rulings | median 21 days (IQR 14–26), n=247 | own count, rulings.cbp.gov | Showable; "team count". It is the time to a binding answer; set it beside Unde's time to a first, non-binding prediction, never as the same thing (D180) |

## Impact story (landing fig.4, illustrative)

A composite contract manufacturer, labelled on the page "Illustrative example — composite, not a customer". The inputs are the heat-gun demo fixture, so the arithmetic is the same one the plan screen does for the shown plan. Copy: `copy.landing.story` in `frontend/src/lib/copy.ts`.

| Number | Value | Derivation and source | Status |
|---|---|---|---|
| Heat guns imported a year | $2M | `annual_import_value_usd: 2000000` in `backend/testdata/heatgun_2022.json` | Illustrative input; say so on screen |
| Duty a year, China origin | $834,000 | 2,000,000 × 0.417 (fixture `duty_rates.China`, from `docs/DUTY-SOURCES.md`) | Illustrative; "classification to be confirmed by a broker" |
| Duty a year, Thailand origin | $334,000 | 2,000,000 × 0.167 (fixture `duty_rates.Thailand`) | Same |
| Duty saved a year | $500,000 (about 25 points) | 834,000 − 334,000 = 2,000,000 × 0.25. Same as the "Duty effect" row above | Same |
| Added cost of the shown plan | $215,000 a year | Fixture `group_costs`, Thailand minus China for the three moved groups: motor-fan 420k − 300k = 120k, PCBA 210k − 150k = 60k, switch, cord and minor parts 105k − 70k = 35k | Illustrative fixture costs |
| Net a year, shown plan | about $285,000 | 500,000 − 215,000; the "net $285k/yr" of the plan-shown row above | Illustrative economics |
| First answer | 33 s (median 49 s, slowest 101 s, across six other products) | 33 s: `stats.wall_seconds` 33.4 in `eval/results/plan-heatgun-2022-local-litellm.json`, the recorded heat-gun plan search (current origin and 6 moves, each predicted and checked; the "Full plan, heat gun" row above). 49 s and 101 s: `wall_seconds_median` 49.2 and `wall_seconds_max` 101.4 in `eval/results/families-r3-live-checks.json`, the six D661 live plan checks (the "Live plan checks, one per new family" row) | Measured wall time of a plan search, one run each. The heat-gun run is the earlier 6-move run (D84), not the production plan. The draft adds no model call. Replaces "about a minute, estimated, not measured" (judge panel round 16, item 5c) |
| Bar lengths in the before/after figure | Unde ÷ before, per row: 33 s ÷ 20 h = 1/2182; 4.6 s ÷ 21 days = 2.5 × 10⁻⁶; $0.0078 ÷ $3,600 = 2.2 × 10⁻⁶ | Drawn to scale per row; anything under 2 px is drawn at 2 px and the figure says so | Drawing only, no new figure |

The other figures in the story are existing rows: 20 hours, 21 days (n = 247), US$3,600, 3,628 ruling responses (market and time), p50 4.6 s and $0.0078 (evaluation), N345938 (plan shown).


## RCEP calculator (D690)

| Number | Value | Source | Status |
|---|---|---|---|
| Annex 3A rows parsed (HS 2022) | 2,452 rows, 96 chapters; 2,189 with a rule, of which 2,181 read by the parser and 8 hand-encoded; 263 rows carry no rule of their own; 0 unreadable | `coverage` in `backend/internal/rcep/data/psr_hs2022.json` (`eval/rcep/parse_psr.py build`); pinned by `TestTableCoverage` | Measured, no model. Parse coverage, not accuracy |
| Cross-check against a second official copy | 0 code or rule differences in 2,452 rows; 1 description difference (text extraction) | `eval/rcep/parse_psr.py diff` on the RCEP Secretariat and Australian Border Force PDFs; files and hashes in `eval/rcep/source.json` | Measured once, 22 Nov 2026 |
| Worked examples reproduced | 13 subtests from the ABF RCEP guide (as at 18 Dec 2024), e.g. build-up 45 %, build-down 50 %, spice mix 38.9 % and 44.4 %, de minimis 5 % by value and 4 % by weight | `TestABFWorkedExamples` in `backend/internal/rcep/engine_test.go` | Tests, not an evaluation. The figures are the guide's illustrations |
| RCEP check on published positives, development | 162 of 304 yes, 142 undetermined, 0 no or errors | `eval/rcep/japan-summary.json`, development | Measured offline (D691, D716, D717); positive recall |
| RCEP check on published positives, held out | 71 of 98 yes (72.4%, 62.9–80.3), 27 undetermined, 0 no or errors. The deciding criterion matched in 71 of 71 decisions (94.9–100). | `eval/rcep/japan-summary.json`, held_out | Measured once offline on published positives. Refusal performance remains unmeasured |
| RCEP check on all published positives | 233 of 402 yes, 169 undetermined; 232 of 233 deciding criteria matched | `eval/rcep/japan-summary.json`, overall | Descriptive pooled split; criterion match denominator is decisions with both labels |
| RCEP with material codes at the precision of the cited tariff shift | Development yes 161 of 213, criterion 161 of 161; held out yes 71 of 72, criterion 71 of 71 | `eval/rcep/japan-summary.json`, published_tariff_shift_with_material_codes | D691 target subset. No claim of overall eligibility accuracy |
| RCEP cited tariff scope agrees with engine row prefix | 293 of 293 development, 95 of 95 held out | `eval/rcep/japan-summary.json`, cited_tariff_scope_compatible | Scope check. No exact Annex 3A row label was published |
| RCEP synthetic negatives | 162 of 162 development and 71 of 71 held out changed from yes to undetermined; 0 explicit refusals | `eval/rcep/japan-summary.json`, synthetic_negatives | Synthetic tariff-code mutations, reported separately from published positives |

Rules: small-n numbers carry CIs; development-only numbers say "development set"; never "approved", "verified", "guaranteed", "cheapest" or "proof that it qualifies". Do not describe a percentage-point gap between subsets as proof of calibration.

## Market on every case: the deterministic market detector (no model call; 22 Nov; D680, D681, D682)

| Number | Value | Source | Status |
|---|---|---|---|
| Markets a case can name | 9 codes (us, eu, uk, ca, jp, kr, au, mx, other); 1 supported (us) | `backend/internal/jurisdiction/jurisdiction.go` (`Markets`, `Default`); `GET /v1/jurisdictions` | Built |
| U.S. rulings the detector reads as non-U.S. (`market-detect-v2`) | 0 of 649 development rulings of every regime (tuned on); 0 of 377 held-out rulings (run once). v1 flagged 3 of the 377 | `eval/results/market-detect-dev-d681-v2.json`, `market-detect-final-d681-v2.json`, `market-detect-final-d680.json` | Measured, deterministic. Weak hints: 71 development and 6 held-out, none on a ruling router-v4 labelled `unclear`, so none changes route |
| Non-U.S. questions read correctly, model-written probe set 2 (unseen) | v2: 22 of 30 exact market, 26 of 30 kept from a U.S. auto-answer; v1: 19 and 23. No text got a wrong non-U.S. market | `eval/results/market-detect-probes-probes-model-2-d681-v{1,2}.json`; probes `eval/jurisdiction/probes-model-2.jsonl` | Measured once each. Synthetic, illustrative questions written by gemini-3.8-flash, not customer text. Say "about three in four", never "detects non-U.S. questions" |
| Not independent: hand set and model set 1 | hand set 30 of 30 exact, 10 of 10 U.S. controls clean (v1 and v2; same author as the detector); model set 1 v1 16 of 30, v2 25 of 30 (read to write v2) | `eval/results/market-detect-probes-probes-{hand,model}-d68*.json` | Measured; do not quote as accuracy |
| Probe set cost | $0.0821 for both model-written sets (gateway-reported) | D680, D681 | Measured |

## Japan trade-agreement candidate (D712–D715)

| Measure | Value | Source | Status |
|---|---|---|---|
| Exact criterion on development | 243 of 400 (60.75%, 95% Wilson interval 55.88–65.41) | `eval/jp/jp-dev_eval-gemini-4-argon-jp-engine-v1-litellm-d712-dev-complete-metrics.json` | Measured. Includes cases the candidate could not decide. |
| Development outcomes and named origin | 244 qualifying, 155 undetermined, one failed. 51 of 111 named countries and 177 of 292 named origins including the EU | Same development metrics | Measured on published affirmative cases. |
| Development run cost and latency | $2.9794 total, $0.0074 per case, p50 6.971 s, p95 10.079 s among model-call cases | Same development metrics | Measured through the gateway. An interrupted attempt logged a further $0.0414 on 17 saved rows. |
| One-time unseen test | 633 positive labels, 251 no-call abstentions, 382 HTTP 403 responses, zero model results, $0 reported | `eval/jp/jp-held_out-gemini-4-argon-jp-engine-v1-litellm-metrics.json` and matching rows under `eval/jp/runs/` | Gateway failure. An accuracy estimate is unavailable. |
| Separate synthetic negatives | Eight fictional cases, eight HTTP 403 responses, zero model results, $0 reported | `eval/jp/jp-synthetic-gemini-4-argon-jp-engine-v1-litellm-metrics.json` and matching rows under `eval/jp/runs/` | Gateway failure. A refusal rate is unavailable. |

### Update: Japan D741 re-run (24 Nov 2026, #735)

Earlier offline evaluation, advisory and unreviewed. Japan is open on the live site (see "Japan on the live site" below); the figures in this table are the earlier engine evidence, superseded by D920 and D922. The D712–D715 rows above stay as recorded (gateway failures, unmeasured, not 0%).

| Measure | Value | Source | Status |
|---|---|---|---|
| Held-out criterion match | 260 of 633 (41.07%, 95% Wilson interval 37.31–44.95) | `eval/jp/jp-held_out-gemini-4-argon-jp-engine-v1-litellm-d741-heldout-1-metrics.json` and `eval/jp/runs/` rows | Measured, positive-only. All 212 abstentions, 4 errors and 363 undetermined stay in 633. Not general accuracy. |
| Held-out positive recall | 266 of 633 (42.02%, 38.24–45.90) | Same metrics | Measured. 260 criterion match plus 6 qualified with a different criterion. |
| Held-out outcomes | 266 qualified, 0 declined, 363 undetermined, 4 errors; 421 model attempts, 212 abstentions before a call, 4 `model_unavailable` | Same metrics | Measured. |
| Synthetic refusal | 8 of 8 (67.56–100) | `eval/jp/jp-synthetic-gemini-4-argon-jp-engine-v1-litellm-d741-synthetic-1-metrics.json` | Eight fictional cases only. Legacy `accuracy` field 0 does not mean 0% refusal. |
| Precedent pool | 197 of 863 cited precedents were earlier held-out rulings; 0 self, 0 future | Held-out rows | Chronological rolling pool. Earlier held-out rulings were retrievable, so the cases are not wholly fresh. |
| Cost | $4.1258349 conservative charge (430 attempts) | `eval/results/RUNLOG.jsonl` rows `d741-heldout-1`, `d741-synthetic-1` | $3.7019269 gateway-reported plus $0.423908 worst case for 4 unreported failures. Not an invoice or a future quote. |
| Latency | Held-out p50 7.701 s, p95 15.515 s; synthetic p50 8.040 s, p95 10.134 s | Same metrics | Model-call cases. |

## Japan on the live site (D909–D922, 26–27 Nov 2026, #909, #912)

Status: Japan is open on the live site. `GET /v1/jurisdictions` returns `jp` with `supported=true` (read 27 Nov 2026), and production runs it on the production engine model, `jp-engine-v1` with `gemini-4-argon`. D919 to D922 measured that same model and evidence through the LiteLLM gateway, one run each, no retries. The Accuracy page shows the rows marked "Accuracy page". Where a deployment configures no Japan engine (`JAPAN_ENGINE=off`, or no usable model), the registry reports Japan unsupported and `copy.japan.notAvailable` is the fallback line; it is not shown on the live site.

| Measure | Value | Source | Status |
|---|---|---|---|
| Development, right rule named | 243 → 272 of 400 (60.8% → 68.0%, 95% Wilson interval 63.3–72.4); 30 gained, 1 lost, exact McNemar p < 0.001 | D909, D910; `eval/jp/jp-dev_eval-gemini-4-argon-jp-engine-v1-litellm-d909-dev-metrics.json` | Measured. The development set was used to build, so not a blind test. Positive-only |
| Rule-text test, EU, UK, CPTPP and AJCEP Chapter 3 text added | 271 against 272 of 400: 0 gained, 1 lost; 0 of 80 negatives approved. Not shipped; adds about 20% to every request with no change in answers | D916, D918; `eval/jp/jp-dev_eval-gemini-4-argon-jp-engine-v1-litellm-d916-dev-metrics.json`, `…d916-neg-metrics.json` | Measured, one run. Result: no measurable effect (net +8 was required) |
| Constructed negatives approved (Accuracy page) | 0 of 80; v2 prompt also 0 of 80 and not adopted (269 of 400) | D913, D915; `eval/jp/jp-dev_neg-gemini-4-argon-jp-engine-v1-litellm-d913-neg-v1-metrics.json` | Measured on made-up cases, not real rulings. After the question card (D922) still 0 of 80 |
| Rulings after 22 Nov, right rule named (Accuracy page) | 37 of 44 (84.1%, 95% Wilson interval 70.6–92.1), the same in both pools | D919, D920; `eval/jp/jp-fresh_1126-gemini-4-argon-jp-engine-v1-litellm-d919-fresh-metrics.json` | Measured, one run, 44 Japan Customs rulings published after the 22 Nov snapshot (issued 14 May to 13 Nov 2026), all approvals. 41 of the 44 repeat an earlier request; with near-duplicates removed from the precedents the same 37 were right. At the development rates per agreement about 32 of 44 would be expected, so part of the rate is an easier mix. 7 misses: 5 undetermined, 2 qualify under a different rule than the ruling's. Cannot measure wrong approvals |
| Rulings after 22 Nov by agreement | RCEP 18 of 18, CPTPP 8 of 9, JP-Indonesia EPA 4 of 4, JP-UK CEPA 1 of 1, JP-EU EPA 6 of 11, AJCEP 0 of 1 | D920; same metrics file | Measured; small counts |
| Rulings after 22 Nov, after one round of the question card (Accuracy page) | 42 of 44 (95.5%, 84.9–98.7); 5 gained, 0 lost; JP-EU EPA 6 → 9 of 11 | D921, D922; `eval/jp/d922-answered-summary.json` | Measured, one run. Needs the user to answer the D917 card; the answers here are read from each ruling's own account of the facts. 2 wrong-rule approvals remain |
| Development after the question card (Accuracy page) | 272 → 374 of 400 (93.5%, 90.7–95.5); 102 gained, 0 lost, exact McNemar p < 0.001; 119 undetermined cases re-checked, 20 stay undetermined | D922; `eval/jp/d922-answered-summary.json` | Measured, one run. Same condition as the row above. Wrong approvals (approved under a rule other than the published one) 1 → 6; the good is still found originating in each |
| JP-EU EPA after the card | 128 → 179 of 181 development rulings | D922; same summary | Measured, same condition |
| Constructed negatives after the card | 0 of 80 approved before and after; of 18 re-checked, 6 declined and 12 undetermined | D922; same summary | Measured on made-up cases |
| Cost of these runs | $0.47 and $0.48 for the 44 (plus $0.08 translation); $1.75, $0.08, $0.26 after-card runs; $1.18 and $6.28 for D918 | D920, D922, D918; `eval/results/RUNLOG.jsonl` | Gateway-reported |
| Specialist review | Not done; tracked in #665 | `docs/scope-and-jurisdiction.md` | Say "review pending" next to any Japan figure |

## USMCA extended rules and ATIGA check (D927, D930, D932; both on since #910)

| Measure | Value | Source | Status |
|---|---|---|---|
| USMCA origin rules beyond the pilot (flag `USMCA_RULES_EXT`), held out (Accuracy page) | 21 of 23 decided products agree with CBP (91.3%, 95% Wilson interval 73.2–97.6). 23 of 59 products decided; 36 undecided, 0 decided by the pilot rows | D932; `eval/usmca/summary-held_out.json` | Measured, scored once on 46 CBP rulings dated 2026 and later (64 products; 4 with CBP's answer unclear and 1 not about eligibility skipped). Agreement on decided products only, not on all 59. Met the pre-registered bar (at least 85% on at least 15 decided) |
| USMCA development result | 118 of 127 decided products agree (92.9%, 87.1–96.2) | D930; `eval/usmca/summary-dev.json` | Not independent: the rows were built against this set |
| ATIGA check on authority worked examples (Accuracy page) | 7 of 7 examples reach the result the source states | D927; `eval/atiga/reference-cases.json`, run by `backend/internal/atiga/reference_test.go` | Not an accuracy rate: no ASEAN Member State publishes origin rulings, so none was found to test against. Worked examples (MITI Malaysia 1 to 5, Singapore Customs B and F), written before the check was run |
| AD/CVD exposure check with the scope reader, held out (Accuracy page; flags `ADCVD_CHECK` and `ADCVD_SCOPE_READER`, off) | Flagged orders covered the product 8/14 (57.1%, 32.6–78.6), against 8/16 (50.0%, 28.0–72.0) for the plain check on the same rulings; covered products flagged 8/18 (44.4%, 24.6–66.3) for both. Flagging every ruling: precision 40.9% | D942, D943; `eval/adcvd/heldout/summary.json` (`go run ./cmd/adcvdprec -check`, see D943) | Measured on 44 Commerce scope rulings made Jul 2023 to Dec 2024, order list frozen 30 Jun 2023, method registered before scoring; 57.1% is with a fix made after the result (registered method 8/13, 61.5%, 35.5–82.3). Three wrong flags removed at most, intervals overlap. The product description is model-written from the ruling and can carry the deciding facts, so the gain is an upper bound. Recall is capped by the codes the scope lists. Flags off |
| AD/CVD exposure check, earlier test, now seen (Accuracy page; flag `ADCVD_CHECK`, off) | Flagged orders covered the product 11/21 (52.4%, 32.4–71.7); covered products flagged 11/12 (91.7%, 64.6–98.5). Flagging every ruling: precision 34.3% | D940; `eval/adcvd/summary.json` (`go run ./cmd/adcvdeval -check`) | Measured, scored once on 27 Nov, on 35 Commerce scope rulings made 2025 and later against an order list frozen 31 Dec 2024. Intervals are wide. The product code is model-suggested, so its error is included. Goods made in a third country (n=7) are not seen. Flag off |

## Family depth: batteries, machinery, furniture, medical and optical (D780–D784)

| Measure | Value | Source | Status |
|---|---|---|---|
| Case universe | 1,223 substantial-transformation rulings dated before 2026 (dev 469, train 632, other 122); 751 held-out IDs excluded by ID | `eval/families/b2-hits.json` (`depth_b2.py`) | Computed, no model call. |
| Frozen kept rulings (pool + dev) | Batteries 6 + 7, machinery 16 + 11, furniture 10 + 1, medical and optical 20 + 12; ambiguous 26, 18, 5, 17 | `eval/families/b2-cases.json` | Hand screen; N354279 moved to ambiguous after the freeze (D782). |
| New templates | 0 of 4 tracks reached D780's bar of 10 citable pool rulings not served by a template (batteries 6, industrial machines 5, optical 2) | D781 | Decision by a pre-registered bar. |
| New citations | 16 rulings, each with its held-origin sentence | `eval/families/b2-citations.json` | Read by the agent, no model check. |
| BM25 top 5 holds a cited ruling (dev) | Batteries 0 → 7 of 7; machinery 3 → 3 of 11; medical 7 → 7 of 10; optical 1 → 1 of 2; furniture 0 → 0 of 1 | `eval/families/b2-retrieval.json` | Measured offline, descriptive; small n. |
| Family-check guard on dev rows | 1 of 349 matched rows changed (pet bed, furniture → none) | `eval/families/b2-guard-dev.json` | Measured offline on recorded v1 checks; the first run's 7 changes included 6 subject-parsing artifacts (D783). |
| Family count | 26 (unchanged) | `TestTemplateSetIsPinned` | Pinned. |
| Dev family check, frozen cases matched (base → #772) | Batteries 5 → 7 of 7; machinery 7 → 7 of 11; medical 9 → 9 of 10; optical 0 → 1 of 2; furniture 1 → 1 of 1 | `eval/families/b2-familycov.json` | Measured once per configuration, development only (D787). Superseded by D789: guard v2 run, kept as run. |
| Dev family check, look-alikes kept off their cue family | 31 → 32 of 33 (failed checks counted as not matched) | Same | Measured (D787). Superseded by D789 (guard v2 run). |
| Dev family check, all 469 | 350 → 348 matched; failed checks 4 → 10; guard took 1 match (pet bed) | `eval/results/families-coverage-dev-all-litellm-f26-b2{-base,}.jsonl` | Measured (D787); within run-to-run variation. Superseded by D789 (guard v2 run). |
| Dev family-check run cost and latency | $3.1427 for both runs; p50 4.0 / 4.4 s, p95 9.8 / 13.0 s per check | Same, RUNLOG rows `families-coverage-dev-all-litellm-f26-b2-base`, `…-f26-b2` | Token-priced at promo rates. |
| Dev family check with guard v3, frozen cases matched (base → #772) | Batteries 5 → 6 of 7 (one failed check); machinery 7 → 7 of 11; medical 9 → 10 of 10; optical 0 → 1 of 2; furniture 1 → 1 of 1 | `eval/families/b2-familycov-g3.json` | Measured once, development only (D790). Current. |
| Dev family check with guard v3, look-alikes kept off their cue family | 31 → 32 of 33 (failed checks counted as not matched) | Same | Measured (D790). Current. |
| Dev family check with guard v3, all 469 | 350 → 351 matched; failed checks 4 → 9; guard took 1 match (pet bed) | `eval/results/families-coverage-dev-all-litellm-f26-b2-g3.jsonl` | Measured (D790). Current. |
| Dev family check with guard v3, cost and latency | $1.6222; p50 5.2 s, p95 15.4 s per check; lane total $4.7649 for three runs | RUNLOG row `families-coverage-dev-all-litellm-f26-b2-g3` | Token-priced at promo rates. |

## Family follow-ups: batteries, lifting machines, furniture, optical instruments (D820–D828, #777–#780)

| Measure | Value | Source | Status |
|---|---|---|---|
| Case universe | 1,247 substantial-transformation rulings (dev 469, train 632, other 146, of which 24 dated 2026); 751 held-out IDs excluded by ID | `eval/families/b3-hits.json` (`depth_b3.py`) | Computed, no model call. |
| Kept pool rulings against D780's bar of 10, strict (D826) | Batteries 6 (4 short); lifting machines 6 of 11 kept, 5 already cited by the off-road template (4 short); optical and measuring instruments 10 kept, 9 citable (met); furniture served by its template | `eval/families/b3-amend-1.json` | D821's count of 11 for lifting was wrong; amended in D826. |
| Templates | 1 new: `optical_precision_instruments` (9 rulings, 20 citations, 6 groups); `offroad_vehicles_machinery` v2 (25 → 40 citations, 12 → 18 rulings, 4 → 6 groups); `display_camera_modules` v3 (scope note only) | `eval/families/b3-citations.json` | Every new pairing read against its held-origin sentence (D822, D826). |
| Frozen development cases (before → after) | Batteries 7 → 7; machinery 11 → 11; lifting (off-road template) 0 → 7; furniture 1 → 4; optical 2 → 9; medical 10 → 10; 48 in all, 30 look-alikes | `eval/families/b3-dev-splits-a1.json` (amends `b3-dev-splits.json`) | Frozen 24 Nov (D820, D821, D826); for #781. |
| BM25 top 5 / top 20 holds a cited ruling (dev, before → after) | Lifting (off-road template) 1 → 7 of 7 / 7 → 7 of 7; optical 1 → 3 of 9 / 5 → 8 of 9; furniture 3 → 3 of 4; others unchanged | `eval/families/b3-retrieval.json` | Measured offline, descriptive; small n. |
| Guard v5 on recorded development checks | v1 checks: 1 of 349 matches changed (pet bed); guard v3 run: 2 of 351 (the 3D printer; the optical window, now sent from the display template to review) | `eval/families/b2-guard-dev.json`, `eval/families/b3-guard-g3.json` | Measured offline, no model re-run (D827). |
| Family count | 28 (27 before D863: battery packs added) | `TestTemplateSetIsPinned` | Pinned. |

## Is the family work working? (D840–D842, #781)

| Measure | Value | Source | Status |
|---|---|---|---|
| Development cases matched to their template | Batteries 6 of 7 (49–97); machinery 6 of 11 (28–79); lifting (off-road) 7 of 7 (65–100); furniture 3 of 4 (30–95); optical 8 of 9 (57–98); medical 9 of 10 (60–98); all 39 of 48 (68–90) | `eval/families/b3-781-measure.json` (`dev_cases`); `eval/results/families-coverage-dev-all-litellm-f27-781{,-noorigin}.jsonl` | Measured once, development; the families were chosen and tuned on these cases |
| Development look-alikes kept off their cue family | 29 of 30 (83–99) | same (`lookalikes`) | Measured once, development |
| Held-out (377) with a plan template, 27 families | 302 (80.1%, 75.8–83.8); 5 failed checks; on the 368 both answered 300 against D661's 294 (9 gained, 3 lost, p = 0.146); optical 7, off-road 5 | same (`heldout_matching`, `heldout_vs_d661`); `eval/results/families-coverage-final-all-litellm-f27-781.jsonl` | Measured once, held out, pre-registered in D840. Scope, not accuracy; D661 is a different setup |
| Held-out origin accuracy by family | With a template 278 of 302 (92.1%, 88.5–94.6); none 62 of 75 (82.7%, 72.6–89.6); food 11 of 15 (73.3%); vehicle parts 44 of 51; cables 43 of 43; PCBA 32 of 32 | same (`heldout_origin`); `eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl` | The official 340/377 run re-sliced; no new calls. The engine reads no template |
| Development origin accuracy, the 48 cases | 41 of 45 scorable (91.1%, 79.3–96.5); machinery 9 of 11, medical 8 of 10, others all correct | same (`dev_origin`) | 44 recorded rows (Vertex) and 1 gateway run |
| Cost and latency | $1.6052; per check p50 5.6 s / p95 30.3 s (development), 4.4 s / 11.9 s (held-out) | `eval/results/RUNLOG.jsonl` | Measured, token-priced at promo rates |

## Machinery boundary (D850–D855, #785)

| Measure | Value | Source | Status |
|---|---|---|---|
| Development cases matched, after the fix (D853) | Machinery 10 of 11 (62–98, was 6); lifting 7 of 7; batteries 7 of 7; medical 10 of 10; optical 8 of 9; furniture 3 of 4; all 45 of 48 (83–98); look-alikes 29 of 30 | `eval/families/b3-852-measure.json`; `eval/results/families-coverage-dev-all-litellm-f28-852{,-noorigin}.jsonl` | Development only, **tuned on these cases** (two scope revisions); `family-check-v2`, guard v6 |
| Confirmation run (D855) | Machinery 10 of 11; medical 9 of 10; optical 9 of 9; all 45 of 48; look-alikes 29 of 30; 0 failed checks (output cap 4096); pre-registered no-regression criterion not met (one medical case, N356150) | `eval/families/b3-854-measure.json`, `b3-854-vs-852-measure.json` | Development only, tuned on these cases |
| Held-out family matching | 302 of 377 stands as measured in D842 with `family-check-v1` and guard v5; not re-run after this change | as above (D842) | No held-out figure for v2 |
| Cost | $1.0027 for six development runs | `eval/results/RUNLOG.jsonl` | Measured |


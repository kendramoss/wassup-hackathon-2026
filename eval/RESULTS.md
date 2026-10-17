# Origin Proof — evaluation results

All runs are listed in `results/RUNLOG.jsonl`; per-case rows are in `results/*.jsonl`. Data: `data/` and `splits/` (sha256 in `SHA256SUMS`).

> **Status (15 Oct 2026): the Vertex AI section is official.** Frozen config: engine-v1, BM25, K = 5, Gemini 4 Argon, router-v2 on Gemini 3.8 Flash, `GOOGLE_CLOUD_LOCATION=global`. The held-out split was scored once. Every section after that is still "proxy, development only" (LiteLLM). Do not put proxy numbers in the README results table or on slides.

## Origin engine — Vertex AI (official, 15 Oct 2026)

Same frozen config as the proxy run below. Scoring adjustments from `eval/data/scoring_adjustments.json` (D23). Fast path: confidence ≥ 0.92 and router-v2 in scope. Rows: `results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl`, `results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl`, `results/scope-*-router-v2-vertex.jsonl`.

| Split | n (after adjustments) | Accuracy (95% CI) | Fast path coverage | Fast path accuracy (95% CI) | Cost / case (promo · list) | Latency p50 / p95 |
|---|---:|---|---:|---|---|---|
| dev (development set) | 463 | 91.1% (88.2–93.4) | 368 / 463 = 79.5% | 96.5% (94.1–97.9) | $0.0079 · $0.0158 | 4.8 s / 8.1 s |
| **final** (held out) | 377 | **90.2% (86.8–92.8)** | 300 / 377 = 79.6% | **96.0% (93.1–97.7)** | $0.0078 · $0.0157 | 4.6 s / 5.7 s |

- Held-out labels: regex 321/355 = 90.4% (86.9–93.1); manual 19/22 = 86.4% (66.7–95.3). Adjustments excluded 0 held-out rows and 6 dev rows. Unadjusted dev is 422/469 = 90.0% (86.9–92.4); that is what `GET /v1/eval/summary` embeds.
- Router out of scope (all labelled substantial transformation): dev 8/469, final 11/377. One final router call (N362431) failed and was retried once; it is in scope. Not a second labeling.
- McNemar vs the proxy run, same adjustments: held-out Vertex fixed 2, broke 1, p = 1.000. Dev: fixed 3, broke 2, p = 1.000.
- Cost is the Developer API promo/list schedule used for the proxy runs. The Vertex price sheet was not re-checked. Nothing was changed after the held-out score.

### Calibration (final, Vertex AI)

| Confidence | n | correct | accuracy (95% CI) |
|---|---:|---:|---|
| ≥ 0.95 | 271 | 265 | 97.8% (95.3–99.0) |
| 0.90–0.95 | 64 | 50 | 78.1% (66.6–86.5) |
| 0.80–0.90 | 31 | 18 | 58.1% (40.8–73.6) |
| < 0.80 | 11 | 7 | 63.6% (35.4–84.8) |

## Auto-accept bar by product family (17 Oct 2026; D63) — not adopted

No model calls: the Vertex engine rows (`results/dev-` and `results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl`) joined with router-v2's subfamily (`results/scope-*-router-v2-vertex.jsonl`), scoring adjustments applied. Bars fitted **on dev only**: per router subfamily, the lowest confidence at which in-scope dev accuracy stays ≥ 95% at every bar keeping ≥ 20 cases; a subfamily without one keeps the global 0.92 (D32). The frozen bars were then applied **once** to the held-out split. Command: `go run ./cmd/eval calibrate` (writes `results/calibration-by-family.json`, `results/calibration-by-family.md` and `backend/internal/triage/family_bars.json`).

**Result: the per-family bars do not beat the global bar on held-out.** Final coverage 76.1% vs 79.6% and accuracy of auto-accepted answers 94.1% (90.7–96.3) vs 96.0% (93.1–97.7), with 17 wrong answers auto-accepted instead of 12. Neither paired difference is significant (coverage p = 0.130; wrong auto-accepts p = 0.227), but both point the wrong way, and the family-bar accuracy falls under the 95% the bars were fitted for. On dev itself the family bars only reshuffle cases (369 vs 368 auto, 17 vs 13 wrong). The low bars (motorised 0.75, electronics and metal_tools_parts 0.78) are set by a handful of low-confidence dev cases each; on held-out the metal_tools_parts bar let in 15 more cases of which 6 were wrong. `FAST_PATH_BY_FAMILY` stays off.

#### Bars fitted on dev (target 95%, at least 20 cases; else global 0.92)

| Subfamily | dev n (in scope) | bar | dev at bar | dev at global bar |
|---|---:|---:|---|---|
| apparel_textile_goods | 1 | 0.92 (global: fewer than 20 in-scope dev cases) | 1 / 1 = 100.0% | 1 / 1 = 100.0% |
| battery_solar | 4 | 0.92 (global: fewer than 20 in-scope dev cases) | 4 / 4 = 100.0% | 4 / 4 = 100.0% |
| electronics | 120 | 0.78 | 115 / 120 = 95.8% | 107 / 108 = 99.1% |
| filters | 38 | 0.95 | 22 / 23 = 95.7% | 24 / 26 = 92.3% |
| furniture_fixtures | 15 | 0.92 (global: fewer than 20 in-scope dev cases) | 14 / 15 = 93.3% | 14 / 15 = 93.3% |
| medical | 12 | 0.92 (global: fewer than 20 in-scope dev cases) | 8 / 9 = 88.9% | 8 / 9 = 88.9% |
| metal_tools_parts | 85 | 0.78 | 80 / 84 = 95.2% | 73 / 74 = 98.6% |
| motorised | 49 | 0.75 | 47 / 49 = 95.9% | 36 / 36 = 100.0% |
| other | 131 | 0.96 | 61 / 64 = 95.3% | 88 / 95 = 92.6% |

#### dev (the bars were fitted here), n = 463

| Policy | auto-accepted (coverage, 95% CI) | accuracy of auto (95% CI) | wrong auto-accepted |
|---|---|---|---:|
| global 0.92 | 368 / 463 = 79.5% (75.6–82.9) | 355 / 368 = 96.5% (94.1–97.9) | 13 |
| per-family bars | 369 / 463 = 79.7% (75.8–83.1) | 352 / 369 = 95.4% (92.7–97.1) | 17 |

Paired (exact McNemar on the cases the policies treat differently): auto only under family bars 35, only under global 34, p = 1.000; wrong answers auto-accepted only under family bars 9, only under global 5, p = 0.424.

| Subfamily | n | bar | global: auto correct / auto | family: auto correct / auto |
|---|---:|---:|---|---|
| apparel_textile_goods | 4 | 0.92 | 1 / 1 = 100.0% | 1 / 1 = 100.0% |
| battery_solar | 4 | 0.92 | 4 / 4 = 100.0% | 4 / 4 = 100.0% |
| electronics | 121 | 0.78 | 107 / 108 = 99.1% | 115 / 120 = 95.8% |
| filters | 38 | 0.95 | 24 / 26 = 92.3% | 22 / 23 = 95.7% |
| furniture_fixtures | 16 | 0.92 | 14 / 15 = 93.3% | 14 / 15 = 93.3% |
| medical | 12 | 0.92 | 8 / 9 = 88.9% | 8 / 9 = 88.9% |
| metal_tools_parts | 85 | 0.78 | 73 / 74 = 98.6% | 80 / 84 = 95.2% |
| motorised | 50 | 0.75 | 36 / 36 = 100.0% | 47 / 49 = 95.9% |
| other | 133 | 0.96 | 88 / 95 = 92.6% | 61 / 64 = 95.3% |

#### final (held out; bars frozen from dev, applied once), n = 377

| Policy | auto-accepted (coverage, 95% CI) | accuracy of auto (95% CI) | wrong auto-accepted |
|---|---|---|---:|
| global 0.92 | 300 / 377 = 79.6% (75.2–83.3) | 288 / 300 = 96.0% (93.1–97.7) | 12 |
| per-family bars | 287 / 377 = 76.1% (71.6–80.2) | 270 / 287 = 94.1% (90.7–96.3) | 17 |

Paired (exact McNemar on the cases the policies treat differently): auto only under family bars 25, only under global 38, p = 0.130; wrong answers auto-accepted only under family bars 8, only under global 3, p = 0.227.

| Subfamily | n | bar | global: auto correct / auto | family: auto correct / auto |
|---|---:|---:|---|---|
| apparel_textile_goods | 6 | 0.92 | 3 / 3 = 100.0% | 3 / 3 = 100.0% |
| battery_solar | 5 | 0.92 | 5 / 5 = 100.0% | 5 / 5 = 100.0% |
| electronics | 76 | 0.78 | 72 / 72 = 100.0% | 74 / 74 = 100.0% |
| filters | 1 | 0.95 | 1 / 1 = 100.0% | 1 / 1 = 100.0% |
| furniture_fixtures | 13 | 0.92 | 4 / 6 = 66.7% | 4 / 6 = 66.7% |
| medical | 8 | 0.92 | 6 / 6 = 100.0% | 6 / 6 = 100.0% |
| metal_tools_parts | 82 | 0.78 | 64 / 66 = 97.0% | 73 / 81 = 90.1% |
| motorised | 49 | 0.75 | 38 / 40 = 95.0% | 44 / 48 = 91.7% |
| other | 137 | 0.96 | 95 / 101 = 94.1% | 60 / 63 = 95.2% |

## Untuned Gemini 3.5 Flash — dev only (15 Oct 2026)

`gemini-4-argon` cannot be tuned (model page: Tuning not supported). This is the untuned `gemini-3.5-flash` baseline on the same dev split, engine-v1, BM25, K = 5, Vertex `global`. No held-out run. Fast path reuses the Argon run's router-v2 scope file. Rows: `results/dev-gemini-3.5-flash-prec-engine-v1-vertex.jsonl`.

| | Argon (dev, adjusted) | Flash 3.5 (dev, adjusted) |
|---|---|---|
| Accuracy | 91.1% (88.2–93.4), 422/463 | 90.5% (87.5–92.8), 419/463 |
| Fast path coverage | 79.5% (368/463) | 91.6% (424/463) |
| Fast path accuracy | 96.5% (94.1–97.9) | 92.0% (89.0–94.2) |
| Latency p50 / p95 | 4.8 s / 8.1 s | 12.1 s / 51.2 s |
| Cost / case | $0.0079 promo | $0.017 standard global |

- McNemar, Argon vs Flash, same adjustments: Flash fixed 16, broke 19, p = 0.736. Gap is 0.6 points, inside the ~3-point band.
- The jsonl `cost_usd` field is 0: the price table has no Flash 3.5 entry. The $0.017 figure is mean tokens (about 2,705 in / 1,453 out) times Vertex global standard $1.50 / $9.00 per 1M, output including reasoning, from the Agent Platform pricing page checked 15 Oct 2026. The 469-case run cost about $8.
- Flash's fast path is below the 95% accuracy the 0.92 bar was chosen for. Overall accuracy is close; the confident answers are not.
- Tuning was not launched. Supervised fine-tuning for Gemini 3.5 Flash is $0.01 per 1,000 training tokens ($10 per 1M). Anas: no Flash tuning before the freeze. The staging deploy of this baseline (health ok, case saves denied) is in D44.

## Textile regime (19 CFR 102.21) — Vertex AI (16 Oct 2026)

`textile-v1`, Gemini 4 Argon, plain BM25, K = 5, precedents limited to textile rulings dated before the case. Split frozen in `eval/splits/splits_textile.json` (dev 99, final 71). Dev was scored first. Held-out was scored once after that, with the prompt, K, and the split left as they were. Rows: `results/textile-dev-gemini-4-argon-prec-textile-v1-vertex.jsonl`, `results/textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl`. The "fast path" line in the raw eval report still uses the substantial-transformation bar of 0.92. The textile bar below is separate.

| Split | n | Accuracy (95% CI) | ≥ 0.95 coverage | ≥ 0.95 accuracy (95% CI) | Cost / case (promo · list) | Latency p50 / p95 |
|---|---:|---|---:|---|---|---|
| dev (Jul–Dec 2025) | 99 | 79.8% (70.8–86.5) | 67 / 99 = 67.7% | 95.5% (87.6–98.5) | $0.0092 · $0.0184 | 5.5 s / 68.1 s |
| **final** (held out, 2026) | 71 | **85.9% (76.0–92.2)** | 42 / 71 = 59.2% | **95.2% (84.2–98.7)** | $0.0093 · $0.0185 | 5.7 s / 47.7 s |

Substantial-transformation dev, same model and K, is 91.1% (88.2–93.4) on 463 adjusted cases. Textile dev is about 11 points lower; held-out is about 4 points under the substantial-transformation held-out of 90.2% (86.8–92.8). Both textile numbers sit well above a most-common-origin rule (always China, the dev majority: 30/99 = 30.3% on dev, 9/71 = 12.7% on held-out). The held-out majority class is Canada at 19/71 = 26.8%; that figure uses the held-out labels, so it is not a rule that could have been set beforehand. A where-assembled read of the facts — the country named on the sewing or assembly step, unextracted cases counted wrong — is 21/99 = 21.2% on dev and 19/71 = 26.8% on held-out.

### Calibration

| Confidence | dev n | dev correct | dev accuracy | final n | final correct | final accuracy (95% CI) |
|---|---:|---:|---|---:|---:|---|
| ≥ 0.95 | 67 | 64 | 95.5% (87.6–98.5) | 42 | 40 | 95.2% (84.2–98.7) |
| 0.90–0.95 | 19 | 10 | 52.6% (31.7–72.7) | 13 | 9 | 69.2% (42.4–87.3) |
| 0.80–0.90 | 6 | 3 | 50.0% (18.8–81.2) | 4 | 3 | 75.0% (30.1–95.4) |
| < 0.80 | 7 | 2 | 28.6% (8.2–64.1) | 12 | 9 | 75.0% (46.8–91.1) |

Cumulative accuracy stays at or above 95% down through confidence 0.95 on both splits (dev 64/67, held-out 40/42). The next grid point, 0.92, drops to 70/76 = 92.1% on dev and 45/49 = 91.8% on held-out. No held-out score falls in [0.93, 0.95), so a bar of 0.93 would select the same 42 cases. Proposed `TEXTILE_FAST_PATH=0.95` (42/71 cases). Zero call errors on both runs. Dev p95 is one slow case, not the typical call (p50 is 5.5 s).

### Dev errors (20)

The rule is the deciding rule the model named (`key_factors`). Three of these name 19 CFR 102.11 or substantial transformation instead of 102.21 (N349660, N352059, H335912); N352763 treats the good as a non-textile LED module. The prompt was not changed after this list.

| Ruling | Gold | Predicted | Confidence | Rule named |
|---|---|---|---:|---|
| N350055 | Pakistan | China | 0.96 | (c)(2) and (e)(2)(i) dyeing, printing, and two finishing operations |
| N349831 | Cambodia | China | 0.92 | (c)(4) most important manufacturing process (outer shell fabric formation) |
| N349660 | Mexico | United States | 0.85 | (c)(4); also 19 CFR 102.11(d)(3) essential character |
| N350421 | Indonesia | China | 0.90 | (c)(4) most important manufacturing process |
| H335912 | Canada | United States | 0.90 | 19 CFR 102.11(b)(1) essential character, via 102.17 |
| N350930 | Cambodia | China | 0.90 | (c)(4) most important assembly or manufacturing process |
| N351308 | Mexico | China | 0.90 | (c)(4) most important manufacturing process (caliper core) |
| N351508 | China | Indonesia | 0.90 | (c)(5) last country of important manufacturing process |
| N352059 | Mexico | Germany | 0.78 | Substantial transformation / 19 CFR 102.11(b)(1) |
| N352404 | Pakistan | China | 0.40 | (c)(2) fabric formation |
| N353243 | China | Pakistan | 0.85 | (c)(4) most important manufacturing process (outer shell fabric) |
| N353342 | China | Canada | 0.65 | (c)(2) tariff shift / substantial transformation in Canada |
| N353658 | Vietnam | China | 0.95 | (c)(4) most important manufacturing process |
| N353972 | Dominican Republic | United States | 0.92 | (c)(2) fabric formation |
| N355835 | Canada | China | 0.35 | (c)(2) tariff shift / assembly |
| N355801 | China | Pakistan | 0.85 | (c)(4) most important manufacturing process |
| N356882 | Vietnam | China | 0.92 | (c)(4) most important manufacturing process |
| N356886 | Vietnam | China | 0.90 | (c)(4) most important manufacturing process |
| N356766 | Uzbekistan | India | 0.98 | (c)(2) tariff shift (wholly assembled) |
| N352763 | Mexico | China | 0.75 | Substantial transformation (non-textile, LED module / PCBA) |

### Held-out errors (10)

| Ruling | Gold | Predicted | Confidence | Rule named |
|---|---|---|---:|---|
| N357199 | Vietnam | China | 0.92 | (c)(4) most important manufacturing process |
| N358446 | India | Uzbekistan | 0.90 | (c)(4) most important assembly or manufacturing process |
| N358577 | United States | Costa Rica | 0.90 | (c)(2) wholly assembled |
| N359056 | Mexico | Canada | 0.65 | (c)(2) tariff shift / substantial transformation via printing and framing |
| N359690 | Canada | Mexico | 0.65 | (c)(2) tariff shift |
| N361684 | United States | Thailand | 0.78 | (c)(2) / 102.20(r) tariff shift and substantial transformation |
| N360050 | Cambodia | China | 0.92 | (c)(4) most important manufacturing process |
| N357180 | Mexico | United States | 0.85 | (c)(4) most important assembly or manufacturing process |
| N364456 | China | Slovakia | 0.98 | (c)(2) tariff shift |
| N364457 | China | Slovakia | 0.95 | (c)(2) tariff shift |

Two held-out misses sit at or above 0.95, both China predicted as Slovakia under (c)(2). That is why the ≥ 0.95 band is 40/42 rather than 42/42, and why the Wilson lower bound is 84.2%.

## Origin engine — Go port check (engine-v1, gemini-4-argon, 5 BM25 precedents dated before each case) — proxy, development only

| Split | n | Accuracy (95% Wilson CI) | Research run (same split, Python) | Same answer as research | Conf ≥ 0.95: coverage / accuracy | Cost / case (promo · list) | Latency p50 / p95 |
|---|---:|---|---|---:|---|---|---|
| dev (Jul–Dec 2025) | 469 | 90.0% (86.9–92.4) | 90.2% | 456 / 469 | 72.1% / 95.9% (93.2–97.5) | $0.0079 · $0.0157 | 4.9 s / 6.1 s |
| **final** (held out, Jan–Sep 2026) | 377 | **89.9% (86.5–92.6)** | 89.1% (85.6–91.9) | 369 / 377 | 71.9% / 97.8% (95.3–99.0) | $0.0078 · $0.0156 | 5.0 s / 6.4 s |

- Acceptance (build prompt, Gate B): final within ~2 points of 89.1% through the same proxy — **met** (+0.8 points; 5 cases newly correct, 2 newly wrong vs the research run).
- `final` was scored once with the frozen engine-v1 configuration (run log entry with `held_out: true`). Nothing was changed because of it.
- Labels: 355 regex-labelled (90.1%) and 22 hand-labelled (86.4%) on final.
- Precedents: same 5-ruling set as the research run on 375 / 377 final cases (`TestResearchParity`); facts text byte-identical on all 2,790 rulings.
- Cost uses Gemini Developer API prices verified 2026-10-14 (promo: Argon $2 / $10 per 1M in/out tokens until 2026-11-30; list $4 / $20). Vertex prices not yet checked. Latency is through the proxy.

## Calibration (final, proxy)

| Confidence | n | correct | accuracy (95% CI) |
|---|---:|---:|---|
| ≥ 0.95 | 271 | 265 | 97.8% (95.3–99.0) |
| 0.90–0.95 | 71 | 53 | 74.6% (63.4–83.3) |
| 0.80–0.90 | 27 | 17 | 63.0% (44.2–78.5) |
| < 0.80 | 8 | 4 | 50.0% (21.5–78.5) |

## Fast path (triage) — router-v2 (gemini-3.8-flash), proxy
Rule: auto-accept when engine confidence ≥ 0.95 **and** the router says the case is in scope (docs/DECISIONS.md D7). Router-v2 was tuned on dev only, then frozen and run once on final.

| Split | Router out of scope (all cases are substantial transformation) | Fast path coverage | Fast path accuracy (95% CI) |
|---|---|---:|---|
| dev | 7 / 469 (1.5%) | 334 / 469 = 71.2% | 95.8% (93.1–97.5) |
| final (held out) | 10 / 377 (2.7%) | 265 / 377 = 70.3% | **97.7% (95.1–99.0)** |

Without the router (confidence ≥ 0.95 alone), final covers 71.9% at 97.8%. The research brief's "65% at 98.0%" came from a different routed pipeline and does not describe this rule.

**Scope recall (dev window Jul–Dec 2025, up to 40 per gold regime):** routed out of scope — Part 102 tariff-shift 36/40 (90%), pharma API 35/36 (97%), substantial transformation 2/40. Full table: `results/scope-window-dev-router-v2.md`.

## Accuracy experiments (16 Oct) — dev only, proxy; none adopted

Paired against the engine-v1 BM25 dev run (90.0%, 422/469). Exact McNemar on discordant cases.

| Variant (dev, n=469) | Accuracy | Fixed / broke vs baseline | p | Cost / case |
|---|---|---|---|---|
| Baseline: BM25, 5 precedents | 90.0% (86.9–92.4) | — | — | $0.0079 |
| Hybrid: BM25 top-30 re-ranked by gemini-embedding-001 | 88.9% (85.7–91.4) | 7 / 12 | 0.36 | $0.0078 |
| Pure embedding retrieval | 89.8% (86.7–92.2) | 5 / 6 | 1.00 | $0.0077 |
| Voting: 5 calls over rotated precedent orders when confidence < 0.95 | 89.6% (86.5–92.0) | 2 / 4 | 0.69 | $0.0169 |

- Scoring fix: 4 dev rulings hold different origins per scenario (`data/multi_scenario.json`, mechanical rule). Excluding them: dev 90.5%; held-out has none (89.9% unchanged). Reported, not a model change.
- Conclusion: better-matched precedents and repeated asking do not change the engine's errors; they are judgment errors (raw-material thresholds, set/kit character, which component is the essence), not retrieval or sampling noise. The frozen configuration stays engine-v1 + BM25, already scored once on held-out (89.9%), so no new held-out run was made.
- Side finding: first-call confidence varies between runs through the proxy (≥ 0.95 coverage 338 vs 369 on the same dev cases), so fast-path coverage is a ±3-point number.

## Scoring adjustments and clarify review (16 Oct)
- Verified scoring adjustments (`data/scoring_adjustments.json`, D23): dev 90.9% (421/463); held-out unchanged 89.9% (339/377).
- Clarify review on 127 dev cases below 0.95 (proxy): engine error rate 26% (16–39) when no precedent is on point vs 22% (14–33) when one is; 24% with a conflict; 28% routed needs_info vs 21% needs_review. No error signal; kept for its questions and conflict display (D24). Rows: `results/clarify-dev-clarify-v1-litellm.jsonl`.
- Learnability audit of the 47 dev errors (`results/learnability-dev.jsonl`): ~12 learnable from earlier rulings, 13 where earlier rulings point to the model's answer, 9 with no precedent, ~8 scoring/label (now adjusted). In 22 errors the deciding ruling was already among the engine's 5 precedents.

## Split votes and follow-up answers (15 Oct) — dev only, proxy

`go run ./cmd/eval torn`: re-vote (5 calls) the 117 dev cases with first-call confidence < 0.95 and keep the **14 torn** cases (runner-up origin got ≥ 2 of 5 calls). For each, clarify-v1 asks questions aimed at the split, a simulated requester (requester-v1) answers from the ruling text before CBP's HOLDING (HQ rulings) or from the facts section only (NY rulings, which state extra facts only inside CBP's reasoning), quoting the file for every answer, and a guard replaces answers that state a conclusion with "Not stated." Then the engine re-votes with the answers appended. Calls are deterministic: re-voting the original facts (control) gives the identical split every time.

| Vote on the 14 torn cases | majority = CBP (95% CI) | settled (≥ 4 of 5 agree) |
|---|---|---|
| before / control (original facts) | 8/14 = 57% (33–79) | 0/14 |
| after answers, "Not stated." lines included | 5/14 = 36% (16–61) | 10/14 |
| after answers, answered questions only | 5/14 = 36% (16–61) | 3/14 |
| after answers, answers-v2 (hedge sentences removed, neutral header) — **adopted** | 7/14 = 50% (27–73) | 5/14 (2 on CBP's origin, 3 not) |

- Including "Not stated." lines: 1 fixed, 4 broken (exact McNemar p = 0.375). In 3 of the 4 breaks every answer was "Not stated."; the engine reads missing information as evidence against the processing country and settles on the imported part's country.
- Answered questions only: 0 fixed, 3 broken (p = 0.25). The breaks came from answers that only restated facts already given (facts-only NY files) or hedged ("whether they are pre-blended is not stated").
- answers-v2 (`clarify.AppendAnswers`, used by the API and the eval): sentences that only report missing information ("…is not explicitly stated") are removed, and the header tells the engine the answers weigh the same as the facts and that unaddressed points are not evidence. 0 fixed, 1 broken (p = 1.0). The remaining break (N351538, impeller castings) received only factual answers: the engine treats the Chinese casting as decisive where CBP held the Malaysian machining (66–85% of material removed) was — the same raw-material judgment error as in the main error analysis, not an answer-format effect.
- 23 of 33 answers were "Not stated.": for most split cases the file holds no extra fact for the requester to give. Tiny sample, wide CIs; the direction is negative in both variants. Torn cases are rare (14 of 117 low-confidence dev cases).
- Rows: `eval/results/torn-scan-dev-vote5-litellm.jsonl` (117-case scan), `torn-dev-clarify-v1-requester-v1-litellm.jsonl`, `torn-dev-clarify-v1-requester-v1-answered-only-litellm.jsonl`, `torn-dev-clarify-v1-requester-v1-answers-v2-litellm.jsonl`. Cost ≈ $11.70 (promo).

## Written rules for the confident errors (15 Oct) — dev only, proxy; not adopted

`run -split dev -rules` appends rules-v1 (`engine.Rules`: three statements of CBP practice on chemical/mineral processing, blanks and preforms, and essential character in assemblies) to engine-v1's system prompt. Same retrieval, single call.

| Dev (n = 463 after adjustments) | accuracy (95% CI) | fast path (≥ 0.95, in scope) |
|---|---|---|
| engine-v1 | 90.9% (88.0–93.2) | 336 cases, 96.4% |
| engine-v1 + rules-v1 | 91.1% (88.2–93.4) | 341 cases, 96.5% |

- 10 fixed, 9 broken (exact McNemar p = 1.0). Of the 5 confident judgment errors the rules targeted, 2 were fixed (borax hydration, wheel preform), both now below the 0.95 bar.
- The breaks are the rules applied where CBP did not: blending of Chinese ingredients in Australia ("no new compound") where CBP held Australia; tissue rolls cut to a toilet-seat shape ("generic material formed") where CBP held Indonesia; carbide-tipped tools and strut assemblies ("functional component") where CBP weighed other factors. The same pattern as the shared-playbook negative result: general rules trade one set of errors for another.
- No held-out run: no dev gain. Rows: `eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-rules-v1.jsonl`. Cost ≈ $3.90 (promo).

## CBP reasoning shown with each precedent (15 Oct) — dev only, proxy; not adopted

`run -split dev -reasoning` adds, under each of the 5 precedents, up to 900 characters of CBP's own application of the test (`corpus.Reasoning`: the sentences leading to the ruling's last conclusion before the HOLDING section, legal standard and citations removed; attached for 2,537 of 2,553 precedents). Facts, holding and retrieval unchanged.

| Dev (n = 463 after adjustments) | accuracy (95% CI) | fast path (≥ 0.95, in scope) | cost per case |
|---|---|---|---|
| engine-v1 | 90.9% (88.0–93.2) | 336 cases, 96.4% | $0.0084 |
| engine-v1 + reasoning-v1 | 91.4% (88.4–93.6) | 346 cases, 96.2% | $0.0095 |

- 7 fixed, 5 broken (exact McNemar p = 0.77). None of the 9 confident errors reviewed for D27 changed: the engine kept the same answer at the same or higher confidence.
- No held-out run: no dev gain. Rows: `eval/results/dev-gemini-4-argon-prec-engine-v1-litellm-reasoning-v1.jsonl`. Cost ≈ $4.50 (promo).

## Planner and verifier (Gate C) — proxy, development only

### Move verifier on development flip pairs (54 CBP natural experiments dated before 2026; 46 changed, 8 unchanged)
DEVELOPMENT SET ONLY. The verifier prompt's burden-of-proof sentence was written after the research team saw the held-out 2026 flip failures, so no held-out test exists yet (needs rulings published after 25 Sep 2026).

| Mode | Precision when it says "changes origin" | Recall | Specificity | Precedent-supported correct | Cost / check |
|---|---|---|---|---|---|
| verifier-v1 (A's origin not given; research setting) | 34/34 = 100% (90–100) | 34/46 = 74% (60–84) | 8/8 = 100% (68–100) | 27/27 (88–100) | $0.032 |
| verifier-v1+known-a (CBP's origin for A stated) | 38/38 = 100% (91–100) | 38/46 = 83% (69–91) | 8/8 = 100% (68–100) | 35/35 (90–100) | $0.031 |

Research (Python, same 54 pairs): 32/32 precision, 70% recall, 8/8 specificity. CIs are Wilson 95% on small n; sampling only, not a deployment guarantee. Rows: `results/flips-dev-verifier-*.jsonl`.

### Demo plan checks (fixtures adjusted during development — not unbiased evaluations)
| Fixture | Result | Full-plan cost |
|---|---|---|
| Heat gun 2022 (N328954) → Thailand, precedents before 14 Jan 2026, CBP's 2022 holding (China) given as current origin | Shown: make motor-fan assembly + PCBA in Thailand (supporting N347421, counter N345859, verifier 0.95). Current setup (final assembly only in Thailand) rejected. Backtest: CBP's actual 2026 change (N357771) also moved knob, power cord, back cover, outer shells. | 7 engine calls + 6 verifier runs (36 tool calls), 95.6k tokens, $0.22 promo / $0.45 list, 33 s |
| LiDAR scenario I (N359371) → Vietnam, precedents before 3 Mar 2026 | RX/TX-only move rejected (engine: stays Thailand). All four core PCBAs: precedent-supported (N339727) but verifier confidence 0.92 < 0.95, so not shown. Contaminated fixture (verifier prompt written after seeing the LiDAR flip failures). | 4 engine calls + 1 verifier run, $0.06 promo |

Engine on the heat-gun **current setup**: Thailand 0.78 from the structured product, China 0.78 from CBP's own 2022 facts text; CBP held China. A borderline case, below the 0.95 fast path either way (routed to review).

## Not yet measured
- Official Vertex AI numbers (blocked on GCP access).
- Held-out test of the verifier and planner (needs CBP rulings published after 25 Sep 2026).

## Textile prompt variants on dev (LiteLLM proxy, 16 Oct; D53)

Same 99 dev cases as D51; proxy runs compare with each other, not with the Vertex rows.

| Variant | Accuracy (95% CI) | ≥ 0.95: n, accuracy | vs textile-v1 (McNemar) | Cost / case |
|---|---|---|---|---|
| textile-v1 | 78/99 = 78.8% (69.7–85.7) | 67, 95.5% | — | $0.0091 |
| textile-v1, K = 8 | 79/99 = 79.8% (70.8–86.5) | 68, 94.1% | — | $0.0114 |
| textile-v1 + precedent reasoning | 84/99 = 84.8% (76.5–90.6) | 66, 95.5% | fixed 9, broke 3, p = 0.15 | $0.0109 |
| **textile-v2 + precedent reasoning** | **85/99 = 85.9% (77.7–91.4)** | 66, 95.5% | **fixed 8, broke 1, p = 0.039** | $0.0109 |

Chosen on dev with the held-out already spent on v1, so no independent test yet (D53).

### Same comparison on Vertex (16 Oct 2026)

`go run ./cmd/eval run -regime textile -textile-prompt v2 -reasoning -split dev`, then `eval compare` against `results/textile-dev-gemini-4-argon-prec-textile-v1-vertex.jsonl`. Held-out was not run.

| | Vertex textile-v1 | Vertex textile-v2 + reasoning |
|---|---|---|
| Accuracy | 79/99 = 79.8% (70.8–86.5) | **86/99 = 86.9% (78.8–92.2)** |
| ≥ 0.95 | 64/67 = 95.5% | 66/68 = 97.1% (89.9–99.2) |
| McNemar vs v1 | — | fixed 8, broke 1, p = 0.039 |
| Cost / case | $0.0092 promo | $0.0111 promo · $0.0222 list |
| Latency p50 / p95 | 5.5 s / 68.1 s | 7.2 s / 76.6 s |

Rows: `results/textile-dev-gemini-4-argon-prec-textile-v2-vertex-reasoning-v1.jsonl`. Five calls returned 429 or 504 and were retried; the file has one row per ruling. `TEXTILE_PROMPT` stays v1.


## USMCA / Part 102 rules on dev (LiteLLM proxy, 17 Oct; D61)

**Proxy, development only.** `go run ./cmd/eval run -regime usmca -split dev` (labels `data/labels_usmca.jsonl`, splits `splits/splits_usmca.json`, facts in `backend/data/facts`). 72 dev cases (2024–2025), K = 5 BM25 precedents drawn only from labelled Part 102 rulings dated before each case. Held-out (32, 2026) was not run.

| | engine-v1 (substantial transformation) | **usmca-v1 (102.11 / 102.20)** |
|---|---|---|
| Accuracy | 52/72 = 72.2% (61.0–81.2) | **54/72 = 75.0% (63.9–83.6)** |
| ≥ 0.95 | 32/34 = 94.1% (80.9–98.4) | 36/37 = 97.3% (86.2–99.5) |
| McNemar vs engine-v1 | — | fixed 6, broke 4, p = 0.75 |
| Cost / case (promo) | $0.0081 | $0.0099 |
| Latency p50 / p95 | 6.0 s / 8.7 s | 6.8 s / 12.9 s |

Always-Mexico is 28/72 = 38.9%, always-Canada 24/72 = 33.3%. Rows: `results/usmca-dev-gemini-4-argon-prec-usmca-v1-litellm.jsonl` and `results/usmca-dev-gemini-4-argon-prec-engine-v1-litellm.jsonl` (same pool and cases, engine-v1 prompt).

Of the 18 usmca-v1 misses, 3 are label problems rather than model errors: H345343 and H342474 (the two manual rows, copied from `labels.jsonl`) hold the Section 301 origin, while CBP's marking origin, the question Part 102 answers, is Mexico in both; N342959's "evidence" is a facts sentence ("German origin Module frames"), not a holding. 12 of the 18 predict the country that made a main material or component (South Korea for cast covers, China for film and spouts, Germany for coated paper, the United States for cereal and potato flakes) where CBP found the tariff shift met in Mexico or Canada: the model under-applies (a)(3).

## Pharmaceutical regime (active ingredient) — LiteLLM proxy, development only (17 Oct 2026; D62)

`pharma-v1`, Gemini 4 Argon, plain BM25, K = 5, precedents limited to pharmaceutical (`pharma_api`) rulings dated before the case. Split frozen in `eval/splits/splits_pharma.json` (pool 187, dev 35 = Jul–Dec 2025, final 56 = 2026). **Proxy, development only**: not an official number; re-run on Vertex AI. Held-out was not run. The "fast path" line in the raw eval report uses the substantial-transformation bar of 0.92; the pharma numbers below are separate.

| Run (dev, 35) | Accuracy (95% CI) | ≥ 0.95 coverage | ≥ 0.95 accuracy (95% CI) | Cost / case (promo) | Latency p50 / p95 |
|---|---|---:|---|---|---|
| **pharma-v1** | **33/35 = 94.3% (81.4–98.4)** | 32/35 = 91.4% | **32/32 = 100% (89.3–100)** | $0.0070 | 6.0 s / 20.1 s |
| engine-v1 (substantial-transformation prompt, all precedents) | 33/35 = 94.3% (81.4–98.4) | 34/35 = 97.1% | 32/34 = 94.1% (80.9–98.4) | $0.0064 | 5.5 s / 13.5 s |

Always-India (the dev majority) is 13/35 = 37.1%. McNemar pharma-v1 vs engine-v1: fixed 1, broke 1. The two prompts are equally accurate here; the difference is calibration. engine-v1's two misses are at 0.98 (N351305 Malaysia for Malaysia/Vietnam; N353822 Brazil for United States, the cough drops whose US menthol is the active ingredient). pharma-v1 gets N353822 right and its two misses are below 0.95.

Cumulative accuracy by confidence (pharma-v1, dev): ≥ 1.00 15/15; ≥ 0.99 30/30; ≥ 0.95 32/32 = 100% (89.3–100); ≥ 0.90 33/34 = 97.1% (85.1–99.5); ≥ 0.85 33/35 = 94.3%. The lowest confidence with ≥ 95% dev accuracy is **0.90** (33/34, 34 of 35 cases). No score falls between 0.90 and 0.95, so 0.91–0.95 select the same 32 cases. Zero call errors.

Dev misses (2), both Dermasil creams whose CBP holding is "Malaysia or Vietnam" (bulk cream made in either country): N351305 predicted Malaysia at 0.90, N351352 predicted Malaysia at 0.85. These are cosmetics, not drugs, and the router (Vertex dev-window scope file) labels them `substantial_transformation`, so in production they would not reach the pharma engine. The router labels the other 33 dev rulings `pharma_api`.

Commands: `go run ./cmd/eval run -regime pharma -split dev`; the engine-v1 row is `eval run -splits ../eval/splits/splits_pharma.json -split dev` (written as `dev-...-engine-v1-litellm.jsonl` and renamed with a `pharma-` prefix so it does not overwrite the main dev file). Rows: `results/pharma-dev-gemini-4-argon-prec-pharma-v1-litellm.jsonl`, `results/pharma-dev-gemini-4-argon-prec-engine-v1-litellm.jsonl`. The `git_revision` in RUNLOG is a local scratch merge with the LiteLLM provider, not a pushed commit.

## Pharmaceutical and USMCA held-out (LiteLLM proxy, 17 Oct; D61, D62)

Scored once each with the frozen prompt (same `gemini-4-argon` as Vertex).

| Regime | Held-out accuracy (95% CI) | Confidence ≥ 0.95: n, accuracy (CI) | Cost / case |
|---|---|---|---|
| Pharmaceutical (`pharma-v1`, 56 rulings, 2026) | 50/56 = 89.3% (78.5–95.0) | 49 (88%), 100% (92.7–100) | $0.0066 |
| USMCA / Part 102 (`usmca-v1`, 32 rulings, 2026) | 25/32 = 78.1% (61.2–89.0) | 15 (47%), 100% (79.6–100) | $0.0096 |


# Unde Labs — evaluation results

All runs are listed in `results/RUNLOG.jsonl`; per-case rows are in `results/*.jsonl`. Data: `data/` and `splits/` (sha256 in `SHA256SUMS`).

> **Status (15 Oct 2026): the Vertex AI section is official.** Frozen config: engine-v1, BM25, K = 5, Gemini 4 Argon, router-v2 on Gemini 3.8 Flash, `GOOGLE_CLOUD_LOCATION=global`. The held-out split was scored once. Later sections are labelled individually as **measured gateway**, **proxy, development only**, or **reported**; use the status labels in [docs/NUMBERS.md](../docs/NUMBERS.md) for slide eligibility. An unlabelled proxy number is not slide-ready.

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

ECE, Brier and a reliability diagram for these rows are in `docs/EVALUATION.md` §4.2 (built by `python3 -I eval/charts/build_evaluation.py .`).

| Confidence | n | correct | accuracy (95% CI) |
|---|---:|---:|---|
| ≥ 0.95 | 271 | 265 | 97.8% (95.3–99.0) |
| 0.90–0.95 | 64 | 50 | 78.1% (66.6–86.5) |
| 0.80–0.90 | 31 | 18 | 58.1% (40.8–73.6) |
| < 0.80 | 11 | 7 | 63.6% (35.4–84.8) |

## Second held-out split, model sweep and calibration layer (21 Nov 2026; D150–D157)

All numbers below are built by `python3 -I eval/charts/build_accuracy.py .` into `results/accuracy-v2-metrics.json` from the committed rows; no model calls. New runs went through an OpenAI-compatible LiteLLM gateway to Gemini (the "gateway"; `LLM_PROVIDER=litellm`, eval only, D151), with temperature 0, seed 20261017 and a 120 s call timeout. Gateway and Vertex AI agree on the development set (same engine and model, exact McNemar p = 0.58, D151); these runs were not repeated on Vertex AI. Scoring adjustments (D23) as above.

### Model sweep, development set (n = 463 after adjustments; D152, D153)

| Arm | Accuracy (95% CI) | Failed calls | Fast path (≥ 0.92, in scope) | Cost / case | p50 / p95 | vs Argon on Vertex |
|---|---|---:|---|---|---|---|
| Gemini 4 Argon, Vertex AI (in use) | 91.1% (88.2–93.4) | 0 | 79.5% at 96.5% | $0.0079 | 4.8 s / 8.1 s | — |
| Gemini 4 Argon, gateway | 91.8% (88.9–94.0) | 0 | 81.0% at 95.7% | $0.0079 | 6.8 s / 10.8 s | fixed 8, broke 5, p = 0.58 |
| Gemini 4 Carbon, gateway | 88.1% (84.9–90.8) | 27 | 81.9% at 96.3% | $0.0155 | 13.0 s / 52.2 s | fixed 6, broke 20, p = 0.009 |
| Gemini 3.1 Pro preview, gateway | 87.3% (83.9–90.0) | 10 | 83.6% at 92.8% | $0.0178 | 10.6 s / 24.7 s | fixed 12, broke 30, p = 0.008 |
| Gemini 3.8 Flash, Vertex AI (17 Oct run) | 90.1% (87.0–92.5) | 0 | 87.7% at 93.1% | $0.0050 | 5.5 s / 20.4 s | fixed 13, broke 18, p = 0.47 |
| Argon + rules-v2 prompt, gateway (D157) | 90.9% (88.0–93.2) | 0 | 77.8% at 96.7% | $0.0086 | — | fixed 9, broke 10, p = 1.0 |
| Majority of Argon, Carbon, 3.1 Pro (computed) | 91.6% (88.7–93.8) | 0 | 76.2% at 96.3% | $0.0432 | 14.3 s / 55.3 s | fixed 8, broke 6, p = 0.79 |

- Rows: `results/dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl`, `results/dev-gemini-4-carbon-prec-engine-v1-litellm.jsonl`, `results/dev-gemini-3.1-pro-preview-prec-engine-v1-litellm.jsonl`, `results/dev-gemini-4-argon-prec-engine-v1-litellm-rules-v2.jsonl`. Gateway cost is the cost the gateway reported per case.
- Carbon's 27 failed calls spent the whole 8,000-token output budget thinking. On the 436 cases it answered it matches Argon on the gateway: 408 against 406 right, p = 0.77. 3.1 Pro returned 10 empty answers and is lower on the cases it answered (404 against 416 on 453, p = 0.05).
- Against Argon on the gateway: rules-v2 fixed 9, broke 13, p = 0.52.
- Candidate pre-registered before any held-out scoring (D153): engine-v1 on Argon, unchanged, plus the calibration layer.

### Second held-out split (D150, D154): 83 rulings, 28 Sep to 13 Nov 2026, scored once

| Split | n | Accuracy (95% CI) | Fast path | Fast path accuracy (95% CI) | Path |
|---|---:|---|---|---|---|
| final (Jan–Sep 2026) | 377 | 90.2% (86.8–92.8) | 300 = 79.6% | 96.0% (93.1–97.7) | Vertex AI (official, 15 Oct) |
| **final2 (Sep–Nov 2026)** | 83 | **92.8% (85.1–96.6)** | 75 = 90.4% | 97.3% (90.8–99.3) | gateway, router-v2 on 3.8 Flash |
| both, pooled | 460 | 90.7% (87.6–93.0) | 375 = 81.5% | 96.3% (93.8–97.8) | mixed (above) |

- Rows: `results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl` (run log revision dfc712b, the commit that froze the split; 49a103b after the branch was rebased onto main). $0.0098 per case (gateway-reported); p50 7.5 s, p95 12.3 s. Labels: regex 75/81, manual 2/2.
- The 6 errors: CBP held China and the engine named the assembling country in 4 (N365621, N365280, N365997, N366380), the reverse in 1 (N365606), and Costa Rica against the United States in 1 (N365362).
- The split is easier for simple rules than the first: the most-mentioned country is right on 71.1% (60.6–79.7) of it against 60.7% (55.7–65.5) on the first (`results/baselines-final2.json`, `results/baselines-final.json`). It also has near-duplicate rulings: 83 rulings, 62 distinct subjects, the largest group 7 (wrapping paper). Counting each subject once, accuracy is 91.9%.

### Calibration layer (D156): isotonic map fitted on the dev rows, applied unchanged

| Held-out set | ECE before | ECE after | Brier before | Brier after | Mean stated before → after (accuracy) |
|---|---|---|---|---|---|
| final, 377 | 0.059 (0.041–0.091) | 0.030 (0.020–0.059) | 0.081 (0.059–0.105) | 0.072 (0.056–0.092) | 94.7% → 90.4% (90.2%) |
| final2, 83 | 0.067 (0.039–0.126) | 0.038 (0.030–0.099) | 0.060 (0.021–0.109) | 0.051 (0.020–0.090) | 96.1% → 93.9% (92.8%) |
| pooled, 460 | 0.055 (0.041–0.085) | 0.027 (0.017–0.053) | 0.077 (0.057–0.098) | 0.069 (0.053–0.085) | 95.0% → 91.0% (90.7%) |

ECE uses one bin per emitted confidence value; intervals are bootstrap 95% (2,000 resamples, seed 20261017). This is cal-v1, eval only. The product uses cal-v2 (next section).

**Read the ECE drop with care (D262, #397, #493).** The map is only weakly monotone. On the 377 it merges 11 stated levels into 6 (0.72–0.88 → 0.6136, 42 cases; 0.98–1.00 → 0.99, 180 cases); on final2 9 become 6, and on the pooled rows 11 become 6. The "before" and "after" ECEs above therefore use different bins on every split. Scoring the stated confidences on the calibrated partition gives the common-bin figure below, so a known share of each drop is bin merging, not better probabilities:

| Held-out set | Stated → calibrated levels | ECE, stated on own bins | ECE, stated on calibrated bins | ECE, calibrated on own bins | Share of drop from bins |
|---|---:|---:|---:|---:|---:|
| final, 377 | 11 → 6 | 0.0585 | 0.0528 | 0.0300 | 20% |
| final2, 83 | 9 → 6 | 0.0667 | 0.0614 | 0.0375 | 18% |
| pooled, 460 | 11 → 6 | 0.0547 | 0.0500 | 0.0273 | 17% |

The fields are `heldout_stated_on_calibrated_bins`, `heldout.final2.calibration_layer.stated_on_calibrated_bins` and `heldout.pooled_final_final2.calibration_layer.stated_on_calibrated_bins` in `results/accuracy-v2-metrics.json`; level counts and per-level ties are beside each. Brier is bin-independent and unchanged by this caveat. There is no calibrated cut equivalent to a stated 0.99 or 1.00 bar; use stated confidence there.

### All regimes, held-out, both windows (D210)

Built by `python3 -I eval/charts/build_accuracy.py .` into `results/accuracy-v2-metrics.json` (`heldout_all_regimes_by_window`); recomputed in Go by `go run ./cmd/claimscheck` (`regimes.*`). Wilson 95% intervals.

The post-cutoff splits of the textile, pharmaceutical and Part 102 engines were built from the 163 labelled rulings of the second window with each first-window builder's rule, unchanged (`splits/build_regimes_final2_split.py`; `splits/splits_{textile,pharma,usmca}_final2.json`, `data/labels_usmca_final2.jsonl`). They were pinned in `SHA256SUMS` and committed (cffff80, 21 Nov 17:18 +08:00) before any run, and each was scored once through the gateway with the production engine of its regime, Gemini 4 Argon, BM25, K = 5, router-v2 on Gemini 3.8 Flash. Every run is in `results/RUNLOG.jsonl` with that revision.

| Regime | Window 1: Jan–Sep 2026 | Path | Window 2: 28 Sep–13 Nov 2026 | Path |
|---|---|---|---|---|
| Substantial transformation, engine-v1 | 340/377 = 90.2% (86.8–92.8) | Vertex AI | 77/83 = 92.8% (85.1–96.6) | gateway |
| Textile (19 CFR 102.21 rule) | 61/71 = 85.9% (76.0–92.2), textile-v1 | Vertex AI | 15/17 = 88.2% (65.7–96.7), textile-v2 (in production) | gateway |
| Pharmaceutical, pharma-v1 | 50/56 = 89.3% (78.5–95.0) | proxy | 4/6 = 66.7% (30.0–90.3) | gateway |
| Part 102 marking origin, usmca-v1 | 25/32 = 78.1% (61.2–89.0) | proxy | 11/13 = 84.6% (57.8–95.7) | gateway |
| Splits added (as the 536 was) | 476/536 = 88.8% (85.9–91.2) | mixed | 107/119 = 89.9% (83.2–94.1) | gateway |
| **Each ruling once** | 451/509 = 88.6% (85.6–91.1) | mixed | 96/107 = 89.7% (82.5–94.2) | gateway |

| All regimes, all held-out windows | n | Accuracy (95% CI) |
|---|---:|---|
| Splits added | 655 | 583 = 89.0% (86.4–91.2) |
| **Each ruling once** | 616 | **547 = 88.8% (86.1–91.1)** |

- **The 536 counted 27 rulings twice.** The textile rule (D51) matches "102.21" anywhere in a ruling, and Part 102 rulings on non-textile goods name 102.21 in their boilerplate. 27 of the 32 first-window Part 102 rulings are also among the 71 textile ones; in window 2, 12 of the 13 Part 102 rulings are among the 17 textile ones. The rule was kept so the windows compare. "Each ruling once" counts those rulings under Part 102, a rule fixed before the runs (D210). On the overlap the textile engine did better than the Part 102 engine (window 1: 25 against 21 of 27; window 2: 11 against 10 of 12), so this choice gives the lower pooled figure. Of the 17 window-2 textile rulings, only 3 apply 102.21 itself, and all 3 were right.
- **Pharmaceutical has 6 rulings.** 12 were labelled pharmaceutical; 4 hold no origin (tariff classification only) and 2 are steel tubes made to "API 5CT" that the regime rule mislabelled, left out before the run. 4 of 6 right says little either way.
- **Misses, window 2:** textile N365940 (gold China, said Canada, 0.55) and N366728 (France, said China, 0.65); pharmaceutical N365255 (India, said South Korea, 0.40) and N366376 (Australia, said China, 0.90); Part 102 N366698 (Mexico, said Brazil, 0.95) and N366728 (France, said Canada, 0.65).
- **Routing went worse than these numbers suggest.** These runs send every case to its regime's engine. The router then in production (router-v2) sent 7 of the 13 Part 102 rulings, 1 of the 6 pharmaceutical rulings and 1 of the 3 rulings that apply 102.21 to the substantial-transformation engine instead (`router_regime` on each row). On the development window it had kept 88.4% of pharmaceutical and Part 102 rulings out of the general engine; on this small post-cutoff set it kept 11 of 19. The routed path is now scored, and the router replaced: see the next section (D230–D233).
- **Cost:** $0.41 for the three runs (gateway-reported): textile $0.0139, pharmaceutical $0.0101, Part 102 $0.0085 per case.
- **Mixed paths.** Window 1 mixes Vertex AI and the earlier LiteLLM proxy; window 2 is the gateway. Textile window 1 is textile-v1; window 2 is textile-v2, the prompt in production.
- **Limits of "held out" and of the intervals.** Held out means not used to tune anything; it is not a sealed set. Within each window, earlier rulings can be precedents for later ones (docs/EVALUATION.md 2.3), BM25 statistics use the whole corpus, and model pre-training exposure cannot be excluded. Each figure is one scored run; the Wilson intervals treat that run's rows as independent and leave out run-to-run, label-error and split-construction uncertainty, and near-duplicate rulings.

## Router: sending each case to the right legal test (21 Nov 2026; D230–D233)

All runs through an OpenAI-compatible LiteLLM gateway to Gemini, router on Gemini 3.8 Flash, `eval route` (rows in `results/route-*.jsonl`, every run in `results/RUNLOG.jsonl`). Splits: `splits/splits_router.json` (`splits/build_router_split.py`, built from the committed engine splits without reading any model output, pinned in `SHA256SUMS`). Each ruling carries the engine of its legal test ("gold engine"): pharmaceutical split → pharmaceutical engine; Part 102 split → Part 102 engine; textile split and the ruling applies 102.21 itself → textile engine; the general engine's splits → general engine. Correct = the router's label picks that engine. Wilson 95% intervals; paired tests are exact McNemar. Recomputed by `go run ./cmd/claimscheck` (`router.*`).

**Why the development figure misled.** The 88.4% "kept out of the general engine" on the Jul–Dec 2025 window counted 43 rulings, 36 of them pharmaceutical. That window holds few Part 102 rulings on non-textile goods. Adding the 72 Part 102 development rulings of 2024–2025 shows router-v2 sent only 36 of 70 to the Part 102 engine.

**Error analysis (router-v2, development).** 33 Part 102 rulings went to the general engine: 26 name Canada or Mexico as where the good is made or finished, and only 3 name USMCA or NAFTA. router-v2's prompt told the model not to pick Part 102 because a plant is in Canada or Mexico. Its one-line reasons explain the route (assembly, commodity, scenario), never the regime. No general-engine development ruling names USMCA or NAFTA (0 of 547), against 34 of 70 Part 102 rulings. The 2 pharmaceutical misses were scenario requests. Of the 11 general rulings sent elsewhere, 8 went to the textile engine (nonwoven pads, wipes, tie-down straps, a towel). Post-cutoff, read only to describe: of the 8 misses among 19 pharmaceutical and Part 102 rulings, 4 facts texts ask about "trade programs" and name no country of processing, 1 names Canada, 2 are made in France or Slovenia with no Part 102 cue, and 1 is a dietary supplement.

**Development set (before 2026; 725 rulings with a gold engine), paired with router-v2.** Pass rule written first (D230): specialist routing up with p < 0.05, all-rulings routing up, general routing at most 1.0 point lower, cost at most 1.5×.

| Router | All | Specialist | General | Part 102 | Pharmaceutical | Textile | Cost per case | Pass |
|---|---|---|---|---|---|---|---|---|
| router-v2 (before) | 677/725 = 93.4% (91.3–95.0) | 141/178 = 79.2% | 536/547 = 98.0% | 36/70 | 33/35 | 72/73 | $0.00214 | — |
| router-v3 | 695/725 = 95.9% (94.2–97.1) | 167/178 = 93.8% | 528/547 = 96.5% | 65/70 | 33/35 | 69/73 | $0.00229 | no: general −1.5 points (p = 0.039) |
| **router-v4** | **697/725 = 96.1% (94.5–97.3)** | **163/178 = 91.6%** | **534/547 = 97.6%** | 57/70 | 34/35 | 72/73 | $0.00231 | **yes** (all p = 0.0005; specialist p < 0.0001; general −0.4 points, p = 0.73) |
| router-v4 on Gemini 4 Carbon | stopped after 25 rulings | | | | | | about $0.006 | no: 2.9× cost, one answer cut off at 2,000 tokens |

router-v3 changed the prompt: the regime decided apart from the route; Part 102 for USMCA, NAFTA or trade-program questions and for goods whose last production step is in Canada or Mexico, unless the question is Section 301 or procurement; textile limited to chapters 50–63; and a check after the model (facts naming USMCA or NAFTA → Part 102). It sent 9 general rulings on goods finished in Canada or Mexico to Part 102 and 3 textile rulings on surgical sponges to the general engine. router-v4 states the textile scope as 102.21 does (chapters 50–63 plus listed headings such as 3005, 4202, 6501–6505, 9404) and adds a second check: a Part 102 label on facts with no classification, heading, marking, trade-program or USMCA wording becomes substantial transformation.

**Held-out, scored once each (D232, D233).**

| Window | Router | All | Specialist | General | Part 102 | Pharmaceutical | Textile |
|---|---|---|---|---|---|---|---|
| Jan–Sep 2026 | router-v2 | 488/505 = 96.6% (94.7–97.9) | 122/128 = 95.3% | 366/377 = 97.1% | 26/32 | 56/56 | 40/40 |
| Jan–Sep 2026 | **router-v4** | **490/505 = 97.0% (95.2–98.2)** | **125/128 = 97.7% (93.3–99.2)** | 365/377 = 96.8% | 29/32 | 56/56 | 40/40 |
| 28 Sep–13 Nov 2026 | router-v2 (D211 labels) | 96/105 = 91.4% (84.5–95.4) | 13/22 = 59.1% (38.7–76.7) | 83/83 | 6/13 | 5/6 | 2/3 |
| 28 Sep–13 Nov 2026 | **router-v4** | **101/105 = 96.2% (90.6–98.5)** | **19/22 = 86.4% (66.7–95.3)** | 82/83 | 11/13 | 6/6 | 2/3 |

Window 1: fixed 5, broke 3, p = 0.73 (all); the adoption check (all higher, general at most 1.0 point lower) holds, so **router-v4 is the production router**. Window 2: specialist fixed 6, broke 0, p = 0.031. In D211's terms router-v4 kept 17 of the 19 post-cutoff pharmaceutical and Part 102 rulings away from the general engine (router-v2: 11). router-v2's window-1 labels for the 377 general-engine rulings are the official Vertex AI scope run, the other 132 a gateway run; its cost there is not comparable.

**The routed path, post-cutoff, all regimes, each ruling once (107 rulings; `eval routed`).** Each ruling is answered by the engine its router label picks. Where that engine already scored the ruling in D211 its row is used; the 13 other (ruling, engine) pairs were run once (`results/*-routed-v2.jsonl`, `results/*-routed-v4.jsonl`).

| Path | Right | Sent to another engine | Of those, right anyway |
|---|---|---:|---:|
| Each ruling to its own engine (D211, the best case) | 96/107 = 89.7% (82.5–94.2) | 0 | — |
| Routed, router-v2 (the product until now) | 94/107 = 87.9% (80.3–92.8) | 11 | 6 |
| **Routed, router-v4 (the product now)** | **95/107 = 88.8% (81.4–93.5)** | 6 | 4 |

- **What it shows.** Misrouting cost router-v2 2 of 107 rulings end to end, less than the routing table suggests, because the general engine often names the same country as the Part 102 engine. router-v4 recovers one of them: it fixed 2 Part 102 rulings and broke 1 pharmaceutical ruling that the general engine had answered right (the pharmaceutical engine missed it). The two routed paths do not differ significantly on 107 rulings.
- **What went worse.** router-v4's general routing is lower on every set (development −0.4, window 1 −0.3, window 2 −1.2 points): goods finished in Canada or Mexico that CBP decided under substantial transformation now go to the Part 102 engine more often. Gemini 4 Carbon cost too much to finish.
- **Not done.** The window-1 routed path (it needs engine runs on every misrouted window-1 pair). A Vertex AI re-run of the router. The official engine figures (fast path, held-out accuracy) were measured with router-v2 and are not re-run.
- **Cue provenance (added after D233).** The "trade programs" cue in router-v3/v4 was written after the post-cutoff misses were read for error analysis (D230); development gives it no support (2 of 729 dev rulings and 2 of 509 window-1 rulings use the phrase, against 6 of 107 post-cutoff rulings). Four of the 6 specialist fixes (N365695, N366014, N366147, N366728) and both end-to-end fixes (N365695, N366014) are "trade programs" rulings; development gives the cue no support either (1 of its 70 Part 102 rulings uses the phrase). Remove all six cue rulings and the post-cutoff advantage disappears: specialist 14/17 for router-v4 against 12/17 for router-v2 (fixed N366376 and N366722, broke 0, exact McNemar p = 0.5) and the routed path 90/101 against 91/101 — router-v4 now trails by one. Both exclusions are recomputed by `go run ./cmd/claimscheck` (`router.final2.nocue.*`, `router.w2.routed.nocue.*`). **The post-cutoff router and routed-path figures above are therefore not clean held-out evidence for router-v3/v4.** The adoption decision used window 1, where the phrase is nearly absent.
- **Spend:** $6.83 in all (gateway-reported).

## Calibrated confidence in the product (21 Nov 2026; D160, D161)

No model calls. `python3 -I eval/charts/build_accuracy.py .` fits cal-v2 on the dev rows of engine-v1 on Argon (Vertex AI), writes it to `backend/internal/calibrate/map.json` (embedded in the server) and applies it once, unchanged, to the committed held-out rows (`results/accuracy-v2-metrics.json`, `calibration_v2`). cal-v2 is the D156 fit with blocks of fewer than 20 dev cases pooled first (D160), so no step rests on one case.

| Stated confidence | 0.65–0.89 | 0.90–0.91 | 0.92–0.94 | 0.95 | 0.96–0.97 | 0.98 | 0.99–1.00 |
|---|---|---|---|---|---|---|---|
| Calibrated (dev cases right) | 0.60 (27/45) | 0.7778 (35/45) | 0.90 (36/40) | 0.9016 (55/61) | 0.9355 (29/31) | 0.99 (113/114) | 0.99 (127/127) |

Values under 0.65 take the lowest step; 0.99 is a cap.

| Held-out set | Mean stated → calibrated (accuracy) | ECE stated → cal-v2 (95% CI) | ECE stated on the map's bins (levels) | Brier stated → cal-v2 (95% CI) |
|---|---|---|---|---|
| final, 377 (Vertex AI) | 94.7% → 90.2% (90.2%) | 0.059 → 0.028 (0.020–0.060) | 0.0528 (11 → 6) | 0.081 → 0.072 (0.056–0.092) |
| final2, 83 (gateway) | 96.1% → 93.8% (92.8%) | 0.067 → 0.037 (0.030–0.099) | 0.0614 (9 → 6) | 0.060 → 0.051 (0.020–0.089) |
| pooled, 460 | 95.0% → 90.9% (90.7%) | 0.055 → 0.026 (0.018–0.053) | 0.0500 (11 → 6) | 0.077 → 0.069 (0.053–0.085) |

cal-v2 pools confidence levels in the same way as cal-v1, so its ECE pairs also compare different bins: about 19%, 18% and 16% of the three drops come from bin merging. The shared-bin figures and level counts are in `results/accuracy-v2-metrics.json` (`calibration_v2.*.stated_on_calibrated_bins` and `.level_counts`) and are served on /accuracy beside the two errors. Brier is bin-independent. cal-v1 on the same rows: ECE 0.030, 0.038, 0.027. The two maps are level.

**Auto-accept bar (rule in D160, fitted on dev).** Lowest map step with in-scope dev accuracy ≥ 96% on ≥ 100 cases: **0.90**. It accepts exactly the cases stated 0.92 accepted, so nothing moves:

| Split | Auto-accepted (calibrated ≥ 0.90, in scope) | Right (95% CI) | Wrong |
|---|---|---|---:|
| dev, 463 (fitted here) | 368 = 79.5% | 96.5% (94.1–97.9) | 13 |
| final, 377 | 300 = 79.6% | 96.0% (93.1–97.7) | 12 |
| final2, 83 | 75 = 90.4% | 97.3% (90.8–99.3) | 2 |
| pooled, 460 | 375 = 81.5% | 96.3% (93.8–97.8) | 14 |

The next step down (0.7778, stated 0.90) would take 89.0% of dev at 94.4%, under the 96% target; the next up (0.9016, stated 0.95) 70.8% at 97.3%. `go run ./cmd/claimscheck` recomputes the auto-accept counts, means and ECE for dev and final in Go from the map and the rows, and fails if the calibrated and stated bars ever accept different cases.

## China-cluster adjudicator — dev only, not adopted (21 Nov 2026; D163, D164)

A second, focused call for the largest error group: CBP holding China (where components or sub-assemblies were made) against the assembling country. `go run ./cmd/eval adjudicate` reads committed stage-1 rows (engine-v1 on Argon, Vertex AI, dev) and, for triggered cases only, asks Gemini 4 Argon through the gateway one question with contrastive precedents: the 3 closest earlier rulings where CBP held China despite later work elsewhere and the 3 closest where the later work conferred origin (BM25, dated strictly before the case). It sees the two candidate countries, not the engine's answer. Its answer replaces the engine's when it names a candidate. Pass rule written first (D163): net fixed minus broke ≥ +5 on the 463, exact McNemar p < 0.1, cost ≤ 1.5× stage 1. Summary: `results/adjudicator-metrics.json` (`python3 -I eval/charts/build_adjudicator.py .`, no model calls).

| Variant | Triggered | Changed | Accuracy (95% CI) | Fixed / broke vs stage 1 | p | Cost / case (× stage 1) | Fast path (≥ 0.92, in scope) | Passes D163 |
|---|---:|---:|---|---|---|---|---|---|
| Stage 1: engine-v1, Vertex AI | — | — | 91.1% (88.2–93.4) | — | — | $0.0079 | 368 at 96.5% | — |
| 1. Narrow: engine named a non-China country, facts name China, confidence < 0.96 | 105 | 9 | 90.1% (87.0–92.5) | 2 / 7 | 0.18 | $0.0098 (1.24×) | 367 at 96.5% | no |
| 2. Symmetric: facts name China and another country, confidence < 0.95 | 115 | 19 | 90.9% (88.0–93.2) | 9 / 10 | 1.0 | $0.0100 (1.26×) | 367 at 96.5% | no |
| 3. Variant 2, only moves away from China kept (computed from its rows; chosen after 1 and 2) | 115 | 10 | 92.0% (89.2–94.1) | 7 / 3 | 0.34 | $0.0100 (1.26×) | 368 at 96.5% | no |

- **What happened.** When the adjudicator moved an answer *to* China it was wrong 7 times in 9 (variant 1) and 7 in 9 (variant 2). Moves *away* from China were right 7 times in 10. On dev, the group it was built for (gold China, engine said the assembling country, under 0.96) has 8 cases; it fixed 2. CBP's own close calls in this group go both ways, and a contrastive prompt did not separate them.
- **Variant 3** is the best dev result but fails the rule (net +4, p = 0.34), and it was chosen after seeing variants 1 and 2, so its +4 is optimistic. It also points the other way from the held-out error cluster (where CBP held China), so there was no reason to expect it to help there.
- **No held-out split was scored.** None passed, so by D163 neither the 377 nor final2 was touched, and nothing is wired into the server.
- Rows: `results/dev-gemini-4-argon-prec-engine-v1-vertex+adjudicate-v1-narrow-b096-litellm.jsonl`, `…-symmetric-b095-litellm.jsonl`; a 40-case probe before variant 1 (13 calls, same configuration) is `…-narrow-b096-litellm-probe.jsonl`. Every run is in `RUNLOG.jsonl`. Cost is the gateway-reported cost; calls the gateway answered from its cache (identical requests in variants 1 and 2, and the probe) are priced from their tokens at the promo rate. Gateway spend for all three runs: $1.32.

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

## Tuned Gemini 3.5 Flash — dev only (17 Oct 2026)

Supervised fine-tuning of `gemini-3.5-flash` on the Argon engine's CBP-reasoning targets (D56). 3 epochs, `us-central1`. Job `projects/88903435351/locations/us-central1/tuningJobs/3897104692752154624` (`origin-engine-cbp-reasoning-20261016-1811`) succeeded 2026-10-17T00:29:25Z. Endpoint `projects/88903435351/locations/us/endpoints/2563603719136477184`. Last logged step (87, end of epoch 3): training loss 0.7506042718887329, validation loss 0.6046217679977417. Dev is engine-v1, BM25, K = 5, Vertex location `us`. The response `model_id` field is `gemini-3.5-flash@default` (the base version string the endpoint returns); the calls used the endpoint above. Fast path reuses the Argon run's router-v2 scope file. Rows: `results/dev-gemini-3.5-flash-tuned-prec-engine-v1-vertex.jsonl` (one row per ruling; earlier 504s were retried). No held-out run.

| | Argon (dev, adjusted) | Untuned Flash 3.5 (dev, adjusted) | Tuned Flash 3.5 (dev, adjusted) |
|---|---|---|---|
| Accuracy | 91.1% (88.2–93.4), 422/463 | 90.5% (87.5–92.8), 419/463 | 87.9% (84.6–90.6), 407/463 |
| Fast path coverage | 79.5% (368/463) | 91.6% (424/463) | 94.0% (435/463) |
| Fast path accuracy | 96.5% (94.1–97.9) | 92.0% (89.0–94.2) | 89.4% (86.2–92.0) |
| Latency p50 / p95 | 4.8 s / 8.1 s | 12.1 s / 51.2 s | 6.0 s / 17.1 s |
| Cost / case | $0.0079 promo | $0.017 standard global | $0.0248 tuned us, 1.5× |

| Confidence | n | correct | accuracy (95% CI) |
|---|---:|---:|---|
| ≥ 0.95 | 440 | 394 | 89.5% (86.3–92.1) |
| 0.90–0.95 | 12 | 11 | 91.7% (64.6–98.5) |
| 0.80–0.90 | 5 | 2 | 40.0% (11.8–76.9) |
| < 0.80 | 0 | 0 | 0.0% (0.0–0.0) |

- McNemar, same adjustments. Tuned vs Argon: tuned fixed 15, broke 30, p = 0.036. Tuned vs untuned Flash: tuned fixed 12, broke 24, p = 0.065. Unadjusted tuned dev is 406/469 = 86.6% (83.2–89.4).
- Cumulative accuracy is at or above 95% only at confidence ≥ 0.99 (148/150 = 98.7%, CI 95.3–99.6). No row sits in [0.96, 0.99). At 0.98 the cumulative set is 305/323 = 94.4%, under 95%.
- Held-out was not run. Adjusted dev is 3.2 points under Argon (the gate is within 2), and the 0.92 fast path is 89.4%, worse than untuned Flash's 92.0%.
- Six calls returned prose with no JSON object and are counted wrong: H344538, N351672, N352670, N355167, N355168, N356144. Example: `engine: predict H344538: llm: unusable model response: no JSON object in response (1514 chars)`. Earlier attempts at the default 30 s timeout failed with `llm: model provider unavailable after 4 attempts: vertex: HTTP 504: Deadline expired before operation could complete.` Those were retried at `LLM_TIMEOUT=120s`.
- `cost_usd` is 0: the price table has no entry for this endpoint. The $0.0248 figure is mean tokens on the 457 successful final calls (2,702 in / 1,221 out) times 1.5× Gemini 3.5 Flash non-global ($1.65 / $9.90 per 1M), so $2.475 / $14.85 per 1M, because a tuned model is served on the `us` multi-region. Those 457 calls are about $11.34. Failed attempts before a later success are not in that figure.

## Base gemini-3.8-flash — dev only (17 Oct 2026)

The fallback model. Not the tuned endpoint above, which scored worse than this base model and was undeployed the same day (D73). Engine-v1, BM25, K = 5, Vertex `global`. Fast path reuses the Argon router-v2 scope file. No held-out run. Rows: `results/dev-gemini-3.8-flash-prec-engine-v1-vertex.jsonl` (last row per ruling; N350517, N350963 and N354291 first returned HTTP 504 and succeeded on a resume at `LLM_TIMEOUT=120s`).

| | Argon (dev, adjusted) | gemini-3.8-flash (dev, adjusted) |
|---|---|---|
| Accuracy | 91.1% (88.2–93.4), 422/463 | 90.1% (87.0–92.5), 417/463 |
| Fast path ≥ 0.92 coverage | 79.5% (368/463) | 87.7% (406/463) |
| Fast path ≥ 0.92 accuracy | 96.5% (94.1–97.9), 355/368 | 93.1% (90.2–95.2), 378/406 |
| ≥ 0.95 band | 97.3% (94.9–98.6), 324/333 | 93.0% (90.1–95.1), 384/413 |
| Latency p50 / p95 | 4.8 s / 8.1 s | 5.5 s / 21.6 s |

McNemar, same adjustments: Flash fixed 13, broke 18, p = 0.473. Unadjusted 418/469 = 89.1% (86.0–91.6). `FALLBACK_FAST_PATH` stays 0: a fallback answer goes to review even when confidence would clear 0.92.

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
Rule at the time: auto-accept when engine confidence ≥ 0.95 **and** the router says the case is in scope (docs/DECISIONS.md D7). Router-v2 was tuned on dev only, then frozen and run once on final.

> **Historical bar.** This proxy table uses the old 0.95 bar. D32 lowered the bar to 0.92 the same day, and the product, `eval report` and the Vertex section above all use 0.92. The same proxy rows at 0.92 (unadjusted): dev 371 / 469 = 79.1% at 353 / 371 = 95.1% (92.5–96.9); final 296 / 377 = 78.5% at 286 / 296 = 96.6% (93.9–98.2). The official figures are the Vertex ones at the top of this file.

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

## Image import on synthetic BOM images (gateway, 21 Nov 2026; D167, D168) — illustrative

`go run ./cmd/mediacheck` with `LLM_PROVIDER=litellm`, Gemini 4 Argon, prompt `extract-image-v1`, one run per set. Synthetic renderings of illustrative BOMs, not real documents. A part counts when its name is read exactly (case and spacing ignored) and its country matches.

| Set | Images | Parts | Names read | Countries right | Extra rows | Cost (gateway) | Rows |
|---|---:|---:|---|---|---:|---:|---|
| 1, plain tables | 5 | 48 | 45 | 45 | 1 | $0.079 | `results/media-v1-gemini-4-argon-litellm.json` |
| 2, varied layouts, photos, scans | 15 | 122 | 106 | 106 | 34 | $0.173 | `results/media-varied-gemini-4-argon-litellm.json` |

All 19 misses are exact-name mismatches with the country right where the row was found: 13 names with the part number appended, 3 from one multi-part row (split into its parts, or cut short with "…" in a set 1 image), 1 row split into PCBA and LED, 2 plant rows counted as parts in set 1's manifest. The returned rows are in `results/media-v1-diagnostic-gemini-4-argon-litellm.json` and `results/media-varied-diagnostic-gemini-4-argon-litellm.json` (re-runs of the imperfect images to keep the rows; answered from the gateway cache, same answers, not scores). The varied run's `git_revision` is marked dirty because the set 1 result file was not yet committed; the code and images were committed (bb563b5).

## Not yet measured
- Held-out test of the verifier and planner (needs CBP rulings published after 25 Sep 2026).
- Run-to-run variance of the official engine numbers (one scored run each). The official Vertex AI numbers, once listed here as blocked on GCP access, are measured: see the top of this file.
- Calibration statistics were added on 21 Nov from the committed rows, with no model calls: see `docs/EVALUATION.md` §4.2 and `results/evaluation-metrics.json`.

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


## Planner product scope: family coverage (LiteLLM gateway, 21 Nov; D138)

What share of substantial-transformation rulings the product's family check (`familycheck`, gemini-3.8-flash, match at confidence ≥ 0.6) maps to a family that has a plan template: the three templates before D138 ("legacy") and all 13 after. Input per ruling: its subject and the first 1,500 characters of its facts. This measures scope, not accuracy. The ten new families were chosen and written from the 845 pool rulings outside the dev, held-out and pilot splits (`results/families-selection.json`). The held-out split was scored once per set, after the templates (`856e215`) and the 1,024-token output cap (`6cd3516`) were committed. Every run is in `RUNLOG.jsonl`; the revision there reads `856e215-dirty` because the runs used a local build with the gateway provider and the 1,024 cap.

| Split | Templates | Matched / all (failed checks = not covered) | Matched / answered (95% CI) | Failed checks | Cost (promo) | Rows |
|---|---|---|---|---:|---:|---|
| dev (469) | 3 (legacy) | 23 / 469 = 4.9% | 23 / 466 = 4.9% (3.3–7.3) | 3 | $0.73 | `results/families-coverage-dev-legacy-litellm.jsonl` |
| dev (469) | 13 | 258 / 469 = 55.0% | 258 / 452 = 57.1% (52.5–61.6) | 17 | $1.09 | `results/families-coverage-dev-all-litellm.jsonl` |
| held-out (377) | 3 (legacy) | 16 / 377 = 4.2% | 16 / 377 = 4.2% (2.6–6.8) | 0 | $0.56 | `results/families-coverage-final-legacy-litellm.jsonl` |
| held-out (377) | 13 | 215 / 377 = 57.0% | 215 / 368 = 58.4% (53.3–63.3) | 9 | $0.82 | `results/families-coverage-final-all-litellm.jsonl` |

Held-out matches by family (13 templates): vehicle parts 56, cables and harnesses 41, hand tools 37, PCBA-based devices 36, motorised tools 15, medical devices 7, power supplies 7, display and cameras 4, furniture 3, gas grills 3, LED lights 3, water filters 2, LiDAR 1.

Disclosed runs that are not in the table: the first dev legacy run with the old 400-token cap (17 / 438 matched, 31 failed checks cut off before the JSON; `results/families-coverage-dev-legacy-maxout400-litellm.jsonl`), which led to the 1,024 cap; and a dev run with all templates stopped after a few seconds to change the cap (455 cancelled; `results/families-coverage-dev-all-maxout400-interrupted-litellm.jsonl`). A spot check of 15 dev matches found one stretch (a pet bed matched to furniture).

Command: `go run ./cmd/eval familycov -split dev|final -set legacy|all [-confirm-final]`.

### Family 14: electric motors, pumps and compressors (21 Nov; D212)

Written by the D138 method from the 10 assembled pool rulings in the `electric_motors_pumps_compressors` category (`results/families-selection.json`): a gemini-4-argon draft (`results/families14-draft.json`), every citation checked by gemini-3.8-flash against the ruling's facts and holding (18 of 30 supported; `results/families14-citation-checks.jsonl`), notes rewritten to cite only kept rulings. The template and the fixture were committed (0b956b2) before any coverage run. Adding a family made the family check fail more often at the 1,024-token cap (dev 35 failed, against 17 with 13 families), so the cap is now 2,048, chosen on dev (d7c58f3); the held-out split was then scored once.

| Split | Templates | Matched / all (failed = not covered) | On rulings both runs answered | Failed checks | Cost (promo) | Rows |
|---|---|---|---|---:|---:|---|
| dev (469) | 13, cap 1,024 | 258 = 55.0% (50.5–59.5) | 253 / 446 | 17 | $1.09 | `results/families-coverage-dev-all-litellm.jsonl` |
| dev (469) | 14, cap 1,024 | 248 = 52.9% | — | 35 | $1.25 | `results/families-coverage-dev-all-litellm-f14.jsonl` |
| dev (469) | 14, cap 2,048 | 265 = 56.5% (52.0–60.9) | 258 / 446 (6 gained, 1 lost) | 9 | $1.23 | `results/families-coverage-dev-all-litellm-f14-cap2048.jsonl` |
| held-out (377) | 13, cap 1,024 | 215 = 57.0% (52.0–61.9) | 214 / 367 | 9 | $0.82 | `results/families-coverage-final-all-litellm.jsonl` |
| held-out (377) | 14, cap 2,048 | 224 = 59.4% (54.4–64.3) | 219 / 367 (6 gained, 1 lost; exact McNemar p = 0.125) | 4 | $0.87 | `results/families-coverage-final-all-litellm-f14.jsonl` |

- **The family adds about one point of scope, like for like** (+1.1 dev, +1.4 held-out on rulings both runs answered). The rest of the held-out gain (59.4% against 57.0%) is fewer failed checks after the cap change, which helps every family. The new family matched 8 held-out rulings; 3 had matched another template before (PCBA devices, motorised tools, vehicle parts), as D138 expected from the overlap.
- **Live check, one fixture** (`backend/testdata/families/condensate_pump_2025.json`, from N347899, CBP held China): engine China 0.90; 7 moves searched, 4 shown; the smallest is making the motor stator and rotor in Thailand, supported by N344081 (a vacuum pump whose stator and rotor were made in Thailand, held Thailand) at verifier confidence 0.94. $0.24 promo (`results/families-plan-condensate-pump-2025-litellm.json`). One development check, not an evaluation of plan quality.
- Every coverage run is in `RUNLOG.jsonl`. Scope, not accuracy: a match means a template exists. One run each, through the gateway.

### Live plan checks on the new families (development, not an evaluation)

One fixture per new family (`backend/testdata/families/`, built from pool rulings), each run once with `eval plan` (engine and verifier gemini-4-argon, verifier-v2, local corpus search, plan bar 0.92). Summary: `results/families-live-checks.json`; full results `results/families-plan-*-litellm.json`.

| Family | Fixture (ruling) | CBP held (current) | Engine on current setup | Result | Promo cost |
|---|---|---|---|---|---:|
| PCBA-based devices | USB-C hub (N341280 → N342837) | China | China | PCBA (SMT) in Taiwan, supported by N339405 at 0.96; matches CBP's later ruling | $0.046 |
| Vehicle parts | Speed sensor (N343953 → N344809) | China | Thailand | Hall IC in Thailand, N343950 at 0.98; matches CBP's later ruling | $0.039 |
| Hand tools | Ratchet wrench (illustrative all-China start; N347615) | none given | China | forging in Vietnam, N335671 at 0.96 (and three larger moves); matches N347615 | $0.181 |
| Power supplies | Smartwatch charger (N347058) | China | China | PCBA and charging module in Vietnam, N333268 at 0.96 | $0.119 |
| Gas grills | Gas grill (N335085, scenario 1 → 2) | China | China | lids and fireboxes, burners and frames in Indonesia, N333420 at 0.96; CBP's scenario 2 moved only the lids and fireboxes | $0.218 |
| Medical devices | Bulb syringe (N342838) | China | Vietnam | moulded group (the bulb) in Vietnam, N341969 at 0.96 | $0.037 |
| Display and cameras | Metering camera (N348165) | China | South Korea | camera PCBA in South Korea, N344595 at 0.95 | $0.061 |
| Water filters | Electric dispenser (N348298) | China | South Korea | no plan above 0.92 (best 0.90): send to a broker | $0.135 |
| Furniture | Speaker stand (N335711) | China | Malaysia | no plan above 0.92 (0.90): send to a broker | $0.049 |
| Cables and harnesses | Server cable (N331047, scenario 1 → 3) | none given | Malaysia | no move supported (only rulings before Mar 2023 are evidence): send to a broker | $0.090 |

Total $0.97 promo / $1.95 list. The engine's view of the current setup disagreed with CBP in 5 of the 8 fixtures where CBP named the origin; it named the assembly or target country where CBP held China, the same direction as the known held-out weakness (D138, MODEL-CARD).

**Facts correction (D264, #392).** These one-run checks used variant facts rendered before the template-default fix. A moved group with no product-supplied operations inherited the template's *union* of processes across products, so the water-dispenser variants also asserted "UV lamp assemblies, lamp PCBAs, quartz tubes, and RFID smart chip assemblies", and its final-assembly line mentioned an "RO membrane". The 11 ruling-grounded families now declare `union_default_operations`: with no supplied operations, `Render` writes a neutral statement naming only this product's components and no process steps. The recorded result files are left untouched as evidence of what was actually run; they are **not** a validation of the corrected facts, and no model call was repeated.

## Planner: post-cutoff flip pairs and per-family plan checks (gateway, 21 Nov; D170, D171)

### Precedent check on 15 post-cutoff flip pairs — frozen before scoring, scored once
- Set: `eval/data/flips_post.jsonl` (11 real changes, 4 controls; frozen in 6f58d54). Production configuration: verifier-v2 on gemini-4-argon, plan bar 0.92, local corpus search, OpenAI-compatible LiteLLM gateway to Gemini. `go run ./cmd/eval flips -post [-known-a]`.
- CBP's starting origin given: shown 8, right 8/8 (67.6–100); recall 8/11 = 72.7% (43.4–90.3); controls shown 0/4. $0.58. `flips-post-verifier-v2-known-a-litellm.jsonl`.
- Not given: shown 5, right 4/5 (37.6–96.4), one wrong plan shown (fracking unit, wrong later origin, 0.92); recall 4/11 = 36.4% (15.2–64.6); controls shown 0/4. $0.59. `flips-post-verifier-v2-unknown-a-litellm.jsonl`.
- Every pair's outcome: `planner-post-summary.json`; docs/EVALUATION.md section 7.1.

### Plan search on the post-cutoff pairs with a template — once each
- Strut (target Thailand): CBP's move is the top plan, 0.96. Stove heater (Thailand): CBP's move shown at 0.92, ranked below a cheaper untested subset (motor alone). Three air-purifier reverse probes (China): no plan shown; two had moves supported at 0.85, below the bar (Champ one, Waffle Cone two; corrected in D301). $0.76. `planner-post-plan-*-litellm.json`.

### Per-family plan checks — development and train rulings, not an evaluation
- 23 searches, one each: 15 of 23 (65.2%, 44.9–81.2) show a precedent-supported plan, 8 go to a broker. Cited ruling, read by hand: supports 9, partial 4, does not 2. $1.90 ($0.083 a search). `planner-families-summary.json`; docs/EVALUATION.md section 7.2.


## Planner: post-cutoff flip-pair addendum (gateway, 21 Nov; D192)

### Frozen before scoring (commit e3ae57f, rebased as 008ec73, 16:37 +08:00), scored once per mode, production configuration
- Searches: the 85 post-cutoff substantial-transformation rulings against BM25 neighbours ranked 11-30; the 78 post-cutoff rulings of other regimes against their top 15; a re-probe of 1,284 CBP ruling numbers (N366250-N367600), which found **no rulings posted since the first probe**. Mining $0.92 (gemini-3.8-flash).
- Kept after reading: **3 pairs**, all edge tier: one real change (power transistor, front end in the United States vs South Korea) and two controls (fiber patch cord cabled in China vs Vietnam; water fountain assembled in Cambodia vs Canada, Chinese pump in both). Every candidate left out is listed with its reason in `data/flips_post_addendum_meta.json`.
- CBP's starting origin given: the real change shown at 0.96 on N337307 with South Korea; both controls not supported. $0.10. `flips-post-verifier-v2-addendum-known-a-litellm.jsonl`.
- Not given: the real change shown at 0.96; the patch cord control not supported; the fountain control **failed** (gateway timeout after 4 attempts) and is counted as not shown, not re-run. $0.07. `flips-post-verifier-v2-addendum-unknown-a-litellm.jsonl`.
- Combined with D170 (18 pairs: 12 real changes, 6 controls; D170 rows not re-run), `results/planner-post-addendum-summary.json`:
  - known-a: shown 9, right 9/9 (70.1-100); recall 9/12 = 75.0% (46.8-91.1); controls shown 0/6 (0-39.0).
  - unknown-a: shown 6, right 5/6 (43.7-97.0); recall 5/12 = 41.7% (19.3-68.1); controls shown 0/6 (0-39.0), one control row an error.
- Core tier unchanged (no core pair was added).

## Planner: the 18 post-cutoff flip pairs, runs 2 and 3 (gateway, 22 Nov; D353, D354)

### Pre-registered in D353 (commit ea48b5a4) before any run; same configuration as run 1
- verifier-v2 on gemini-4-argon, one vote, no panel, local corpus search, plan bar 0.92, fixed seed, OpenAI-compatible LiteLLM gateway to Gemini, `LITELLM_NO_CACHE=1`. `go run ./cmd/eval flips -post [-known-a] -tag run<N>` and the addendum with `-tag addendum-run<N>`.
- Starting origin given (known-a), combined 18 pairs:
  - run 2: shown 9, right 9/9 (70.1-100); recall 9/12 = 75.0% (46.8-91.1); controls 0/6. $0.66.
  - run 3: shown 9, right 9/9; recall 9/12; controls 0/6. $0.69.
  - Same 9 moves in runs 1, 2 and 3; confidences within 0.02; 8 of 9 cite the same ruling each run.
- Not given (unknown-a):
  - run 2: shown 6, right 5/6; recall 5/12; controls 0/6; one control row failed (gateway timeout), counted as not shown. $0.63.
  - run 3: shown 5, right 5/5; recall 5/12; controls 0/6. $0.69.
  - The fracking unit (wrong later origin) was shown at 0.92 in runs 1 and 2 and at 0.88, below the bar, in run 3.
- Total $2.66. Rows `results/flips-post-verifier-v2-{run2,run3,addendum-run2,addendum-run3}-{known-a,unknown-a}-litellm.jsonl`; summary `results/planner-post-repeats-summary.json`. Every run is in RUNLOG.

## Planner: the 0.92 plan bar re-examined on committed rows (no model calls, 21 Nov; D300, D301)

### Rule written first (D300, commit e4a555e6), bar chosen on development rows only
- Data: the nine verifier-v2 dev flip runs D86 pools (54 pairs, 485 verdicts). Shown = supported at confidence ≥ bar; right = real change with CBP's later origin named (D170 rule). Chosen bar = the lowest of 0.92, 0.90, 0.88, 0.85, 0.80, 0.75 that, with every higher one, has ≥ 100 shown verdicts at a precision lower bound ≥ 97% and no wrong pair shown. A production change also needs ≥ 10 wrong supported dev verdicts.
- Dev: 0.92 shows 159, all right (97.6–100); 0.90 shows 193 with 2 wrong (2 pairs); 0.85 shows 256 with 3 wrong; real-change pairs shown in every run 13/46 at 0.92, 19/46 at 0.85. **Chosen bar 0.92.** Wrong supported dev verdicts: 3 of 287, so the gate fails. Production unchanged.
- Reported after the choice, from existing rows (0.92 figures were already published): post-cutoff 18 pairs known-a, 0.92 = 0.85: 9/9 right, recall 9/12, controls 0/6. Unknown-a: 0.92 5/6, recall 5/12; 0.85 8/9, recall 8/12, controls 0/6. 2026 pairs (contaminated): 0.92 21/23 right; 0.85 36/40 with two controls shown. Plan searches: 0.85 would add 3 moves on the post-cutoff air-purifier probes (one directly contradicted by CBP) and 10 unlabelled moves on the per-family checks (21 of 23 products with a plan instead of 15).
- `eval/results/plan-bar-analysis.json`, `python3 -I -B eval/planner/plan_bar.py . --check`. No RUNLOG line: nothing ran against a model.

## Repeatability: engine-v1 on Argon, development set, three runs (gateway, 21 Nov; D190)
- Run 1: `results/dev-gemini-4-argon-prec-engine-v1-litellm-gw.jsonl` (D152). Runs 2 and 3: `results/dev-gemini-4-argon-prec-engine-v1-litellm-router-rep2.jsonl`, `...-rep3.jsonl`, with `LITELLM_NO_CACHE=1` (fresh calls, every row has a gateway cost). Five gateway timeouts (1 + 4) re-run by resume; RUNLOG has a line for each run and each resume.
- Adjusted (463): 425 / 422 / 422 = 91.8% / 91.1% / 91.1%; mean 91.3%, spread 0.7 points. Answer flips 4; fast-path membership flips 8; calibrated value changes 50 (mean change 0.005).
- Unadjusted (469): 425 / 422 / 422 = 90.6% / 90.0% / 90.0%.
- Gateway cost $4.66 and $4.61 for runs 2 and 3 (engine and router calls).
- Summary: `results/repeatability-dev-litellm.json`.

## Repeatability: engine-v1 on Argon, held-out splits, two repeats each (gateway, 21 Nov; D240, D241)
- Pre-registered in D240 (commit de0029f, the `git_revision` in RUNLOG for all six lines; kept as branch `improve/heldout-repeat-prereg`, since the PR branch was rebased onto main afterwards) before any repeat was run. A measurement of the frozen engine, not tuning; nothing was changed because of it. `LITELLM_NO_CACHE=1` on every run (fresh calls; every row has a gateway cost).
- First held-out split (377), no `-router`, fast path on the fixed Vertex scope file: `results/final-gemini-4-argon-prec-engine-v1-litellm-repeat1.jsonl`, `...-repeat2.jsonl`. 339 and 340 right = 89.9% and 90.2%. Official (Vertex, 15 Oct) 340 = 90.2%; 15 Oct proxy 339. Answer flips over official and repeats 7; fast-path membership flips 28 (6 between the repeats). One timeout in repeat 1 and three in repeat 2, re-run by resume (their answers count). RUNLOG's fast-path fields for these two runs have no scope check (no router in the run); the docs use the scope file.
- Second held-out split (83), with `-router`: `results/final2-gemini-4-argon-prec-engine-v1-litellm-router-repeat1.jsonl`, `...-repeat2.jsonl`. 77 and 79 right = 92.8% and 95.2%. Official (D154) 77 = 92.8%. Answer flips 2 (N365606, N365997, right only in repeat 2). No errors.
- Gateway cost: $2.96 and $2.93 (377, engine only, with resumes); $0.82 and $0.76 (83, engine and router). $7.47 in all.
- Summary: `results/repeatability-heldout-litellm.json`.

## Plan check run again: the heat-gun demo, three runs (gateway, 21 Nov; D321, D322)
- Pre-registered in D321 (commit 0abafde, the `git_revision` of all three runs and of their RUNLOG lines; kept as branch `improve/panel5-plan-stability-prereg`, since the PR branch was rebased onto main afterwards) before any run. A later change on main (#510) added backtest fields to `backend/testdata/heatgun_2022.json`; it does not touch the moves or the checks. Production's verifier configuration (verifier-v2, panel, near-miss and follow advice, one vote, bar 0.92) and searcher (relaxed CROSS search filled from the corpus), `LITELLM_NO_CACHE=1`. A demonstration, not a test.
- Rows: `results/plan-heatgun-2022-relaxed-verifier-v2-stability-run{1,2,3}-litellm.json`. Summary with every earlier recorded run: `results/plan-stability-heatgun.json` (`python3 -I -B eval/planner/plan_stability.py . --check`).

| Move to Thailand | Run 1 | Run 2 | Run 3 | Shown |
|---|---|---|---|---|
| Motor-fan + PCBA + switch, cord and minor parts | 0.95 (N347421) | 0.96 (N345859) | 0.96 (N345859) | 3 of 3 |
| Motor-fan + PCBA | 0.92 | 0.92 | 0.94 | 3 of 3 |
| Motor-fan + switch, cord and minor parts | 0.92 | 0.92 | 0.92 | 3 of 3 |
| Motor-fan alone | 0.90 | 0.91 | 0.91 | 0 |
| PCBA + switch, cord and minor parts | 0.85 | 0.78 | 0.85 | 0 |
| Switch, cord and minor parts (unsupported) | 0.85 | 0.90 | 0.85 | 0 |
| PCBA alone (unsupported) | 0.75 | 0.75 | 0.82 | 0 |

- Gateway cost $0.46, $0.45, $0.41 ($1.32); wall time 91 s, 105 s, 138 s. No run failed or was retried.

## Plan after the split: the heat-gun demo, two runs (gateway, 22 Nov; D330, D331, D332)
- The motorised-tools template now keeps the power cord, knobs and covers apart from the switch and wiring (D330), so CBP's own 2026 move (N357771: motor-fan assembly, PCBA, and power cord, knobs and covers to Thailand; switch, wiring and fasteners kept in China) can be searched. Made after seeing N357771: a demonstration, not a test.
- Pre-registered in D331 (commit 071043a, the `git_revision` of both runs and their RUNLOG lines; kept as branch `improve/split-switch-prereg`). D321's configuration, `LITELLM_NO_CACHE=1`, `-tag split-run{1,2}`. Run 2's revision is marked dirty: the summary script and eval summary code were being edited while it ran; the planner, template, fixture and verifier were as committed.
- Rows: `results/plan-heatgun-2022-relaxed-verifier-v2-split-run{1,2}-litellm.json`. Summary: `results/plan-stability-heatgun.json` (`d331`).

| Move to Thailand (net a year, illustrative) | Run 1 | Run 2 | Shown |
|---|---|---|---|
| Motor-fan alone ($380k) | 0.92 | 0.90 | 1 of 2 |
| Motor-fan + power cord, knobs and covers ($365k) | 0.92 | 0.92 | 2 of 2 |
| Motor-fan + switch, wiring and minor parts ($360k) | 0.92 | 0.92 | 2 of 2 |
| Motor-fan + PCBA ($320k) | 0.93 | 0.92 | 2 of 2 |
| **Motor-fan + PCBA + power cord, knobs and covers: CBP's move ($305k)** | **0.94 (N347421)** | **0.94 (N347421)** | 2 of 2 |
| Motor-fan + PCBA + switch, wiring and minor parts ($300k) | 0.94 | 0.93 | 2 of 2 |
| All four groups, followed from near-miss advice ($285k) | 0.96 | 0.96 | 2 of 2 |
| Power cord, knobs and covers alone (unsupported) | 0.92 | 0.92 | 0 |
| PCBA + switch, wiring and minor parts | 0.85 | 0.78 | 0 |
| PCBA + power cord, knobs and covers | 0.75 | 0.75 (unsupported) | 0 |
| Switch, wiring and minor parts alone (unsupported) | 0.90 | 0.85 | 0 |
| Switch group + cord group (unsupported) | 0.85 | 0.85 | 0 |
| PCBA alone (unsupported) | 0.75 | 0.75 | 0 |

- CBP's move ranked 5 of 7 (run 1) and 4 of 6 (run 2): above the bigger moves, below the cheaper ones. The backtest reads the top-ranked plan: run 1 motor-fan alone (2 of 6 changed parts moved), run 2 motor-fan + power cord, knobs and covers (5 of 6, nothing CBP kept). Before the split (D322) the top-ranked plan moved 5 of 6 plus the switch, wires and fasteners, and the nearest move to CBP's checked 0.95–0.96.
- Gateway cost $0.96, $0.82 ($1.78); wall time 154 s, 206 s; 13 moves each (12 searched, 1 followed). No run failed or was retried.

## Plan ranking by tier, re-ranked on committed rows (no model calls, 22 Nov; D340, D341)

### Rule written first (D340, commit 61fbd0f7), margin chosen on development rows only
- Data: the eight known-a verifier-v2 dev flip runs (54 pairs). For each pair and ordered pair of runs (a, b), a tier-1 verdict in a (supported, check ≥ 0.92 + m, no counter-ruling) holds when b shows the pair at 0.92. Chosen m = the smallest of 0.01–0.05 that, with every larger one, holds ≥ 95% over ≥ 50 run pairs.
- Dev: every shown verdict held 906/994 (91.1%); tier 1 held 644/644 at m = 0.01 and 0.02, 433/433 at 0.03, 363/363 at 0.04, 63/63 at 0.05. **Chosen m = 0.01** (tier 1 from 0.93). At-the-bar verdicts held 262/350 (74.9%).
- Re-ranked after the choice (offline): 44 committed plan runs, 30 with a shown plan; the top plan changes in 6 (heat gun D322 runs 1 and 3, D332 runs 1 and 2, per-family N350826, post-cutoff stove heater). The new top backtests worse in 2 (D332 run 1, the stove heater). CBP's heat-gun move after the split ranks 6th of 7 (run 1, counter-ruling N345859) and 1st of 6 (run 2).
- `eval/results/plan-rank-analysis.json`, `python3 -I -B eval/planner/plan_rank.py . --check`. No RUNLOG line: nothing ran against a model.

## 19 CFR 102.20 rule check on the Part 102 rulings (offline scoring, gateway extraction, 22 Nov; D360, D361, D362)
- The deterministic rule layer (`backend/internal/tariffshift`, chapters 73, 82, 84, 85, 87, 90, 94) applied to the Part 102 splits. A model (Gemini 3.8 Flash through the gateway, prompt `tariffshift-extract-v2`) copies the HTS codes, origins and country of production out of each text, with a verbatim quote that code checks; it classifies nothing. Pre-registered in D361 (commit 65b1cb2f); code frozen at 89552154 before the held-out extraction; each held-out file extracted and scored once.
- "facts" is the text the engine reads. "full ruling text" includes CBP's own classifications and analysis: it checks the encoding given CBP's codes, not the product.

| Split | Input | n | Out of scope | Decided | Agrees with CBP | Differs from the model |
|---|---|---:|---:|---:|---:|---:|
| dev | facts (prompt v1) | 72 | 5 | 0 | – | – |
| dev | full ruling text | 72 | 39 | 2 | 2 of 2 | 0 |
| final (held out) | facts | 32 | 6 | 0 | – | – |
| final (held out) | full ruling text | 32 | 17 | 2 | 2 of 2 | 0 |
| final2 (held out) | facts | 13 | 3 | 0 | – | – |

- Decided rulings: N344091 (8507.90, Mexico) and N344002 (8414.30, Mexico) on dev; N360891 (8708.29, Canada) and N362506 (8507.10, Mexico) held out. One more dev ruling, H339179 (unfinished valves, 8409.91), is open only on the de minimis value; its answer if de minimis does not apply, China, is CBP's.
- Development v1 runs (0 decided on facts; 0 decided, 1 open only on de minimis on full text) are in `tariffshift/dev-v1/`. Nine dev v2 rows ran out of output tokens and were re-run once at 8,192; 2 dev and 2 final full-text rows still failed. final2 full text was not run (raw texts not available locally).
- Rows: `tariffshift/inputs-*.jsonl` (extractions with quotes), `tariffshift/results-*.jsonl`, `tariffshift/summary.json`. `cd backend && go run ./cmd/tariffshift eval -check` recomputes them (in `make reproduce`). Cost $2.58 gateway-reported; RUNLOG `tariffshift-extract-*`.

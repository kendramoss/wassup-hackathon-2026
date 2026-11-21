# On-screen numbers and their sources

Every number that may appear in the demo, deck or results page. **Status** says whether it can be shown today. The held-out engine, fast path, cost and latency rows below are the Vertex AI re-run (D39). "Proxy" = produced through the LiteLLM proxy: development only, and not a substitute for those rows.

## Evaluation (results page — served by `GET /v1/eval/summary`)

| Number | Value | Source | Status |
|---|---|---|---|
| Held-out origin accuracy (377 rulings, Jan–Sep 2026) | **90.2% (86.8–92.8)**, Vertex AI, frozen engine-v1. Proxy re-run 89.9% (86.5–92.6); research run 89.1% (85.6–91.9) | `eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl` (D39). Proxy file kept for the comparison | Official. Proxy and research figures stay labelled as such |
| Best simple baseline (most-mentioned country) | 60.5% (55.5–65.3) | research `RESULTS-v2.md` (no model involved); not committed, so not reproducible here | Reported, unverified. Show only with that label. Since D155 the screens use the recomputed figures instead (most-mentioned 60.7%, best simple rule 63.4%; row "Rule baselines, recomputed"). Never show a lift between this figure and the 96.0% fast-path row: the fast path is a 300-case subset, this baseline is all 377 |
| Other baselines | final-assembly 40.6%, last-mentioned 39.8%, majority 12.2% (majority since recomputed exactly, D155) | research `RESULTS-v2.md`; not committed, so not reproducible here | Reported, source not in this repository, unverified. Show only with that label, like the 60.5% row |
| Fast path (calibrated confidence ≥ 0.90, the same cases as stated confidence ≥ 0.92, and in scope; D160), held-out | 79.6% of cases (300/377) at 96.0% (93.1–97.7), Vertex AI, router-v2. Proxy was 78.5% at 96.6% (93.9–98.2). At the old 0.95 bar, proxy coverage was 70.3% at 97.7% | final Vertex rows + `eval/results/scope-final-router-v2-vertex.jsonl` | Official. The proxy figure stays labelled proxy. 288 of 300; the subset is selected by the model's own confidence and router scope |
| Confidence ≥ 0.95 alone, held-out | 71.9% at 97.8% (95.3–99.0) | final rows | Proxy. Do **not** show the research "65% at 98.0%": it describes a different routed pipeline |
| Change detection from scratch, held-out 2026 pairs | 68% (47–84), n=22; always-change baseline 63.6%; balanced accuracy 61.6% | research `RESULTS-v2.md` | Showable, always with both baselines; label "weak" |
| Move verifier (verifier-v2), development set (54 pre-2026 pairs) | precision 32/32 (89–100), recall 69.6% (55.2–80.9, n=46), specificity 8/8; with CBP's current origin given: 97.3% of 37 (86.2–99.5), 78.3%, 7/8 | `eval/results/flips-dev-verifier-v2-{unknown,known}-a-litellm.jsonl` (served by `/v1/eval/summary`). The earlier 34/34 and 74% were verifier-v1 | Proxy; must say **"development set"** |
| Development engine accuracy (Jul–Dec 2025) | Unadjusted, 469 rulings: 422/469 = 90.0% (86.9–92.4); fast path 371/469 = 79.1% at 355/371 = 95.7% (93.1–97.3). After scoring adjustments (D23), 463 rulings: 422/463 = 91.1% (88.2–93.4); fast path 368/463 = 79.5% at 355/368 = 96.5% (94.1–97.9). Always say which | `eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl` + `eval/results/scope-dev-router-v2-vertex.jsonl` + `eval/data/scoring_adjustments.json`; adjusted figures recomputed in `eval/results/evaluation-metrics.json` | Showable; must say **"development set"**. `GET /v1/eval/summary` embeds the unadjusted figures; `eval/RESULTS.md`, `docs/claims-reconciliation.md` and docs/EVALUATION.md use the adjusted ones |
| Calibration, held-out (377) | mean stated confidence 94.7% vs accuracy 90.2%; ECE 0.059 (0.041–0.091) with one bin per emitted confidence value, 0.046 (0.022–0.074) with 10 equal-width bins; Brier 0.081 (0.059–0.105) against 0.089 for a constant forecast. Bootstrap 95% intervals, 2,000 resamples, seed 20261017 | `eval/results/evaluation-metrics.json`, built by `python3 -I eval/charts/build_evaluation.py .` from the final Vertex rows | Measured offline, no model calls. Say "overconfident by about 4.5 points on average"; never call the stated confidence a calibrated probability (the calibrated one is the D160 row below) |
| Calibration, development set (463, adjusted) | mean confidence 94.9% vs accuracy 91.1%; ECE 0.050 (0.033–0.078) per emitted value, 0.038 (0.016–0.063) 10-bin; Brier 0.072 (0.051–0.094) against 0.081 constant | same file | Measured offline; must say **"development set"** |
| Split exclusions | rulings where CBP held two or more countries: 29 of 498 in the dev window, 7 of 396 in 2026 (then 12 pilot cases removed: 389 → 377) | `eval/results/evaluation-metrics.json` (`funnel`), reconstructed from `eval/data/labels.jsonl` | Measured; the original cleaning script is not committed, so this is a reconstruction that reproduces the counts exactly |
| Textile rules, held-out (71 rulings, 2026, textile-v1) | 85.9% (76.0–92.2); 95.2% (84.2–98.7) when auto-accepted at confidence ≥ 0.95, on 59.2% of cases. Production prompt textile-v2 scored 86.9% (78.8–92.2) on 99 development rulings | `eval/results/textile-final-gemini-4-argon-prec-textile-v1-vertex.jsonl`; dev: `eval/results/textile-dev-…-vertex.jsonl` | Official (owner, 18 Oct); the v2 figure says "development set" |
| Pharmaceutical rules, held-out (56 rulings, 2026, pharma-v1) | 89.3% (78.5–95.0); 100% (92.7–100) at confidence ≥ 0.95, on 87.5% of cases | `eval/results/pharma-final-gemini-4-argon-prec-pharma-v1-litellm.jsonl` | Official (owner, 18 Oct). LiteLLM proxy run, not re-run on Vertex |
| Part 102 marking-origin rules (the `usmca_part102` engine; not USMCA qualification), held-out (32 rulings, 2026, usmca-v1) | 78.1% (61.2–89.0); 100% (79.6–100) at confidence ≥ 0.95, on 46.9% of cases | `eval/results/usmca-final-gemini-4-argon-prec-usmca-v1-litellm.jsonl` | Official (owner, 18 Oct). LiteLLM proxy run; small n, always with the interval |
| Router, development window (Jul–Dec 2025, router-v2) | pharmaceutical and USMCA kept out of the substantial-transformation engine 88.4% (75.5–94.9, n=43); textile sent to the textile engine 87.9% (72.7–95.2, n=33); ordinary rulings wrongly sent out of scope 5% (1.4–16.5, n=40) | `eval/results/scope-window-dev-router-v2-vertex.jsonl` | Showable; must say **"development set"** |
| Second held-out split (83 rulings, 28 Sep–13 Nov 2026; D150, D154) | 77/83 = 92.8% (85.1–96.6); fast path 75/83 = 90.4% at 97.3% (90.8–99.3); $0.0098 per case (gateway-reported); p50 7.5 s / p95 12.3 s | `eval/results/final2-gemini-4-argon-prec-engine-v1-litellm-router.jsonl`; `eval/results/accuracy-v2-metrics.json` (`heldout.final2`) | Measured, scored once, engine-v1 on Argon through the gateway (not Vertex AI). Always with n and the interval; say it confirms the engine after its cutoff, not that it improved |
| Both held-out splits pooled (460) | 417/460 = 90.7% (87.6–93.0); fast path 375/460 = 81.5% at 96.3% (93.8–97.8) | `eval/results/accuracy-v2-metrics.json` (`heldout.pooled_final_final2`) | Measured; mixes the Vertex run (377) and the gateway run (83). Say so wherever it is shown |
| All regimes, first held-out window, pooled | 476/536 = 88.8% (85.9–91.2): substantial transformation 377, textile 71, pharmaceutical 56, Part 102 32 | `eval/results/accuracy-v2-metrics.json` (`heldout_all_regimes`) | Measured; pharmaceutical and Part 102 are proxy runs |
| Model sweep, development set (463 adjusted; D153) | Argon 91.1% on Vertex, 91.8% through the gateway (p = 0.58); Carbon 88.1% (27 failed calls); 3.1 Pro preview 87.3% (10 failed calls); majority of three 91.6% | `eval/results/accuracy-v2-metrics.json` (`dev`) | Measured; must say **"development set"**; gateway runs |
| Calibration layer (isotonic, fitted on dev; D156) | held-out 377: ECE 0.059 → 0.030 (0.020–0.059), Brier 0.081 → 0.072 (0.056–0.092), mean stated 94.7% → 90.4% (accuracy 90.2%); second split: ECE 0.067 → 0.038, Brier 0.060 → 0.051 | `eval/results/accuracy-v2-metrics.json` (`calibration_layer_engine_v1`, `heldout.final2.calibration_layer`) | Measured offline. cal-v1, eval only; the product uses cal-v2 (next row) |
| Calibrated confidence in the product (cal-v2, fitted on dev; D160) | held-out 377: mean stated 94.7% → calibrated 90.2% (accuracy 90.2%), ECE 0.059 → 0.028 (0.020–0.060), Brier 0.081 → 0.072 (0.056–0.092); final2 (83): 96.1% → 93.8% (accuracy 92.8%), ECE 0.067 → 0.037 (0.030–0.099), Brier 0.060 → 0.051; both (460): ECE 0.055 → 0.026 (0.018–0.053) | `eval/results/accuracy-v2-metrics.json` (`calibration_v2`); map `backend/internal/calibrate/map.json`; 377 figures also recomputed in Go by `go run ./cmd/claimscheck` (`final.cal.*`) and served by `GET /v1/eval/summary` (`final_calibration_*`) | Built: the API returns `calibrated_confidence` for engine-v1 on Gemini 4 Argon; the Result screens and MCP tools show it. Other engines and the fallback model: stated confidence only |
| Calibrated auto-accept bar (D160) | 0.90 on calibrated confidence: the lowest map step with in-scope dev accuracy ≥ 96% on ≥ 100 cases (dev 368/463 = 79.5% at 96.5%). It accepts exactly the cases stated ≥ 0.92 did, so the held-out fast-path figures are unchanged (377: 300 at 96.0%; final2: 75 at 97.3%) | `eval/results/accuracy-v2-metrics.json` (`calibration_v2.dev_curve`, `.final.at_bar`, `.final2.at_bar`); `config.DefaultCalibratedFastPath` | Built; `CALIBRATED_FAST_PATH` default, `CALIBRATION=off` restores the stated 0.92 bar |
| Rule baselines, recomputed (no model; D155) | first held-out split: first country mentioned 63.4% (58.4–68.1), most-mentioned 60.7% (55.7–65.5), last 23.6%, majority (Vietnam) 12.2%; second split: most-mentioned 71.1% (60.6–79.7), first 60.2% | `eval/results/baselines-final.json`, `eval/results/baselines-final2.json` (`go run ./cmd/eval baselines`) | Measured. The best simple rule on the 377 is the first country mentioned, 63.4% |
| Cost per origin prediction | $0.0078 (promo price) / $0.0157 (list), Vertex AI held-out | final Vertex rows × Gemini Developer API prices verified 2026-10-14 | Token counts are Vertex. The Vertex price sheet was not re-checked; this is the same promo/list schedule as the proxy runs |
| Cost per verified move | $0.031–0.032 (promo) | verifier dev rows | Proxy |
| Full plan, heat gun | $0.22 promo / $0.45 list, 33 s, 7 engine calls + 6 verifier runs | `eval/results/plan-heatgun-2022-local-litellm.json` | Proxy; demo fixture |
| Latency per prediction | p50 4.6 s / p95 5.7 s | final Vertex rows | Official Vertex AI. The proxy run was p50 5.0 s / p95 6.4 s |

## API limits (developers page, `docs/api/SECURITY.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Requests a minute per client, routes without a model call | 120 | `RATE_BASIC_PER_MIN` default, `backend/internal/config/config.go` (D137) | Built; default, configurable |
| Light / heavy model requests a minute per client | 20 / 4 | `RATE_LIGHT_PER_MIN`, `RATE_HEAVY_PER_MIN` defaults, same file (D45) | Built; default, configurable |
| Model-calling requests running at once per client | 4 | `RATE_MAX_IN_FLIGHT` default (D137) | Built; per instance |
| Invalid API keys a minute per address | 10 | `RATE_AUTH_FAILURES_PER_MIN` default (D137) | Built; per instance |
| One client's share of the daily model budget | a tenth (0.1) | `DAILY_SPEND_CLIENT_SHARE` default (D137) | Built; applies only with `DAILY_SPEND_CAP_USD` set |

## Demo story

| Number / claim | Value | Source | Status |
|---|---|---|---|
| Heat gun 2022 origin | CBP held China | CBP ruling N328954 (15 Nov 2022) | Showable |
| Heat gun 2026 origin after the change | CBP held Thailand | CBP ruling N357771 (29 Jan 2026) | Showable |
| Duty difference China vs Thailand, heat gun | ≈25 points (≈41.7% vs ≈16.7% ad valorem) | `docs/DUTY-SOURCES.md` (HTS 8419.89.95 inferred; Section 301 List 2) | Showable with "product-specific; classification to be confirmed by a broker" |
| Plan shown | Make the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand: check 0.96 (at or above 0.92), supporting N345938 (hair clipper kit, 6 Mar 2025), no counter-ruling, engine Thailand 1.00, net $285k/yr (illustrative economics). 1 of 7 moves cleared the bar; the closest single move, motor-fan assembly alone, is 0.90 on N325810. Backtest against N357771: the plan moves the motor, fan, PCBA, knob, power cord and back cover; only the outer shells from CBP's change are not in it (the housing moulding is already Thai) | Production run, backend-00051-kmk, case_878d76540b91, cached for everyone | One production run, after three-group moves (D84) and the relaxed precedent search (D89). Earlier run (backend-00038-zw2): no plan cleared 0.92, closest move motor-fan assembly 0.90 |
| Duty effect / added cost on plan cards | $500k/yr and $180k/yr | fixture inputs (illustrative — editable) | Show only as "illustrative — editable" |

## Known failure cases and corpus counts (Accuracy page, MODEL-CARD.md, DATASHEET.md)

Built by `python3 -I eval/charts/build_trust_data.py .` from committed files; `--check` fails if the outputs are stale. No model calls.

| Number | Value | Source | Status |
|---|---|---|---|
| Held-out misses listed | all 37 of 377; 12 auto-accepted (confidence ≥ 0.92 and in scope), 25 sent to review | `frontend/src/lib/failure-cases.json`, from the final Vertex rows + `eval/results/scope-final-router-v2-vertex.jsonl` | Measured. Every miss is listed, none selected |
| Accuracy by origin CBP held, held-out | Thailand 60/66 = 90.9% (81.6–95.8); China 44/57 = 77.2% (64.8–86.2); United States 51/53 = 96.2% (87.3–99.0); Vietnam 43/46 = 93.5% (82.5–97.8); Malaysia 34/35 = 97.1% (85.5–99.5); South Korea 20/22 = 90.9% (72.2–97.5); Taiwan 18/19 = 94.7% (75.4–99.1); 26 other countries 70/79 = 88.6% (79.8–93.9). Wilson 95% | same file, `by_origin` | Measured. Small n per origin: always with the interval |
| Direction of the misses | CBP held China, Unde said another: 13 (7 auto-accepted). Unde said China, CBP held another: 18 (4 auto-accepted). Neither China: 6 | same file, `china` | Measured |
| Held-out accuracy by collection | NY 328/364; HQ 12/13 | same file, `by_collection` | Measured. HQ n = 13 is too small to compare |
| Corpus | 2,790 rulings, 2023-01-10 to 2026-09-25; NY 2,619 / HQ 171; by year 354 / 425 / 1,358 / 653; regex 2,696 / manual 94; 3,081 facts files | `eval/results/corpus-stats.json` | Measured |
| Substantial-transformation pool | 1,703 rulings, 1,593 with a single held origin, 93 countries; China + ASEAN 62.8% of those 1,593 | same file | Measured |

## Planner product scope (D138)

| Number | Value | Source | Status |
|---|---|---|---|
| Product families with a plan template | 13 (3 before D138, 10 added) | `backend/internal/planner/templates/*.json`; pinned by `TestTemplateSetIsPinned` | Built |
| Selection pool | 845 substantial-transformation rulings outside the dev, held-out and pilot splits; 840 labelled | `eval/results/families-selection.json` | Measured (model labels, gemini-3.8-flash) |
| Assembled pool rulings per new family | vehicle parts 88, PCBA-based devices 86, hand tools 67, cables and harnesses 59, medical devices 26, power supplies 23, furniture 15, display and cameras 14, water filters 12, gas grills 11 | `eval/results/families-selection.json` (`chosen`) | Measured (model labels) |
| Rulings cited by the new templates | 156 distinct; 267 of 326 citation checks supported, the rest dropped | `backend/internal/planner/templates/*.json` (`sources`), `eval/results/families-citation-checks.jsonl` | Measured; each citation checked by gemini-3.8-flash against the ruling's facts and holding |
| Coverage, held-out (377, 2026) | 16 (4.2%) before, 215 (57.0%) after; failed checks counted as not covered. On the 368 checks that answered: 58.4% (53.3–63.3) | `eval/results/families-coverage-final-{legacy,all}-litellm.jsonl` | Measured once per set (LiteLLM gateway, not Vertex). Say "matched to a family with a plan template", never "planned" or "accurate" |
| Coverage, development (469, Jul–Dec 2025) | 23 (4.9%) before, 258 (55.0%) after; on the 452 that answered: 57.1% (52.5–61.6) | `eval/results/families-coverage-dev-{legacy,all}-litellm.jsonl` | Measured; say "development set" |
| Family-check failures | 17 of 469 dev, 9 of 377 held-out at 1,024 output tokens; 31 of 469 dev at the old 400 | coverage rows above | Measured |
| Live plan checks, new families | 7 of 10 fixtures show a precedent-supported plan, 3 go to a broker; $0.97 promo for the ten | `eval/results/families-live-checks.json` | Development checks on pool-ruling fixtures, one run each; not an evaluation |
| Gas-grill demo, development check | engine China 0.95 (CBP held China, N335085 scenario 1); 7 moves searched, 1 shown: lid and firebox forming, burners and gas train, and cooking surface, panels and frame in Indonesia, check 0.96, supporting N333420; backtest: moves both parts CBP's scenario 2 moved, plus more. Duty rates, import value and costs are illustrative placeholders | `eval/results/families-plan-gas-grill-2023-litellm.json`; demo `backend/internal/demos/fixtures/gas_grill_2023.json` | One run through the LiteLLM gateway, not production. The demo runs live when loaded, so the site's answer may differ; quote the screen, not this row |

## Planner evaluation (D170, D171)

| Number | Value | Source | Status |
|---|---|---|---|
| Post-cutoff flip pairs | 15 frozen pairs from CBP rulings, at least one dated after 25 Sep 2026: 11 real origin changes (6 core, 5 edge), 4 controls; 6 with a plan template | `eval/data/flips_post.jsonl`, `eval/data/flips_post_meta.json`, `eval/results/planner-post-familycheck-litellm.jsonl` | Built; each pair read by hand by one reader (not a customs expert) |
| Precedent check on them, CBP's starting origin given | shown at the 0.92 bar: 8 of 8 right (67.6–100); real changes shown 8 of 11 = 72.7% (43.4–90.3); core 5 of 6; controls shown 0 of 4; $0.58 for 15 checks (promo) | `eval/results/flips-post-verifier-v2-known-a-litellm.jsonl`, `eval/results/planner-post-summary.json`; recomputed by `claimscheck` (`planner_post.*`) and served as `planner_post_known-a_*` | Measured, scored once, gateway (not Vertex AI), local corpus search. Always with n and the interval; say "precedent check", not "plan accuracy" |
| Precedent check on them, starting origin not given | shown 5, right 4 (37.6–96.4): the fracking unit was shown at 0.92 with the wrong later origin; real changes shown 4 of 11 = 36.4% (15.2–64.6); controls shown 0 of 4 | `eval/results/flips-post-verifier-v2-unknown-a-litellm.jsonl`, `eval/results/planner-post-summary.json` | Measured, scored once, gateway. Show beside the known-a row, never alone |
| Cited ruling supports the shown move (hand read) | 5 of 8 support, 3 partial (known-a) | `eval/planner/spot_checks.json` | One reader |
| Plan search on the post-cutoff pairs | strut: CBP's move is the top plan (0.96); stove heater: CBP's move shown (0.92) but ranked below a cheaper subset CBP never ruled on; three air-purifier reverse probes: no plan shown (one supported at 0.85, below the bar); $0.76 | `eval/results/planner-post-plan-*-litellm.json` | Measured, once each, gateway; n = 5 |
| Per-family plan checks | 23 products (two per family where eligible; gas grills one, LiDAR none): 15 of 23 = 65.2% (44.9–81.2) show a precedent-supported plan, 8 go to a broker; China-held 10 of 15, reverse probes 5 of 8; cited ruling supports the move in 9 of 15, partial 4, does not 2; $1.90 promo, $0.083 a search | `eval/results/planner-families-summary.json`, `eval/results/planner-families-*-litellm.json` | Development checks on development and train rulings, one run each, illustrative economics; not an evaluation of plan quality |

## Claude connector (MCP; built, see `docs/MCP.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| MCP tools | 11 | `docs/MCP.md` (Tools table), `backend/internal/mcpserver` | Built; a count of tools, not a result |

## Image and scan import (built; see `docs/MULTIMODAL.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Upload limits | 1–4 files, 8 MB together, 10 PDF pages together, 12,000 px a side, 40 megapixels | `backend/internal/media/media.go` constants | Showable; these are settings, not results |
| Browser downscale | photos over 3 MB or 4,096 px re-encoded at 3,072 px longest side | `frontend/src/lib/media-upload.ts` | Showable; a setting |
| Field accuracy reading BOM images (synthetic) | 20 synthetic images, 170 parts: names read 151 (88.8%), countries right 151 (88.8%); every matched name had the right country; 15 of 20 images fully read. Set 1 (5 plain tables) 45/48; set 2 (15 varied layouts, photos, scans) 106/122. The 19 misses are exact-name mismatches: part numbers appended (13), a multi-part row split (3, one cut short in the image), one row split into two (1), plant rows counted as parts (2). Gateway cost $0.252 | `eval/results/media-v1-gemini-4-argon-litellm.json`, `eval/results/media-varied-gemini-4-argon-litellm.json`, `eval/results/RUNLOG.jsonl` (D167, D168) | Measured once, gateway (not Vertex AI), Gemini 4 Argon. Always say "synthetic images, illustrative" and give the counts. Not a measure of accuracy on real photos or scans; do not show it as one |

## Market and time (from `hackathon-research/evidence/fees/LEDGER.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Preparation per ruling request | 20 hours | CBP, 90 FR 54349, 26 Nov 2025 (OMB 1651-0085) — all ruling subjects, not only origin | Showable with that qualifier |
| Ruling responses per year | 3,628 regional (NCSD) | same notice | Showable; "all ruling subjects" |
| Price paid for US-origin advice | median KRW 5.0M ≈ US$3,600 (23 purchases, 2024–26) | KOTRA export-voucher marketplace purchase records | Showable; "subsidised vouchers; team-compiled" |
| CBP turnaround, Asian-origin rulings | median 21 days (IQR 14–26), n=247 | own count, rulings.cbp.gov | Showable; "team count" |

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
| First answer | about a minute | Heat-gun plan search uncached, `docs/DETAILS.md` (step 4) and README; the recorded full plan took 33 s (proxy row above), D79 sizes a 6-move search at 1–2 minutes when Vertex is healthy | Rounded; never quote as a measured latency |
| Bar lengths in the before/after figure | Unde ÷ before, per row: 1 min ÷ 20 h = 1/1200; 4.6 s ÷ 21 days = 2.5 × 10⁻⁶; $0.0078 ÷ $3,600 = 2.2 × 10⁻⁶ | Drawn to scale per row; anything under 2 px is drawn at 2 px and the figure says so | Drawing only, no new figure |

The other figures in the story are existing rows: 20 hours, 21 days (n = 247), US$3,600, 3,628 ruling responses (market and time), p50 4.6 s and $0.0078 (evaluation), N345938 (plan shown).

Rules: small-n numbers carry CIs; development-only numbers say "development set"; never "approved", "verified", "guaranteed", "cheapest" or "proof that it qualifies". Do not describe a percentage-point gap between subsets as proof of calibration.

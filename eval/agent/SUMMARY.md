# ADK agent evaluation: scores

Written by `python3 -I -B eval/agent/score.py .` from `eval/agent/results/*.jsonl` (rules: docs/DECISIONS.md D450; D510 adds the D470 changes). Do not edit by hand.

## fixcheck-d470-local-attempt1

Fix check (D470), first attempt, not a score: the changed agent on Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini against a local server built from this branch, products a03, a04, a06, a10, a12, 22 Nov 2026 about 08:59-09:01 +08:00. The local server's daily spend cap (0.35 USD) gives one client 10% of it by default, so after about 0.05 USD of server model calls it refused further calls and every run stopped early on a tool error. Kept as run; it tests none of the issues. The second attempt is fixcheck-d470-local.

| Measure | Result |
|---|---|
| Runs finished with an answer | 5/5 (100%) |
| Tool order correct | 1/5 (20%) |
| Arguments valid | 1/5 (20%) |
| Final answer states predict_origin's origin | 2/2 (100%) |
| Cites a ruling the tools returned | 2/5 (40%) |
| Runs naming a ruling no tool returned | 0/5 (0%) |
| "Not a ruling" line, word for word | 5/5 (100%) |
| Target plant added when none was named | 0/2 (0%) |
| Passed every check | 1/5 (20%) |
| predict_origin agrees with CBP (not an accuracy figure, see D450) | 2/2 (100%); the scored product's own ruling was among the tools' rulings in 2 and named in the answer in 2 |
| Wall time per run, median (min to max) | 47.1 s (7.8 to 53.9 s) |
| Agent model cost (gateway-reported) | $0.0973 total, $0.0195 mean per run, $0.0295 max |
| D510 rules: tool order correct | 1/5 (20%) |
| D510 rules: arguments valid | 1/5 (20%) |
| D510 rules: no move claimed after a search with no plan | 0/0 |
| D510 rules: passed every check | 1/5 (20%) |

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a03 | extract product > predict origin | no | no | n/a | no | none | yes | / none | 47.1 s | missing plan_production_moves; tool errors: predict_origin |
| a04 | extract product > predict origin > suggest part hts > plan production moves | yes | yes | yes | yes | none | yes | Indonesia / Indonesia | 53.9 s | tool errors: plan_production_moves |
| a06 | extract product > predict origin > suggest part hts | no | no | yes | yes | none | yes | Vietnam / Vietnam | 49.6 s | missing plan_production_moves; tool errors: suggest_part_hts |
| a10 | extract product | no | no | n/a | no | none | yes | / none | 7.8 s | missing predict_origin; missing plan_production_moves; tool errors: extract_product |
| a12 | extract product | no | no | n/a | no | none | yes | / none | 8.4 s | missing predict_origin; missing plan_production_moves; tool errors: extract_product |

## fixcheck-d470-local

Fix check (D470), not a score and not pooled with D451: the changed agent on Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini against a local server built from this branch (engine, router and verifier on Gemini 3.8 Flash through the same gateway; local server spend cap 0.30 USD), products a03, a04, a06, a10, a12 once each, 22 Nov 2026 about 09:02-09:05 +08:00. a10's plan search hit that cap, so a10 tests nothing; see D472 for the per-issue reading.

| Measure | Result |
|---|---|
| Runs finished with an answer | 5/5 (100%) |
| Tool order correct | 5/5 (100%) |
| Arguments valid | 3/5 (60%) |
| Final answer states predict_origin's origin | 5/5 (100%) |
| Cites a ruling the tools returned | 5/5 (100%) |
| Runs naming a ruling no tool returned | 0/5 (0%) |
| "Not a ruling" line, word for word | 5/5 (100%) |
| Target plant added when none was named | 2/2 (100%) |
| Passed every check | 3/5 (60%) |
| predict_origin agrees with CBP (not an accuracy figure, see D450) | 5/5 (100%); the scored product's own ruling was among the tools' rulings in 5 and named in the answer in 5 |
| Wall time per run, median (min to max) | 98.5 s (78.2 to 116.2 s) |
| Agent model cost (gateway-reported) | $0.1751 total, $0.0350 mean per run, $0.0452 max |
| D510 rules: tool order correct | 5/5 (100%) |
| D510 rules: arguments valid | 5/5 (100%) |
| D510 rules: no move claimed after a search with no plan | 2/2 (100%) |
| D510 rules: passed every check | 5/5 (100%) |

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a03 | extract product > predict origin > suggest part hts > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Cambodia / Cambodia | 98.5 s | export without move_key or plan_index; D510 pass: yes; tool errors: suggest_part_hts |
| a04 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Indonesia / Indonesia | 85.0 s |  |
| a06 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Vietnam / Vietnam | 78.2 s | export without move_key or plan_index; D510 pass: yes |
| a10 | extract product > predict origin > suggest part hts > plan production moves | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 106.5 s | tool errors: plan_production_moves |
| a12 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 116.2 s |  |

## live-gemini-3.8-flash-undelabs

Scored run (D450): the agent from examples/adk-agent, unchanged, on Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini, against https://undelabs.dev/mcp anonymously, on products a01-a12, one run each, three at a time, 22 Nov 2026 08:10-08:35 +08:00. Each run made one anonymous case and one plan search; the tools' own model calls ran on Unde's side and are not in the cost.

| Measure | Result |
|---|---|
| Runs finished with an answer | 12/12 (100%) |
| Tool order correct | 11/12 (92%) |
| Arguments valid | 11/12 (92%) |
| Final answer states predict_origin's origin | 12/12 (100%) |
| Cites a ruling the tools returned | 12/12 (100%) |
| Runs naming a ruling no tool returned | 0/12 (0%) |
| "Not a ruling" line, word for word | 12/12 (100%) |
| Target plant added when none was named | 4/4 (100%) |
| Passed every check | 10/12 (83%) |
| predict_origin agrees with CBP (not an accuracy figure, see D450) | 12/12 (100%); the scored product's own ruling was among the tools' rulings in 12 and named in the answer in 12 |
| Wall time per run, median (min to max) | 313.0 s (113.2 to 395.5 s) |
| Agent model cost (gateway-reported) | $0.4722 total, $0.0394 mean per run, $0.0472 max |
| D510 rules: tool order correct | 9/12 (75%) |
| D510 rules: arguments valid | 12/12 (100%) |
| D510 rules: no move claimed after a search with no plan | 0/1 (0%) |
| D510 rules: passed every check | 8/12 (67%) |

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a01 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 365.3 s |  |
| a02 | extract product > predict origin > suggest part hts > plan production moves > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 395.5 s | tool errors: plan_production_moves |
| a03 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Cambodia / Cambodia | 314.6 s | export without move_key or plan_index; D510: claims a move changes origin after no plan |
| a04 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft > export ruling request draft | yes | yes | yes | yes | none | yes | Indonesia / Indonesia | 371.2 s | export repeated after a draft that worked; D510 pass: no |
| a05 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Malaysia / Malaysia | 351.9 s |  |
| a06 | extract product > predict origin > predict origin > suggest part hts > plan production moves > export ruling request draft | no | yes | yes | yes | none | yes | Vietnam / Vietnam | 311.3 s | predict_origin called more than once (an earlier call was refused; no second case); tool errors: predict_origin |
| a07 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 113.2 s |  |
| a08 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 262.4 s |  |
| a09 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Thailand / Thailand | 371.5 s |  |
| a10 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 285.4 s | export repeated after a draft that worked; D510 pass: no |
| a11 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 160.9 s |  |
| a12 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 247.0 s |  |

## live-v2-run1

Scored run (D510), run 1 of the two pre-registered: the agent from examples/adk-agent as at the D510 commit, on Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini, against https://undelabs.dev/mcp anonymously, products a01-a12, one run each, three at a time, 22 Nov 2026 10:30-10:40 +08:00. From about 10:39:50 the live server refused this address's model calls with 429 client_budget_reached (the anonymous per-client share of the day's model budget, which reopens at 00:00 UTC): a09's plan search, a10's part codes and a11's and a12's extraction were refused, so those four runs stopped early. Kept and scored as the rules say; see D511. Run 2 could not start for the same reason. Changed after the run: the scorer reads "NY N" and "HQ H" as the ruling number (norm_one, D511). It changes one row: a06 named "NY N340438", which the scorer had read as NN340438 and counted as invented; a06 passes under both rule sets after the fix and failed before. The headline: all 8 runs the budget did not touch (a01-a08) passed; 9 of 12 as scored, a09 passing because the agent reported the refused plan call and stopped.

| Measure | Result |
|---|---|
| Runs finished with an answer | 12/12 (100%) |
| Tool order correct | 9/12 (75%) |
| Arguments valid | 8/12 (67%) |
| Final answer states predict_origin's origin | 10/10 (100%) |
| Cites a ruling the tools returned | 10/12 (83%) |
| Runs naming a ruling no tool returned | 0/12 (0%) |
| "Not a ruling" line, word for word | 12/12 (100%) |
| Target plant added when none was named | 3/4 (75%) |
| Passed every check | 8/12 (67%) |
| predict_origin agrees with CBP (not an accuracy figure, see D450) | 10/10 (100%); the scored product's own ruling was among the tools' rulings in 10 and named in the answer in 9 |
| Wall time per run, median (min to max) | 127.0 s (7.3 to 251.9 s) |
| Agent model cost (gateway-reported) | $0.4189 total, $0.0349 mean per run, $0.0521 max |
| D510 rules: tool order correct | 9/12 (75%) |
| D510 rules: arguments valid | 9/12 (75%) |
| D510 rules: no move claimed after a search with no plan | 1/1 (100%) |
| D510 rules: passed every check | 9/12 (75%) |

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a01 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 210.5 s |  |
| a02 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 209.2 s |  |
| a03 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Cambodia / Cambodia | 144.8 s | export without move_key or plan_index; D510 pass: yes |
| a04 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Indonesia / Indonesia | 215.5 s |  |
| a05 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Malaysia / Malaysia | 82.0 s |  |
| a06 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 232.0 s |  |
| a07 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 251.9 s |  |
| a08 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 101.4 s |  |
| a09 | extract product > predict origin > suggest part hts > plan production moves | yes | yes | yes | yes | none | yes | Thailand / Thailand | 109.1 s | tool errors: plan_production_moves |
| a10 | extract product > predict origin > suggest part hts | no | no | yes | yes | none | yes | Vietnam / Vietnam | 80.6 s | missing plan_production_moves; tool errors: suggest_part_hts |
| a11 | extract product | no | no | n/a | no | none | yes | / none | 9.8 s | missing predict_origin; missing plan_production_moves; tool errors: extract_product |
| a12 | extract product | no | no | n/a | no | none | yes | / none | 7.3 s | missing predict_origin; missing plan_production_moves; tool errors: extract_product |

## local-v2-run2

Run 2 of the v2 series (D866), against a local server, not the live site: the agent from examples/adk-agent as on main, on Gemini 3.8 Flash through an OpenAI-compatible LiteLLM gateway to Gemini, against http://localhost:8787/mcp built from main at 1d73ba29 with an uncommitted patch accepting LLM_PROVIDER=litellm (engine, router and verifier on their production model names through the same gateway), in-memory store, products a01-a12, one run each, three at a time, 25 Nov 2026 13:13-13:22 +08:00. Not a pure repeat of run 1: the server's code, corpus and model path differ (D866).

| Measure | Result |
|---|---|
| Runs finished with an answer | 12/12 (100%) |
| Tool order correct | 12/12 (100%) |
| Arguments valid | 10/12 (83%) |
| Final answer states predict_origin's origin | 12/12 (100%) |
| Cites a ruling the tools returned | 12/12 (100%) |
| Runs naming a ruling no tool returned | 0/12 (0%) |
| "Not a ruling" line, word for word | 12/12 (100%) |
| Target plant added when none was named | 4/4 (100%) |
| Passed every check | 10/12 (83%) |
| predict_origin agrees with CBP (not an accuracy figure, see D450) | 12/12 (100%); the scored product's own ruling was among the tools' rulings in 12 and named in the answer in 12 |
| Wall time per run, median (min to max) | 117.0 s (94.8 to 144.5 s) |
| Agent model cost (gateway-reported) | $0.5369 total, $0.0447 mean per run, $0.0594 max |
| D510 rules: tool order correct | 12/12 (100%) |
| D510 rules: arguments valid | 12/12 (100%) |
| D510 rules: no move claimed after a search with no plan | 2/2 (100%) |
| D510 rules: passed every check | 12/12 (100%) |

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a01 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 109.7 s |  |
| a02 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 117.6 s |  |
| a03 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Cambodia / Cambodia | 118.1 s | export without move_key or plan_index; D510 pass: yes |
| a04 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Indonesia / Indonesia | 94.8 s |  |
| a05 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Malaysia / Malaysia | 131.1 s |  |
| a06 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 106.7 s |  |
| a07 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 129.2 s |  |
| a08 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 144.5 s |  |
| a09 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Thailand / Thailand | 106.1 s | export without move_key or plan_index; D510 pass: yes |
| a10 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 127.8 s |  |
| a11 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 116.4 s |  |
| a12 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 108.4 s |  |

## plumbing-scripted-fake

Plumbing only: a scripted stand-in for the model (no model, no cost) against a local server with LLM_PROVIDER=fake. It tests the harness and this scorer, not the agent. The fake server's extract returns an empty draft, so the script sends a committed fixture product (the heat gun) to predict_origin; the fake plan search shows no plans, and the script still calls export, which fails.

| Measure | Result |
|---|---|
| Runs finished with an answer | 14/14 (100%) |
| Tool order correct | 14/14 (100%) |
| Arguments valid | 0/14 (0%) |
| Final answer states predict_origin's origin | 14/14 (100%) |
| Cites a ruling the tools returned | 0/14 (0%) |
| Runs naming a ruling no tool returned | 0/14 (0%) |
| "Not a ruling" line, word for word | 14/14 (100%) |
| Target plant added when none was named | 0/4 (0%) |
| Passed every check | 0/14 (0%) |
| predict_origin agrees with CBP (not an accuracy figure, see D450) | 0/14 (0%); the scored product's own ruling was among the tools' rulings in 14 and named in the answer in 0 |
| Wall time per run, median (min to max) | 0.1 s (0.1 to 0.1 s) |
| Agent model cost (gateway-reported) | $0.0000 total, $0.0000 mean per run, $0.0000 max |
| D510 rules: tool order correct | 14/14 (100%) |
| D510 rules: arguments valid | 0/14 (0%) |
| D510 rules: no move claimed after a search with no plan | 14/14 (100%) |
| D510 rules: passed every check | 0/14 (0%) |

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a01 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a02 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a03 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Cambodia / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a04 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Indonesia / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a05 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Malaysia / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a06 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a07 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a08 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a09 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Thailand / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a10 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a11 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a12 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a13 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Thailand / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |
| a14 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; export names a plan when the search showed none; tool errors: export_ruling_request_draft |

## Series v2: 2 runs per product so far, compared with the baseline (D510)

Runs: live-v2-run1, local-v2-run2. Baseline: live-gemini-3.8-flash-undelabs (one run per product). Pass is under D510 rules unless the row says D450.

| Measure | Result |
|---|---|
| Baseline passed every check, D450 rules as published | 10/12 (83%) |
| Baseline passed every check, rescored under D510 rules | 8/12 (67%) |
| Run 1 (live-v2-run1) passed every check | 9/12 (75%) (D450 rules: 8/12) |
| Run 2 (local-v2-run2) passed every check | 12/12 (100%) (D450 rules: 10/12) |
| All runs pooled | 21/24 (88%) |
| Same pass or fail in every run | 9/12 (75%) (9 pass in all, 0 fail in all) |
| Same predicted origin in every run | 10/12 (83%) |
| Same plan found or not found in every run | 9/12 (75%) |
| Same number of plans shown in every run | 7/12 (58%) |
| Same plan drafted (export plan_id) in every run | 6/12 (50%) |
| Same order of the four required tools in every run | 8/12 (67%) |
| Same full tool sequence in every run | 8/12 (67%) |

| Product | v1 (D450) | v1 (D510) | v2 run 1 | v2 run 2 | Predicted | Plans shown | Drafted plan_id | Issues |
|---|---|---|---|---|---|---|---|---|
| a01 | pass | pass | pass | pass | Vietnam / Vietnam | 4 / 4 | pcba_manufacturing@plant_in_thailand / pcba_manufacturing@target_plant |  |
| a02 | pass | pass | pass | pass | Vietnam / Vietnam | 4 / 4 | pcba_manufacturing@target_plant / pcba_manufacturing@target_plant |  |
| a03 | fail | fail | pass | pass | Cambodia / Cambodia | 0 / 0 | current setup / current setup |  |
| a04 | pass | fail | pass | pass | Indonesia / Indonesia | 1 / 2 | pcba_manufacturing@plant_in_malaysia / pcba_manufacturing@target_plant |  |
| a05 | pass | pass | pass | pass | Malaysia / Malaysia | 2 / 2 | pcba_manufacturing@target_plant / pcba_manufacturing@target_plant |  |
| a06 | fail | fail | pass | pass | Vietnam / Vietnam | 2 / 2 | precision_machined_parts@plant_in_thailand / precision_machined_parts@plant_in_thailand |  |
| a07 | pass | pass | pass | pass | Vietnam / Vietnam | 3 / 2 | led_pcba@plant_in_thailand / led_pcba@plant_in_thailand |  |
| a08 | pass | pass | pass | pass | Vietnam / Vietnam | 1 / 1 | primary_metal_forming@plant_in_cambodia+handles_and_hardware@plant_in_cambodia / primary_metal_forming@plant_in_cambodia+handles_and_hardware@plant_in_cambodia |  |
| a09 | pass | pass | pass | pass | Thailand / Thailand | 0 / 0 | no draft / current setup | run 1: tool errors: plan_production_moves |
| a10 | pass | fail | fail | pass | Vietnam / Vietnam | 0 / 4 | no draft / pcba_manufacturing@target_plant | run 1: missing plan_production_moves; tool errors: suggest_part_hts |
| a11 | pass | pass | fail | pass | none / Vietnam | 0 / 4 | no draft / pcba_manufacturing@target_plant | run 1: missing predict_origin; missing plan_production_moves; tool errors: extract_product |
| a12 | pass | pass | fail | pass | none / Vietnam | 0 / 2 | no draft / precision_machined_parts@target_plant | run 1: missing predict_origin; missing plan_production_moves; tool errors: extract_product |


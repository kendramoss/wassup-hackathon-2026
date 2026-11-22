# ADK agent evaluation: scores

Written by `python3 -I -B eval/agent/score.py .` from `eval/agent/results/*.jsonl` (rules: docs/DECISIONS.md D450). Do not edit by hand.

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

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a03 | extract product > predict origin > suggest part hts > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Cambodia / Cambodia | 98.5 s | export without move_key or plan_index; tool errors: suggest_part_hts |
| a04 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Indonesia / Indonesia | 85.0 s |  |
| a06 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Vietnam / Vietnam | 78.2 s | export without move_key or plan_index |
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

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a01 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 365.3 s |  |
| a02 | extract product > predict origin > suggest part hts > plan production moves > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 395.5 s | tool errors: plan_production_moves |
| a03 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | no | yes | yes | none | yes | Cambodia / Cambodia | 314.6 s | export without move_key or plan_index |
| a04 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft > export ruling request draft | yes | yes | yes | yes | none | yes | Indonesia / Indonesia | 371.2 s |  |
| a05 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Malaysia / Malaysia | 351.9 s |  |
| a06 | extract product > predict origin > predict origin > suggest part hts > plan production moves > export ruling request draft | no | yes | yes | yes | none | yes | Vietnam / Vietnam | 311.3 s | predict_origin called more than once (an earlier call was refused; no second case); tool errors: predict_origin |
| a07 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 113.2 s |  |
| a08 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 262.4 s |  |
| a09 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Thailand / Thailand | 371.5 s |  |
| a10 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 285.4 s |  |
| a11 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 160.9 s |  |
| a12 | extract product > predict origin > suggest part hts > plan production moves > export ruling request draft | yes | yes | yes | yes | none | yes | Vietnam / Vietnam | 247.0 s |  |

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

| Product | Tools called | Order | Args | Origin said | Cites | Invented | Disclaimer | CBP / predicted | Time | Notes |
|---|---|---|---|---|---|---|---|---|---|---|
| a01 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a02 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a03 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Cambodia / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a04 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Indonesia / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a05 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Malaysia / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a06 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a07 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a08 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a09 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Thailand / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a10 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a11 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a12 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a13 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Thailand / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |
| a14 | extract product > predict origin > plan production moves > export ruling request draft | yes | no | yes | no | none | yes | Vietnam / China | 0.1 s | predict without the named family; tool errors: export_ruling_request_draft |


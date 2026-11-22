# ADK agent evaluation: scores

Written by `python3 -I -B eval/agent/score.py .` from `eval/agent/results/*.jsonl` (rules: docs/DECISIONS.md D450). Do not edit by hand.

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
| predict_origin agrees with CBP (not an accuracy figure, see D450) | 0/14 (0%); source ruling among the tools' rulings in 14 |
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


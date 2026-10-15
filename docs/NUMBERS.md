# On-screen numbers and their sources

Every number that may appear in the demo, deck or results page. **Status** says whether it can be shown today. "Proxy" = produced through the LiteLLM proxy: development only, replace with the Vertex AI re-run before the submission (`make eval-test`, `make eval-summary`).

## Evaluation (results page — served by `GET /v1/eval/summary`)

| Number | Value | Source | Status |
|---|---|---|---|
| Held-out origin accuracy (377 rulings, Jan–Sep 2026) | Go port via proxy 89.9% (86.5–92.6); research run 89.1% (85.6–91.9) | `eval/results/final-gemini-4-argon-prec-engine-v1-litellm.jsonl`; research `RESULTS-v2.md` | Proxy — **replace with Vertex** |
| Best simple baseline (most-mentioned country) | 60.5% (55.5–65.3) | research `RESULTS-v2.md` (no model involved) | Showable |
| Other baselines | final-assembly 40.6%, last-mentioned 39.8%, majority 12.2% | research `RESULTS-v2.md` | Showable |
| Fast path (confidence ≥ 0.92 and in scope), held-out | 78.5% of cases at 96.6% (93.9–98.2), router-v2 (at the old 0.95 bar: 70.3% at 97.7%) | final rows + `eval/results/scope-final-router-v2.jsonl` | Proxy — replace with Vertex |
| Confidence ≥ 0.95 alone, held-out | 71.9% at 97.8% (95.3–99.0) | final rows | Proxy. Do **not** show the research "65% at 98.0%": it describes a different routed pipeline |
| Change detection from scratch, held-out 2026 pairs | 68% (47–84), n=22; always-change baseline 63.6%; balanced accuracy 61.6% | research `RESULTS-v2.md` | Showable, always with both baselines; label "weak" |
| Move verifier, development set (54 pre-2026 pairs) | precision 34/34 (90–100), recall 74%, specificity 8/8; with CBP's current origin given: 38/38 (91–100), 83%, 8/8 | `eval/results/flips-dev-verifier-*.jsonl` | Proxy; must say **"development set"** |
| Cost per origin prediction | $0.0078 (promo price) / $0.0156 (list) | final rows × Gemini Developer API prices verified 2026-10-14 | Proxy tokens; Vertex prices unchecked |
| Cost per verified move | $0.031–0.032 (promo) | verifier dev rows | Proxy |
| Full plan, heat gun | $0.22 promo / $0.45 list, 33 s, 7 engine calls + 6 verifier runs | `eval/results/plan-heatgun-2022-local-litellm.json` | Proxy; demo fixture |
| Latency per prediction | p50 5.0 s / p95 6.4 s | final rows | Proxy (includes proxy overhead) |

## Demo story

| Number / claim | Value | Source | Status |
|---|---|---|---|
| Heat gun 2022 origin | CBP held China | CBP ruling N328954 (15 Nov 2022) | Showable |
| Heat gun 2026 origin after the change | CBP held Thailand | CBP ruling N357771 (29 Jan 2026) | Showable |
| Duty difference China vs Thailand, heat gun | ≈25 points (≈41.7% vs ≈16.7% ad valorem) | `docs/DUTY-SOURCES.md` (HTS 8419.89.95 inferred; Section 301 List 2) | Showable with "product-specific; classification to be confirmed by a broker" |
| Plan shown | motor-fan assembly + PCBA to Thailand; supporting N347421, counter N345859 | `eval/results/plan-heatgun-2022-local-litellm.json` | Proxy demo run; re-run on Vertex before recording. Verifier confidence varies 0.90–0.95 across runs: at 0.90 it appears as a greyed "below confidence bar" row (D21) — say so if it does |
| Duty effect / added cost on plan cards | $500k/yr and $180k/yr | fixture inputs (illustrative — editable) | Show only as "illustrative — editable" |

## Market and time (from `hackathon-research/evidence/fees/LEDGER.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Preparation per ruling request | 20 hours | CBP, 90 FR 54349, 26 Nov 2025 (OMB 1651-0085) — all ruling subjects, not only origin | Showable with that qualifier |
| Ruling responses per year | 3,628 regional (NCSD) | same notice | Showable; "all ruling subjects" |
| Price paid for US-origin advice | median KRW 5.0M ≈ US$3,600 (23 purchases, 2024–26) | KOTRA export-voucher marketplace purchase records | Showable; "subsidised vouchers; team-compiled" |
| CBP turnaround, Asian-origin rulings | median 21 days (IQR 14–26), n=247 | own count, rulings.cbp.gov | Showable; "team count" |

Rules: small-n numbers carry CIs; development-only numbers say "development set"; never "approved", "verified", "guaranteed", "cheapest" or "proof that it qualifies".

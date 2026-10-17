# On-screen numbers and their sources

Every number that may appear in the demo, deck or results page. **Status** says whether it can be shown today. The held-out engine, fast path, cost and latency rows below are the Vertex AI re-run (D39). "Proxy" = produced through the LiteLLM proxy: development only, and not a substitute for those rows.

## Evaluation (results page — served by `GET /v1/eval/summary`)

| Number | Value | Source | Status |
|---|---|---|---|
| Held-out origin accuracy (377 rulings, Jan–Sep 2026) | **90.2% (86.8–92.8)**, Vertex AI, frozen engine-v1. Proxy re-run 89.9% (86.5–92.6); research run 89.1% (85.6–91.9) | `eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl` (D39). Proxy file kept for the comparison | Official. Proxy and research figures stay labelled as such |
| Best simple baseline (most-mentioned country) | 60.5% (55.5–65.3) | research `RESULTS-v2.md` (no model involved) | Showable |
| Other baselines | final-assembly 40.6%, last-mentioned 39.8%, majority 12.2% | research `RESULTS-v2.md` | Showable |
| Fast path (confidence ≥ 0.92 and in scope), held-out | 79.6% of cases (300/377) at 96.0% (93.1–97.7), Vertex AI, router-v2. Proxy was 78.5% at 96.6% (93.9–98.2). At the old 0.95 bar, proxy coverage was 70.3% at 97.7% | final Vertex rows + `eval/results/scope-final-router-v2-vertex.jsonl` | Official. The proxy figure stays labelled proxy |
| Confidence ≥ 0.95 alone, held-out | 71.9% at 97.8% (95.3–99.0) | final rows | Proxy. Do **not** show the research "65% at 98.0%": it describes a different routed pipeline |
| Change detection from scratch, held-out 2026 pairs | 68% (47–84), n=22; always-change baseline 63.6%; balanced accuracy 61.6% | research `RESULTS-v2.md` | Showable, always with both baselines; label "weak" |
| Move verifier, development set (54 pre-2026 pairs) | precision 34/34 (90–100), recall 74%, specificity 8/8; with CBP's current origin given: 38/38 (91–100), 83%, 8/8 | `eval/results/flips-dev-verifier-*.jsonl` | Proxy; must say **"development set"** |
| Cost per origin prediction | $0.0078 (promo price) / $0.0157 (list), Vertex AI held-out | final Vertex rows × Gemini Developer API prices verified 2026-10-14 | Token counts are Vertex. The Vertex price sheet was not re-checked; this is the same promo/list schedule as the proxy runs |
| Cost per verified move | $0.031–0.032 (promo) | verifier dev rows | Proxy |
| Full plan, heat gun | $0.22 promo / $0.45 list, 33 s, 7 engine calls + 6 verifier runs | `eval/results/plan-heatgun-2022-local-litellm.json` | Proxy; demo fixture |
| Latency per prediction | p50 4.6 s / p95 5.7 s | final Vertex rows | Official Vertex AI. The proxy run was p50 5.0 s / p95 6.4 s |

## Demo story

| Number / claim | Value | Source | Status |
|---|---|---|---|
| Heat gun 2022 origin | CBP held China | CBP ruling N328954 (15 Nov 2022) | Showable |
| Heat gun 2026 origin after the change | CBP held Thailand | CBP ruling N357771 (29 Jan 2026) | Showable |
| Duty difference China vs Thailand, heat gun | ≈25 points (≈41.7% vs ≈16.7% ad valorem) | `docs/DUTY-SOURCES.md` (HTS 8419.89.95 inferred; Section 301 List 2) | Showable with "product-specific; classification to be confirmed by a broker" |
| Plan shown | none: no plan clears 0.92. Closest move: motor-fan assembly to Thailand, check 0.90 (under the bar), supporting N325810, no counter-ruling, net $380k/yr; motor-fan + switch, cord and minor parts 0.75 (N345859) | `docs/DEMO.md` production run (backend-00038-zw2, verifier-v2, Vertex), cached for everyone | One production run, kept as is (D75: a steadier verifier-v3 didn't help, so no re-roll). The Plan screen shows "No plan cleared the bar" with "Draft for the closest move". The LiteLLM run (`eval/results/plan-heatgun-2022-verifier-v2-litellm.json`) had motor-fan + switch/cord at 0.92; proxy only |
| Duty effect / added cost on plan cards | $500k/yr and $180k/yr | fixture inputs (illustrative — editable) | Show only as "illustrative — editable" |

## Market and time (from `hackathon-research/evidence/fees/LEDGER.md`)

| Number | Value | Source | Status |
|---|---|---|---|
| Preparation per ruling request | 20 hours | CBP, 90 FR 54349, 26 Nov 2025 (OMB 1651-0085) — all ruling subjects, not only origin | Showable with that qualifier |
| Ruling responses per year | 3,628 regional (NCSD) | same notice | Showable; "all ruling subjects" |
| Price paid for US-origin advice | median KRW 5.0M ≈ US$3,600 (23 purchases, 2024–26) | KOTRA export-voucher marketplace purchase records | Showable; "subsidised vouchers; team-compiled" |
| CBP turnaround, Asian-origin rulings | median 21 days (IQR 14–26), n=247 | own count, rulings.cbp.gov | Showable; "team count" |

Rules: small-n numbers carry CIs; development-only numbers say "development set"; never "approved", "verified", "guaranteed", "cheapest" or "proof that it qualifies".

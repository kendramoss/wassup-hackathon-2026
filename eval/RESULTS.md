# Origin Proof — evaluation results

All runs are listed in `results/RUNLOG.jsonl`; per-case rows are in `results/*.jsonl`. Data: `data/` and `splits/` (sha256 in `SHA256SUMS`).

> **Status (15 Oct 2026): every number below is "proxy, development only".** It was produced through the LiteLLM proxy because the team has no Vertex AI access yet. Official numbers come from re-running `make eval-test` with `LLM_PROVIDER=vertex` and replace this table. Do not put these in the README results table or on slides.

## Origin engine — Go port check (engine-v1, gemini-4-argon, 5 BM25 precedents dated before each case)

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

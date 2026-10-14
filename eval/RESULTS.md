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

## Fast path (triage)
Rule: auto-accept when engine confidence ≥ 0.95 **and** the router says the case is in scope (docs/DECISIONS.md D7). Router labels for dev are measured below; router labels for final are pending the frozen router (one run).

## Not yet measured
- Official Vertex AI numbers (blocked on GCP access).
- Planner and verifier (Gate C); full-plan cost.

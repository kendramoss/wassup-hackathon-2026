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

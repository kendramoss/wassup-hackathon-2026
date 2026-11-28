# Small benchmark expansion implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to execute each independent lane. Steps use checkbox syntax for tracking.

**Goal:** Give each current small feature benchmark a source-grounded synthetic challenge study with at least 100 reserved test cases, improve the implementation on separate development cases, and display the measured result without disguising synthetic cases as independent customs decisions.

**Architecture:** Separate runners and datasets per independent engine family; existing production engine entry points are exercised directly. Each lane retains its own protocol, frozen datasets, development attempts, candidate identity, per-case test results and compact summary. The coordinator integrates public evidence after verifying every lane. Historical real-case studies remain intact.

**Tech stack:** Existing Go engines/runners, deterministic Python dataset/rendering scripts, approved model gateway for model-backed scoring, existing Next.js Accuracy screen.

**Spec:** User instruction, 28 November 2026, tracked in #981 and children #982–#986: find online examples, synthesize informative variations, improve features and test on 100+ cases. No model fine-tuning.

## Global constraints

- Author and committer: Mizore66. Required project commit attribution and PR footer apply.
- No frontend changes except coordinated Accuracy evidence and related tests. No owner work, workflows, deployment scripts, Terraform or production-data mutations.
- Paid model scoring uses only the approved gateway; never Vertex. Never print or write credentials, gateway host or credential-file location.
- Programme cap **$1,000 total**, raised directly by the user on 28 November while this campaign was running; ledger at campaign start $361.6195. Initial lane model allowances remain specialists/Japan $35, media $15, move verifier $30, deterministic lanes $0 and coordinator reserve $20. These are initial allocations, not feature-quality gates; coordinator can record reallocation within the new programme cap before additional runs. Check ledger before calls and record actual costs promptly.
- Do not install frontend dependencies in backend-only lanes. Initial disk availability is 13GiB; generated artifacts must be bounded, and merged worktrees removed without deleting branches.
- No force-push, reset, stash, clean, branch deletion or killing another lane's processes. Only coordinator merges under the existing release lock.
- Search cap: three external source operations for the FTA lane, two for legal rules, two for specialists/Japan, two shared between media and move verification, plus the coordinator's completed generic discovery: ten total. Count source searches, fetches and any additional discovery against the lane allowance. Reuse archived primary documents and do not substitute commercial secondary examples as official truth.
- Reserved decisions: campaign D965; FTA D966/D967; legal rules D968/D969; specialists/Japan D970/D971; media D972/D973; verifier D974/D975; public evidence D976. Each paid lane must commit its DECISIONS preregistration before the first paid evaluation.

## Shared evidence contract

Each lane creates `eval/challenges/<lane>/PROTOCOL.md`, versioned dataset(s), per-case result(s), and `summary.json`. The summary is plain JSON with one entry per capability:

```json
{
  "schema_version": 1,
  "evidence_kind": "source_grounded_synthetic",
  "features": [
    {
      "id": "atiga",
      "label": "ATIGA rules",
      "candidate_revision": "full git SHA of frozen candidate",
      "dataset_sha256": "SHA-256 of frozen dataset bytes",
      "source_families": 0,
      "development": {"cases": 0, "matched": 0, "errors": 0},
      "test": {"cases": 0, "matched": 0, "mismatched": 0, "errors": 0},
      "cost_usd": 0,
      "latency_p50_ms": 0,
      "latency_p95_ms": 0,
      "limitations": []
    }
  ]
}
```

Zero values above illustrate types, not promised results. Replace them with measured values. Report extra metrics in additional named fields where needed. Test errors and abstentions must never disappear from the denominator. If expected abstention is a correct result, make that explicit. A conditional precision/recall/decided-only metric must retain its own denominator.

Every case retains an id, split, source-family id, primary-source references, a rationale for its expected result, semantic variation tags, engine input and expected checks. Preserve generated expected labels before scoring. Do not grade via the same function that implements the candidate rule.

Minimum useful coverage: positive, negative and missing/uncertain inputs; cases crossing thresholds; exclusions and exceptions; trade-agreement membership/cumulation; conflicting evidence where the engine supports it. Formatting-only rewrites are robustness tests, not new legal scenarios. Near-duplicate/parent relationships and family counts stay visible. Do not claim 100 independent legal sources or an i.i.d. real-world confidence interval.

Development and test cases must have disjoint normalized inputs. Prefer disjoint scenario templates/source families where feasible; otherwise disclose shared families rather than claim a blind study. Tests generated by the implementer are not independently authored real-world ground truth even when reserved from tuning.

## Task 1 — FTA calculators (#982)

**Scope/files:** `eval/challenges/fta/`, `backend/cmd/ftachallenge/` (or a small existing-runner extension), and only necessary fixes in `backend/internal/{atiga,aseanfta,rcep}`. No central evidence/UI files.

**Produces:** Summary features `atiga`, `akfta`, `acfta`, `rcep` using the shared contract; at least 100 reserved test cases per feature, plus separate development data. Existing seven/eight/five examples and RCEP real-case studies are unchanged.

- [ ] Inspect each existing input schema and authority sources; write a bounded source/coverage map.
- [ ] Write scorer tests that fail for missing/duplicate cases, wrong expected results, cross-split duplicates, mismatched summaries and fabricated agreement membership.
- [ ] Build source-grounded development scenarios and run them; fix demonstrated implementation defects test-first, recording negative results.
- [ ] Freeze candidate, reserved inputs, expected labels and hashes in D966 before first test score.
- [ ] Score once, retain all failures, record D967; ensure deterministic reproduction agrees byte-for-byte.
- [ ] Verify Go formatting, vet, full tests, claimscheck; commit, push and open a PR. Coordinator merges.

## Task 2 — USMCA/textile/tariff rules (#983)

**Scope/files:** `eval/challenges/legal-rules/`, `backend/cmd/legalchallenge/`, and necessary fixes in `backend/internal/{usmca,textilerules,tariffwatch}`. No FTA runner, central evidence/UI files or paid models.

**Produces:** Summary features `usmca_rules`, `textile_rules`, `tariff_watch_rates`; each with 100+ reserved cases and separate development cases. Distinguish invented tariff snapshots from actual government tariff revisions.

- [ ] Inspect current engines and primary rule sources; identify distinct process, exclusion, rate-delta and unknown-input families.
- [ ] Write failing scorer/integrity tests, including source-family accounting and explicit error retention.
- [ ] Construct and run development cases; fix only demonstrated defects with regression tests.
- [ ] Commit D968 candidate/dataset/label freeze before test; score once and retain every mismatch in D969/results.
- [ ] Recompute summaries from raw rows and verify all relevant backend checks; open PR without merging.

## Task 3 — Specialist/Japan models (#984)

**Scope/files:** `eval/challenges/specialists/`, a bounded runner under `backend/cmd/`, and demonstrated dev-only prompt/rule changes in `backend/internal/engine` or Japan engine packages. Coordinate any broad engine/shared-model changes before editing. No other lane files or frontend.

**Produces:** Summary features `textile_specialist`, `pharma_specialist`, `part102_specialist`, `japan_engine`; routing challenge metrics may reuse the same appropriate source-grounded cases, with distinct denominators.

- [ ] Read existing specialist and Japan harnesses; cite primary law/rulings behind expected synthetic outcomes.
- [ ] Write harness tests for split isolation, unknown/abstention handling, source-ruling exclusion and complete scoring.
- [ ] Build diverse dev data and at least 100 reserved cases per feature; keep unsupported/ambiguous labels explicit rather than invent a customs holding.
- [ ] Preregister D970 before paid development scoring; improve only from dev, report cost/latency and freeze final candidate and test hashes before first test score.
- [ ] Score once through the gateway within $35; preserve every attempt/error, record actual spend and D971.
- [ ] Verify offline reproduction and full backend checks; open PR without merging.

## Task 4 — BOM image extraction (#985)

**Scope/files:** `eval/challenges/media/`, existing `eval/media` renderers and `backend/cmd/mediacheck` only as needed, plus dev-justified extraction changes.

**Produces:** `media_extraction` summary with at least 100 reserved documents, field-level and document-level results, distinct content/layout-family counts, and separate dev data.

- [ ] Author varied source-grounded BOMs and rendering families; no filename/metadata label leakage.
- [ ] Test rendering determinism and scorer strictness before model scoring.
- [ ] Preregister D972; improve only on development documents; freeze candidate, test manifest and image checksums.
- [ ] Score once through gateway within $15, report omissions/hallucinations and all errors, write D973 and ledger entries.
- [ ] Verify runner/scorer tests; retain only bounded run-of-record image artifacts and reproducible render definitions; open PR without merging.

## Task 5 — Move verifier (#986)

**Scope/files:** `eval/challenges/verifier/`, a small existing/new verifier runner and dev-justified `backend/internal/verifier` changes. Full plan search is not the same capability and must not be implied by this study.

**Produces:** `move_verifier` summary with 100+ reserved tests, preferably at least 100 positive and 100 negative cases so precision/recall have meaningful support. Give actual denominators even if they remain smaller than that preference.

- [ ] Build a primary-ruling-pair scenario map and legal label rubric; do not treat arbitrary counterfactuals as actual CBP rulings.
- [ ] Test source-answer exclusion, split integrity, stable result scoring, preservation/changed/uncertain controls and complete errors.
- [ ] Preregister D974 and dev fixtures before paid scoring; improve on dev only; freeze final test and candidate.
- [ ] Score once through gateway within $30; record D975, all raw outcomes, precision/recall denominators, cost and latency.
- [ ] Verify backend tests and reproducibility; open PR without merging.

## Task 6 — Public evidence and release (#981)

**Scope/files:** `frontend/src/lib/market-evidence.ts` or a small dedicated challenge evidence module, `frontend/src/components/op/screens/accuracy-page.tsx`, related frontend tests, `docs/NUMBERS.md`, `docs/ROADMAP.md`, `docs/DECISIONS.md`, `eval/RESULTS.md`, run ledger/checksum manifest.

- [ ] Validate every result against frozen case hashes and raw rows. Do not accept padded counts or a rerun advertised as first-run held-out.
- [ ] Keep source-grounded synthetic challenge results clearly separate from historical real rulings, teaching examples, old versions and demonstrations.
- [ ] Replace tiny-example-led presentation with the larger challenge evidence where relevant, retaining original source evidence in Details.
- [ ] Pin UI metrics to committed summaries in tests; show denominators, errors, uncertain outcomes and source-family limits.
- [ ] Run full Go/frontend tests, lint/build, claimscheck, offline scorer reproduction and real-browser 1280/390 checks.
- [ ] Review, merge only our PRs on green under the release lock, watch exact deployments and verify live Accuracy. Update the shared handover with outcomes and remaining work.

## Completion rules

Do not claim the campaign complete until all current small-feature lanes are accounted for. If a feature cannot be fairly measured with 100 new tests within available source semantics, document that instead of filling a headline with fabricated accuracy. No owner/expert gate is introduced. If a reserved test exposes a defect, retain that result; improvements require a separately declared new test round, not relabelling the scored set.

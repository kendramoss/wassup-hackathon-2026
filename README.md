# Origin Proof

**Plan the production change before you build it.** Origin Proof predicts the country of origin U.S. Customs and Border Protection (CBP) would likely assign to a manufactured product, citing only earlier CBP rulings, and searches where operations could be performed for plans that would reach a target origin. Each plan is checked against precedent and shown only if it is **precedent-supported**.

> Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

Google Cloud AI Builder Cup 2026 (JAPAC), theme: Manufacturing.

## The problem

When a manufacturer moves production out of China, CBP decides the product's origin with the *substantial-transformation* test, and origin decides whether China-specific duties apply. For our demo product (a heat gun, HTS 8419.89.95) that is about a 25-point difference in ad-valorem duty, China vs Thailand origin ([docs/DUTY-SOURCES.md](docs/DUTY-SOURCES.md); exposure is product-specific). Today a factory finds out by paying advisers and waiting for CBP: CBP estimates 20 hours of preparation per ruling request (90 FR 54349, all ruling subjects), and our count of 247 CBP origin rulings on Asian goods in 2025–26 shows a median of 21 days to answer.

The user is the industrialisation or trade-compliance lead at a China-plus-one contract manufacturer (Thailand, Vietnam, Malaysia, Korea, Taiwan).

## How it works

```
frontend (Next.js, Cloud Run)  ──HTTP/JSON──►  backend (Go, Cloud Run)
                                                 ├─ router    Gemini 3.8 Flash: product family + origin-regime check
                                                 ├─ engine    Gemini 4 Argon + 5 earlier CBP rulings (BM25, dated strictly before the case)
                                                 │            → origin, key factors, confidence, cited rulings
                                                 ├─ triage    confidence ≥ 0.92 and in scope → auto; otherwise needs expert review
                                                 ├─ clarify   below the bar: is an earlier ruling on point? conflicting rulings? follow-up questions
                                                 ├─ planner   single/double moves of operation groups to capable plants
                                                 │            → render variant facts → engine → keep moves that reach the target
                                                 ├─ verifier  ADK Go agent (Argon) + search_rulings / read_ruling tools
                                                 │            → precedent-supported only if an earlier ruling held the same kind of change decisive
                                                 ├─ export    draft ruling request for a broker to file
                                                 └─ cases     saved cases (JSON files), scenarios side by side, ruling watch
eval (Go CLI) → engine / router / verifier on frozen splits → eval/results/*.jsonl, eval/RESULTS.md
```

1. **Predict.** Paste a CBP-style description, or upload the bill of materials as CSV or Excel (components, where each is made, operations by plant; `POST /v1/products/import`). The engine reads it with the five most similar CBP rulings dated before the case and returns origin, key factors, confidence and the rulings it relied on. The router labels the origin regime: textile (19 CFR 102.21), USMCA/Part 102 and pharmaceutical-ingredient cases go to expert review.
2. **Plan.** Pick a target origin. The planner moves *operation groups* (e.g. motor-fan assembly = stamp laminations, wind rotor, build stator, fit fan; PCBA SMT; housing moulding) to plants that can perform them, one or two groups at a time, and asks the engine about each variant.
3. **Verify.** For variants predicted to reach the target, an ADK agent searches and reads earlier CBP rulings and answers "precedent-supported" only when it finds one where CBP treated the same kind of change as decisive; it also returns a counter-ruling where a similar change did not work. Only precedent-supported plans with verifier confidence ≥ 0.95 are shown; the rest are returned greyed out with the reason.
4. **Rank and export.** Plans are ranked by duty effect minus added cost — best-ranked within the searched moves and your cost inputs (illustrative — editable). The chosen plan exports as a draft CBP ruling request. The search streams its progress (`Accept: text/event-stream`), so each move appears as it is predicted and checked (~25 s for the heat gun).

Below the confidence bar, a **clarify** review says whether an earlier ruling is on point, shows conflicting earlier rulings with the difference that decided them, and asks up to three follow-up questions; when nothing is on point the case is routed **needs more information**. Answers are added to the facts and the case is re-analysed. Cases are saved, can compare up to three **production scenarios** side by side (with yearly duty, savings and added cost from your economics), and can be **re-checked against rulings CBP issued after the case date** (ruling watch).

Model choices are measured: Argon and Flash are equally accurate with precedents, Argon is faster (research p95 5 s vs 18 s). Every response and eval row records the provider, model IDs and prompt versions.

## Results

Data: 2,790 CBP rulings (2023-01 to 2026-09, rulings.cbp.gov, public domain). Labels are CBP's own stated holdings. Held-out test: 377 rulings dated Jan–Sep 2026 (355 regex-labelled, 22 hand-labelled), never used to develop prompts; precedents are restricted to rulings dated before each case (model pre-training exposure not excluded).

| Method (held-out, n = 377) | Accuracy (95% CI) |
|---|---|
| Majority country (Vietnam) | 12.2% (9.3–15.9) |
| Last country mentioned in facts | 39.8% (35.0–44.8) |
| Final-assembly country | 40.6% (35.8–45.6) |
| Most-mentioned country (best simple baseline) | 60.5% (55.5–65.3) |
| **Origin Proof engine (Gemini 4 Argon + dated precedents), Vertex AI** | *pending the Vertex AI re-run* |
| Fast path (confidence ≥ 0.92 and in scope), Vertex AI | *pending* |

On the development split through the proxy, the engine is 90.9% accurate overall; it auto-accepts 79% of cases (confidence ≥ 0.92, in scope) at 95.1% accuracy (held-out: 78.5% at 96.6%), and the rest go to expert review. 0.92 is the lowest bar at which accuracy stays at or above 95% on both splits (D32). Development numbers through the LiteLLM proxy (labelled "proxy, development only", not official) are in [eval/RESULTS.md](eval/RESULTS.md): the Go engine reproduces the research result on the held-out split within the 2-point acceptance band, and the move verifier's development-set results are there with CIs. Change detection from scratch on 22 held-out 2026 pairs is weak (68% vs a 63.6% always-change baseline; balanced accuracy 61.6%) — which is why every proposed change must be backed by a precedent.

### Negative results (kept on purpose)
- **One shared playbook** of legal guidance lowered accuracy versus the plain engine.
- **Routed specialists** overfit: dev 91.9% → held-out 88.3%, below the single engine; adjudication mostly overruled a correct generalist. The router is kept only as a triage label and never overrides the engine.
- **Agentic search** did not improve single-product origin (dev 89.8% vs 90.2%) at 3–4× the cost; it is used only to verify moves.
- **Change detection** from scratch over-predicts changes (specificity 38% on held-out pairs), the costly error for a planner.
- **Every in-context accuracy change** tried on dev landed within ±0.5 points of the plain engine: voting over precedent orders, hybrid and embedding retrieval, written CBP rules for the confident errors, and CBP's reasoning shown with each precedent (`docs/DECISIONS.md` D22, D27, D28). The remaining confident errors are stable judgments; fine-tuning on Vertex AI is the next step.
- **Follow-up answers** did not move split votes toward CBP's answer on dev (D26). The engine read "not stated" as evidence, so such answers are now left out of the re-analysis. Clarify ships as help for the broker, not as an accuracy gain.

## Limitations
- The planner — the distinctive part — is the least validated. The verifier has no held-out test yet (its prompt was written after seeing the held-out flip failures; a clean test needs CBP rulings published after 25 Sep 2026). Demo fixtures were adjusted during development; they are demonstrations, not evaluations.
- Inputs in the benchmark are CBP's restatements of each request, not raw manufacturer BOMs.
- Labels come from one extraction pipeline plus a second extractor and a 30-item audit, not an expert panel.
- Origin prediction covers the substantial-transformation regime only. The planner covers one family (motorised tools and small appliances); a second template (LiDAR modules) shows the extension path.
- Plans model duty effect and user-entered cost only — not feasibility, tooling, capacity or transition costs. Duty rates for the demo are sourced and dated; classification is inferred and should be confirmed by a broker.
- The precedent check is not deterministic through the development proxy: the same heat-gun plan cleared the 0.95 bar in one run and checked 0.92 in the next.
- Cases are saved as files on local disk; on Cloud Run they need Firestore (pending the GCP setup). The ruling watch sees new CBP rulings only after the corpus is rebuilt.
- Precedent support is not a ruling. Plans go to a broker or counsel and, if needed, to CBP.

## Competitors (credited)
Altana does agentic origin determination from BOM and value-chain data, with CBP-ruling checks, human review and sourcing scenarios. Trademo markets BOM-driven origin with continuous monitoring. KYG, SAP GTS, Descartes and Thomson Reuters cover origin and FTA analysis. Kinaxis, Coupa and Optilogic optimise production placement against tariffs without substantial-transformation logic. Our angle: automatic search over *where operations are performed*, with CBP-precedent origin as the objective and every plan checked against accepting and rejecting rulings. We found no documentation of that combination — not proof it doesn't exist.

## Run locally

Requirements: Go 1.27, `make`. The corpus (≈40 MB of public CBP rulings) is fetched once and never committed.

```bash
cd backend && go run ./cmd/corpus fetch && go run ./cmd/corpus build && cd ..   # ~50 min at 1 request/s
make test lint verify-data                                                        # offline
```

Model access is configured by environment only:

| Provider | Use | Env |
|---|---|---|
| `vertex` | deployment and official numbers | `LLM_PROVIDER=vertex GOOGLE_CLOUD_PROJECT=… GOOGLE_CLOUD_LOCATION=global` + `gcloud auth application-default login` |
| `litellm` | local development only (refuses to start on Cloud Run) | `LLM_PROVIDER=litellm LITELLM_BASE_URL=… LITELLM_API_KEY=…` (never in files, CI or logs) |
| `fake` | offline tests | `LLM_PROVIDER=fake` |

```bash
make run          # API on :8080; cases saved under data/cases (CASES_DIR); contract in docs/api/openapi.yaml
make smoke        # one live model call
make eval-report  # summaries of the committed eval runs, no model calls
```

API: `POST /v1/products/import`, `POST /v1/cases/predict`, `GET /v1/cases`, `GET /v1/cases/{id}`, `POST /v1/cases/{id}/answers | plan | scenarios | watch | export`, `POST /v1/watch/run`, `GET /v1/rulings/{id}`, `GET /v1/demos`, `GET /v1/eval/summary`.

Eval commands (`backend/cmd/eval`): `run -split dev|final` (final needs `-confirm-final`; never tune on it), `compare`, `scope`, `flips`, `plan -fixture testdata/heatgun_2022.json`, `predict`, `clarify`, `torn`, `learnability`, `summary`, `report`. Every run is appended to `eval/results/RUNLOG.jsonl`.

## Repository

| Path | What |
|---|---|
| `backend/` | Go API and CLIs. Packages: `domain`, `config`, `llm` (LiteLLM, Vertex, fake), `corpus`, `retrieval`, `engine`, `router`, `triage`, `planner` (+ `templates/`), `verifier` (ADK), `clarify`, `cross`, `export`, `store`, `httpapi`, `evalkit`, `evalsummary` |
| `frontend/` | Next.js app (being rebuilt for Origin Proof) |
| `eval/` | Splits, labels, flip pairs (sha256-pinned), result files, `RESULTS.md` |
| `docs/` | `ARCHITECTURE.md`, `DECISIONS.md`, `PROMPTS.md`, `api/openapi.yaml`, `DUTY-SOURCES.md`, `NUMBERS.md`, `SETUP-GCP.md` |

## Deploy

CI (`.github/workflows/ci.yml`) runs gofmt, build, vet, staticcheck and tests on every PR. Pushing to `main` deploys changed services to Cloud Run (`deploy.yml`, keyless via Workload Identity Federation) with `LLM_PROVIDER=vertex`. Before the first Origin Proof deploy: run [docs/SETUP-GCP.md](docs/SETUP-GCP.md), and make the corpus available to the service (see `docs/DECISIONS.md`).

Data: CBP rulings from [rulings.cbp.gov](https://rulings.cbp.gov) (U.S. government works, public domain).

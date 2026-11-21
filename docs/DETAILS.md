# Unde Labs — details

The full method, results, limitations and developer notes. The [README](../README.md) is the short version. Decision numbers (D32, D90, ...) refer to [DECISIONS.md](DECISIONS.md).

## The problem

When a manufacturer moves production out of China, CBP decides the product's origin with the *substantial-transformation* test, and origin decides whether China-specific duties apply. For our demo product (a heat gun, HTS 8419.89.95) that is about a 25-point difference in ad-valorem duty, China vs Thailand origin ([docs/DUTY-SOURCES.md](DUTY-SOURCES.md); exposure is product-specific). Today a factory finds out by paying advisers and waiting for CBP: CBP estimates 20 hours of preparation per ruling request (90 FR 54349, all ruling subjects), and our count of 247 CBP origin rulings on Asian goods in 2025–26 shows a median of 21 days to answer.

The user is the industrialisation or trade-compliance lead at a China-plus-one contract manufacturer (Thailand, Vietnam, Malaysia, Korea, Taiwan).

## How it works

```
frontend (Next.js, Cloud Run)  ──HTTP/JSON──►  backend (Go, Cloud Run)
                                                 ├─ router    Gemini 3.8 Flash: product family + origin-regime check
                                                 ├─ engine    Gemini 4 Argon + 5 earlier CBP rulings (BM25, dated strictly before the case)
                                                 │            → origin, key factors, confidence, cited rulings
                                                 ├─ triage    calibrated confidence ≥ 0.90 and in scope → auto; otherwise needs expert review
                                                 ├─ clarify   below the bar: is an earlier ruling on point? conflicting rulings? follow-up questions
                                                 ├─ planner   single/double moves of operation groups to capable plants
                                                 │            → render variant facts → engine → keep moves that reach the target
                                                 ├─ verifier  ADK Go agent (Argon) + search_rulings / read_ruling tools
                                                 │            → precedent-supported only if an earlier ruling held the same kind of change decisive
                                                 ├─ export    draft ruling request for a broker to file
                                                 └─ cases     saved cases (JSON files), scenarios side by side, ruling watch
eval (Go CLI) → engine / router / verifier on frozen splits → eval/results/*.jsonl, eval/RESULTS.md
```

1. **Predict.** Paste a CBP-style description, or upload the bill of materials as CSV or Excel (components, where each is made, operations by plant; `POST /v1/products/import`). A photo or scan of a BOM is read by Gemini into an editable draft that the user checks before predicting (`POST /v1/products/extract-media`; see [MULTIMODAL.md](MULTIMODAL.md), accuracy not yet measured). The engine reads it with the five most similar CBP rulings dated before the case and returns origin, key factors, confidence and the rulings it relied on. The router labels the origin regime, and textile (19 CFR 102.21), USMCA/Part 102 and pharmaceutical-ingredient cases go to their own engines with their own fast-path bars (D51, D61, D62).
2. **Plan.** Pick a target origin. The planner moves *operation groups* (e.g. motor-fan assembly = stamp laminations, wind rotor, build stator, fit fan; PCBA SMT; housing moulding) to plants that can perform them, one, two or three groups at a time (three-group moves up to `PLAN_MAX_MOVES`, D84), and asks the engine about each variant. Before searching, `/plan/estimate` gives the number of moves, the time and the cost, with no model call (D87).
3. **Verify.** For variants predicted to reach the target, an ADK agent searches and reads earlier CBP rulings and answers "precedent-supported" only when it finds one where CBP treated the same kind of change as decisive; it also returns a counter-ruling where a similar change did not work. Only precedent-supported plans with verifier confidence ≥ 0.92 are shown; the rest are returned greyed out with the reason.
   - **Visible search:** every verdict carries the agent's search trail, each query with the rulings it returned and each ruling read (D80).
   - **Relaxed search:** CBP's search needs every word to match, so when a query finds too little, shorter queries are tried and the local corpus fills the rest (D89).
   - **Panel near the bar:** a check landing between 0.89 and 0.95 gets two second opinions, a skeptic view and a same-product view; the majority decides (D83).
   - **Near-miss advice:** for the closest moves under the bar, the agent says what differs from the ruling that nearly supports it and what would match it. This is advice only; it never changes the bar (D81).
   - **Repeatable:** every model call carries a fixed seed (D76).
4. **Rank and export.** Plans are ranked by duty effect minus added cost — best-ranked within the searched moves and your cost inputs (illustrative — editable). The chosen plan exports as a draft CBP ruling request. The search streams its progress (`Accept: text/event-stream`), so each move appears as it is predicted and checked (about a minute for the heat gun uncached, a few seconds from the verdict cache).

Below the confidence bar, a **clarify** review says whether an earlier ruling is on point, shows conflicting earlier rulings with the difference that decided them, and asks up to three follow-up questions; when nothing is on point the case is routed **needs more information**. Answers are added to the facts and the case is re-analysed. Cases are saved, can compare up to three **production scenarios** side by side (with yearly duty, savings and added cost from your economics), can be **re-checked against rulings CBP issued after the case date** (ruling watch), can **compare target countries** (one plan search per country, ranked by net savings), and have an **ask-the-case agent** that explains the prediction from the rulings and runs what-ifs.

**Duty rates** are live: the MFN rate from the HTS plus the Section 301 duties for every HTS line (the four China lists and the 2026 country-wide headings), parsed from HTS Chapter 99, and the case shows the breakdown (D90).

Model choices are measured: Argon and Flash are equally accurate with precedents, Argon is faster (research p95 5 s vs 18 s). Every response and eval row records the provider, model IDs and prompt versions.

## Results

How the splits are built, what the results trade off and how the benchmark is kept honest: [EVALUATION.md](EVALUATION.md).

Data: 2,790 labelled CBP rulings (2023-01-10 to 2026-09-25, rulings.cbp.gov, public domain; 2,553 with a single primary origin) plus 291 further Part 102 rulings in the 3,081-file facts pool. Labels are CBP's own stated holdings. Held-out test: 377 rulings dated Jan–Sep 2026 (355 regex-labelled, 22 hand-labelled), never used to develop prompts; precedents are restricted to rulings dated strictly before each case (earlier held-out rulings are retrievable: 414 of 1,885 retrieved precedent slots) (model pre-training exposure not excluded).

| Method (held-out, n = 377) | Accuracy (95% CI) |
|---|---|
| Majority country (Vietnam, the development set's most common origin; recomputed) | 12.2% (9.3–15.9) |
| Last country mentioned in facts (research definition; reported, unverified: source not in this repository) | 39.8% (35.0–44.8) |
| Final-assembly country (reported, unverified: source not in this repository) | 40.6% (35.8–45.6) |
| Most-mentioned country (recomputed; the research figure, 60.5%, is unverified) | 60.7% (55.7–65.5) |
| First country named in the facts (best simple rule; recomputed) | 63.4% (58.4–68.1) |
| **Unde engine (Gemini 4 Argon + dated precedents), Vertex AI** | **90.2% (86.8–92.8)** |
| Fast path (calibrated confidence ≥ 0.90, the same cases as stated ≥ 0.92, and in scope), Vertex AI | 79.6% of cases (300 of 377) at 96.0% (93.1–97.7), 288 of 300 |

Recomputed baselines: `go run ./cmd/eval baselines` on the committed facts, no model (D155). The two Vertex rows are the official re-run of the frozen engine (engine-v1, BM25, K = 5, Gemini 4 Argon on Vertex AI, location `global`), scored once on the held-out split.

Development split, three figures, all the same engine: on Vertex AI, 422 of 469 = 90.0% unadjusted, and 91.1% (88.2–93.4) on the 463 rulings left after the scoring adjustments (D23), with the fast path at 79.5% of cases and 96.5% accuracy; through the LiteLLM proxy (development only, not official), 90.9% on the 463.

Proxy run of the held-out split, kept apart from the official rows above: fast path 78.5% of cases at 96.6% (93.9–98.2). It is not a second official figure.

A second held-out split of 83 rulings published 28 Sep to 13 Nov 2026 scores 92.8% (85.1–96.6) through an OpenAI-compatible gateway to the same model, scored once ([EVALUATION.md](EVALUATION.md#6-the-second-held-out-split-21-nov-2026)).

Cases below the bar go to expert review. The bar is applied to the calibrated confidence (0.90, D160), which accepts exactly the cases a stated 0.92 does. 0.92 is the lowest stated bar at which accuracy stays at or above 95% on both splits (D32). Development numbers through the LiteLLM proxy (labelled "proxy, development only", not official) are in [eval/RESULTS.md](../eval/RESULTS.md): the Go engine reproduces the research result on the held-out split within the 2-point acceptance band, and the move verifier's development-set results are there with CIs. Change detection from scratch on 22 held-out 2026 pairs is weak (68% vs a 63.6% always-change baseline; balanced accuracy 61.6%) — which is why every proposed change must be backed by a precedent.

### Negative results (kept on purpose)
- **One shared playbook** of legal guidance lowered accuracy versus the plain engine.
- **Routed specialists** overfit: dev 91.9% → held-out 88.3%, below the single engine; adjudication mostly overruled a correct generalist. The router is kept only as a triage label and never overrides the engine.
- **Agentic search** did not improve single-product origin (dev 89.8% vs 90.2%) at 3–4× the cost; it is used only to verify moves.
- **Change detection** from scratch over-predicts changes (specificity 38% on held-out pairs), the costly error for a planner.
- **Every in-context accuracy change** tried on dev landed within ±0.5 points of the plain engine: voting over precedent orders, hybrid and embedding retrieval, written CBP rules for the confident errors, and CBP's reasoning shown with each precedent (`docs/DECISIONS.md` D22, D27, D28). The remaining confident errors are stable judgments; fine-tuning on Vertex AI is the next step.
- **Follow-up answers** did not move split votes toward CBP's answer on dev (D26). The engine read "not stated" as evidence, so such answers are now left out of the re-analysis. Clarify ships as help for the broker, not as an accuracy gain.

### The demo plans, on production (Vertex, 18 Oct)
- **Heat gun** (CBP ruling N328954, China, 2022). The plan shown: make the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand. Precedent check 0.96, supported by N345938, no counter-ruling. This is the change the manufacturer actually made: CBP's 2026 ruling N357771 held Thailand after the motor, fan, PCBA, knob, cord and back cover moved. The plan covers all of them except the outer shells, whose moulding is already Thai.
- **LED tub light** (N350085, China). The plan shown: make the LED PCBA in Thailand, check 0.94 on N345899. This is exactly CBP's next ruling, N351143 (Thailand).

Across every precedent-check run we scored (dev and the 2026 pairs, 182 verdicts at or above the 0.92 bar), none was wrong (D86). The bar is strict: about 30% of real changes clear it in every run, and the rest go to a broker as "below the bar" with their supporting ruling.

## Limitations
- Confidence is the model's self-report against a bar, not a calibrated probability. Held-out accuracy at confidence 0.90–0.95 is 78.1% (66.6–86.5) and at 0.80–0.90 is 58.1% (40.8–73.6); on average the stated confidence is 94.7% against 90.2% observed (ECE 0.059, Brier 0.081; [EVALUATION.md](EVALUATION.md#42-calibration-computed-now)). See docs/claims-reconciliation.md.
- The planner — the distinctive part — is the least validated. The precedent check (verifier-v2, D74) was scored once on the 2026 flip pairs: every precedent-supported verdict was a real origin change (12/12 and 11/11), but part of its prompt was written after the research team saw failures on those pairs, so they are not a clean held-out test; a clean test needs CBP rulings published after 25 Sep 2026. On 18 Oct, CROSS had none with an origin holding (its newest NY ruling was dated 14 Aug), so that test could not be built yet (D82). Demo fixtures were adjusted during development; they are demonstrations, not evaluations.
- Inputs in the benchmark are CBP's restatements of each request, not raw manufacturer BOMs.
- Labels come from one extraction pipeline plus a second extractor and a 30-item audit, not an expert panel.
- Origin prediction has engines for substantial transformation, textiles (19 CFR 102.21), Part 102 marking origin (the `usmca_part102` label; not USMCA qualification or regional value content) and pharmaceutical active ingredients (D51, D61, D62). The specialist engines are off by default. A case the router labels for an engine that is off goes to expert review; `unclear` and `substantial_transformation` labels are handled by the general engine, and there is no jurisdiction detector or guard for a question about another country's rules. Scope, unsupported use cases and the EU BOI and USMCA RVC deferral: [scope-and-jurisdiction.md](scope-and-jurisdiction.md). The planner has 13 family templates (D138: ten added from CBP precedent, each group citing its rulings) and always uses the general engine; a new family is a template file. 57.0% of the 2026 held-out rulings match a family with a template (4.2% with the first three); products outside them get an origin but no plan.
- The origin, plan, compare, scenarios and draft screens each carry one short, screen-only note: results are suggestions for a licensed customs broker to review, not rulings; "best" is the best of the moves or scenarios searched under the duty and cost inputs shown (which may be prefilled or illustrative), with factors the model does not include left out; the tool is for U.S. origin only. The notes are not part of the broker letter, its download or its print (D134). The note text is `copy.review` and `copy.jurisdiction` in `frontend/src/lib/copy.ts`.
- Plans model duty effect and user-entered cost only. An optional operating-cost mode (D136, [modelled-operational-costs.md](modelled-operational-costs.md)) adds labour, logistics, one-time capex and retooling over a chosen horizon and lead-time and minimum-order checks; capacity, time value, tax, yield and subsidies are still not modelled, and nothing is called cost-optimal. Duty rates for the demo are sourced and dated; classification is inferred and should be confirmed by a broker.
- The precedent check is not fully deterministic run to run. Before D76 we sent no seed, and Gemini returned different searches for the same request even at temperature 0, so the same heat-gun move checked between 0.75 and 0.95. Every model call now carries a fixed seed. On the dev flip pairs, identical verdicts between two runs rose from 13/54 to 21/54, with no change in accuracy (D76). Google treats the seed as best effort, and a multi-turn agent turns a one-token difference into a different ruling, so some variance remains. A verdict cache (Firestore on Cloud Run) keeps repeat searches identical. Demo verdicts are the first recorded ones, with two documented exceptions, each approved once: the tub-light checks were re-run with the seed (D76, D77), and the heat gun's three-group check was re-run after its stored verdict was shown to have searched with no results (D89).
- Saved cases are stored in Firestore on Cloud Run (`agent-backend` has `roles/datastore.user`) and as files locally (D39). The ruling watch sees new CBP rulings only after the corpus is rebuilt.
- Precedent support is not a ruling. Plans go to a broker or counsel and, if needed, to CBP.

## Competitors (credited)
Altana does agentic origin determination from BOM and value-chain data, with CBP-ruling checks, human review and sourcing scenarios. Trademo markets BOM-driven origin with continuous monitoring. KYG, SAP GTS, Descartes and Thomson Reuters cover origin and FTA analysis. Kinaxis, Coupa and Optilogic optimise production placement against tariffs without substantial-transformation logic. Our angle: automatic search over *where operations are performed*, with CBP-precedent origin as the objective and every plan checked against accepting and rejecting rulings. We found no documentation of that combination — not proof it doesn't exist.

## Run locally

Requirements: Go 1.27, `make`. The corpus (≈40 MB of public CBP rulings) is committed under `backend/data` and baked into the container image, so a fresh checkout runs offline. Fetching is only needed to rebuild it:

```bash
make test lint verify-data                                                        # offline, no GCP credentials
# only to rebuild the corpus (~50 min at 1 request/s):
cd backend && go run ./cmd/corpus fetch && go run ./cmd/corpus build && cd ..
```

Model access is configured by environment only:

| Provider | Use | Env |
|---|---|---|
| `vertex` (default) | development, evals and deployment | `GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 GOOGLE_CLOUD_LOCATION=global` + `gcloud auth application-default login` |
| `fake` | offline tests and UI work (fixed answer, no model calls) | `LLM_PROVIDER=fake` |

The LiteLLM proxy used before Vertex access was removed (D49); only `vertex` and `fake` are accepted. Result files named `*-litellm.*` are earlier proxy runs and stay labelled "proxy" until re-run on Vertex. A gitignored `.env` at the repo root holds the local settings: `set -a; source .env; set +a; make run`.

Vertex needs **application-default credentials**, which are separate from `gcloud auth login`: run `gcloud auth application-default login` once, or the server exits at startup and every page shows "Couldn't reach the Unde API". For UI work with no model calls and no cost, use the explicit fake mode: `LLM_PROVIDER=fake make run` (fixed, clearly-labelled answers; the Accuracy page and demo data are still real). It needs no GCP project, is refused on Cloud Run, and the server prints both hints when Vertex credentials are missing. Cases are saved locally under `data/cases`; only `CASES_STORE=firestore` (Cloud Run) needs Firestore, and the precedent corpus is bundled.

```bash
LLM_PROVIDER=fake make run                         # credential-free API on :8080 (simulated answers)
# second terminal:
cd frontend && BACKEND_URL=http://localhost:8080 npm run dev
make smoke        # one live model call (needs vertex credentials)
make eval-report  # summaries of the committed eval runs, no model calls
```

API: `POST /v1/products/import`, `POST /v1/products/extract-media`, `POST /v1/cases/predict`, `GET /v1/cases`, `GET /v1/cases/{id}`, `POST /v1/cases/{id}/answers | plan | scenarios | compare-targets | ask | watch | export`, `POST /v1/watch/run`, `GET /v1/rulings/{id}`, `GET /v1/demos`, `GET /v1/eval/summary`.

Eval commands (`backend/cmd/eval`): `run -split dev|final` (final needs `-confirm-final`; never tune on it), `compare`, `scope`, `flips`, `plan -fixture testdata/heatgun_2022.json`, `predict`, `clarify`, `torn`, `learnability`, `summary`, `report`. Every run is appended to `eval/results/RUNLOG.jsonl`.

## Repository

| Path | What |
|---|---|
| `backend/` | Go API and CLIs. Packages: `domain`, `config`, `llm` (Vertex, fake), `corpus`, `retrieval`, `engine`, `router`, `triage`, `planner` (+ `templates/`), `verifier` (ADK), `clarify`, `cross`, `export`, `store`, `httpapi`, `evalkit`, `evalsummary` |
| `frontend/` | Next.js app: landing page, cases (predict, plan, scenarios, export), triage queue, accuracy page, developer docs |
| `eval/` | Splits, labels, flip pairs (sha256-pinned), result files, `RESULTS.md` |
| `docs/` | `ARCHITECTURE.md`, `DECISIONS.md`, `PROMPTS.md`, `api/openapi.yaml`, `DUTY-SOURCES.md`, `NUMBERS.md`, `SETUP-GCP.md` |

## Deploy

CI (`.github/workflows/ci.yml`) runs gofmt, build, vet, staticcheck and tests on every PR. Pushing to `main` deploys changed services to Cloud Run (`deploy.yml`, keyless via Workload Identity Federation) with `LLM_PROVIDER=vertex`. Before the first deploy: finish [docs/SETUP-GCP.md](SETUP-GCP.md). Cases are stored in Firestore on Cloud Run; the corpus is baked into the image and copied to the corpus bucket (D37).

Data: CBP rulings from [rulings.cbp.gov](https://rulings.cbp.gov) (U.S. government works, public domain).

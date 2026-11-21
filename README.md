# Unde Labs

**Plan the production change before you build it.**

*Unde* is Latin for "from where?", the question customs asks about every product.

- **Live app:** https://undelabs.dev (also at https://frontend-88903435351.us-central1.run.app)
- **Built for:** Google Cloud AI Builder Cup 2026 (JAPAC), theme: Manufacturing

> Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling.

## Problem statement (Manufacturing theme)

Manufacturers moving production out of China must decide which operations to move, and where. U.S. Customs (CBP) sets a product's country of origin by where those operations happen, and origin sets the duty. Example: our demo heat gun pays about 41.7% duty from China and 16.7% from Thailand ([sources](docs/DUTY-SOURCES.md)).

Checking is slow, and a wrong move is costly:
- **20 hours** to prepare a CBP ruling request ([CBP, 90 FR 54349](https://www.federalregister.gov/citation/90-FR-54349), all ruling types)
- **21 days** median for CBP to answer (our count of 247 origin rulings on Asian goods, 2025–26, [rulings.cbp.gov](https://rulings.cbp.gov); [docs/NUMBERS.md](docs/NUMBERS.md))
- A move that doesn't change origin is only found out after production has moved.

Unde turns a manufacturer's operational data (bill of materials, where each part is made, which plant does each step) into:
- **Insight:** the origin CBP would likely assign, with the rulings it's based on
- **Recommendations:** production moves that reach a target origin, each checked against CBP precedent
- **Action:** a draft ruling request for a customs broker

Theme directions it covers: production optimization and supply-chain intelligence.

## What it does

When parts come from several countries, CBP asks where the product became a new thing ("substantial transformation"). Unde answers that ahead of time:

1. **Describe the product.** Paste a description, or upload the bill of materials (CSV or Excel).
2. **See the likely origin.** Gemini reads the product with the most similar past CBP rulings and predicts the origin, with a confidence and the rulings it relied on.
3. **Get a plan.** Pick a target country. Unde tries moving groups of manufacturing steps between your plants. An AI agent checks each move against real CBP rulings, and a plan is shown only if an earlier ruling backs it (**precedent-supported**).
4. **Hand it to a broker.** Export the chosen plan as a draft ruling request.

It's built for the trade-compliance or industrialisation lead at a contract manufacturer moving work to Thailand, Vietnam, Malaysia, Korea or Taiwan.

## Why it matters

- **Duty:** for our demo heat gun, China vs Thailand origin is about a **25-point** difference in duty. Unde uses live U.S. rates, including Section 301 duties, for every HTS line.
- **Time:** CBP estimates **20 hours** of preparation per ruling request, and recent origin rulings took a median of **21 days**. Unde gives a first answer in about a minute.
- **Risk:** moving production and still getting China origin is the costly mistake. Every plan is checked against past rulings, including ones where a similar change did *not* work.

## Other features

- **Follow-up questions** when the prediction is unsure, then a re-check with your answers
- **Side-by-side scenarios** with yearly duty, savings and added cost
- **Compare target countries** (e.g. Thailand vs Vietnam), ranked by net savings
- **Ruling watch:** re-check saved cases against rulings CBP issued later
- **Ask the case:** an agent that explains the prediction and runs what-ifs
- **API** for ERP and sourcing tools ([docs/api/openapi.yaml](docs/api/openapi.yaml))

## How a business could use it

1. **Start with the web app.** Upload a bill of materials, review the origin and plans, and send the draft to your broker.
2. **Make it a step before any production move.** Check the plan before approving a supplier change, a new plant or a country shift.
3. **Connect your systems.** Send bills of materials from your ERP through the API.
4. **Keep watching.** Re-check saved cases on a schedule as CBP issues new rulings.
5. **Keep the broker in the loop.** Unde speeds up the decision; a broker or a CBP ruling still confirms it.

## Does it work?

Tested on **377 real CBP rulings** from January to September 2026 that were never used to tune prompts or settings (earlier rulings, including earlier test rulings, can be retrieved as precedent for later ones, but never a case's own ruling or a later one):

| Method | Accuracy |
|---|---|
| Best simple rule (most-mentioned country) | 60.5% (reported; source not in this repository, unverified) |
| **Unde** (Gemini 4 Argon on Vertex AI + past rulings), all 377 cases | **90.2%** (340 of 377) |
| Unde, confident cases only: confidence at least 0.92 and in scope, 79.6% of cases | 96.0% (288 of 300) |

The 96.0% covers a subset the model's own confidence selected, so it is not comparable with the all-case baseline, and a gap in percentage points is not evidence that the confidence scores are calibrated. See [docs/claims-reconciliation.md](docs/claims-reconciliation.md).

On the two demo products, the plan Unde shows matches what really happened:

- **Heat gun:** Unde's plan (move the motor-fan assembly, circuit board and switch to Thailand) matches CBP's later 2026 ruling holding Thai origin.
- **LED tub light:** Unde's plan (make the LED circuit board in Thailand) is exactly CBP's next ruling.

Across 182 precedent checks that cleared the bar, none was wrong. The bar is strict: about 30% of real changes clear it, and the rest go to a broker with their closest supporting ruling. Full results, confidence intervals and what didn't work are in [docs/DETAILS.md](docs/DETAILS.md#results).

## Limitations

- **The planner is the least tested part.** A clean test needs CBP rulings published after 25 Sep 2026, and none with an origin holding existed yet on 18 Oct.
- **Benchmark claims are reconciled, with limits.** Every headline number is recomputed from committed files by `cd backend && go run ./cmd/claimscheck`; the 60.5% baseline cannot be reproduced from this repository. See [docs/claims-reconciliation.md](docs/claims-reconciliation.md).
- **U.S. origin only.** The tool answers U.S. CBP country-of-origin questions under four regimes (substantial transformation, textile 19 CFR 102.21, Part 102 marking origin, pharmaceutical active ingredient). EU Binding Origin Information, USMCA regional value content and preferential-origin claims are not supported, and nothing detects or blocks a question about another country's rules. See [docs/scope-and-jurisdiction.md](docs/scope-and-jurisdiction.md).
- **Plans count duty and the costs you enter only.** An optional operating-cost mode adds labour, logistics, one-time capex and retooling spread over a horizon you choose, plus lead-time and minimum-order checks. It prices only what you typed, lists what it leaves out (capacity, time value, tax, yield, subsidies) and never calls a plan the cheapest or best possible; see [docs/modelled-operational-costs.md](docs/modelled-operational-costs.md).
- **Three product families** have plan templates (motorised tools and small appliances, LiDAR modules, LED lights). Adding one is a template file.
- **Results vary a little run to run.** A fixed seed and a verdict cache keep repeat searches stable.
- **It's a hackathon prototype.** Company sign-in and data hosting would need work before production use.

All limitations: [docs/DETAILS.md](docs/DETAILS.md#limitations).

What we would build to turn the prototype into an enterprise product, phase by phase: [docs/ROADMAP.md](docs/ROADMAP.md).

## How it's built

```
Next.js frontend (Cloud Run)  ──►  Go backend (Cloud Run)  ──►  Gemini on Vertex AI
                                     │                          (4 Argon, 3.8 Flash)
                                     ├─ origin engine + 2,790 labelled CBP rulings (BM25)
                                     ├─ planner (moves steps between plants)
                                     ├─ verifier (ADK agent that searches CBP rulings)
                                     └─ cases in Firestore
```

## Run it locally

You need Go 1.27 and `make`. The public-CBP-rulings corpus is already committed and bundled in the image, so you can start without fetching it (fetching is only needed to rebuild the corpus, ~50 minutes).

```bash
make run                                # API on :8080, no GCP credentials needed for UI work
```

Predict/plan calls need a model. To work on the UI with fixed, clearly-labelled simulated answers and no cost, use the explicit fake mode:

```bash
LLM_PROVIDER=fake make run              # API on :8080
# in a second terminal, point the frontend at it:
cd frontend && BACKEND_URL=http://localhost:8080 npm run dev
```

For real model calls, get Application Default Credentials (`gcloud auth application-default login`, separate from `gcloud auth login`) and set `GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026`. `vertex` is the default; `fake` is refused on Cloud Run so production cannot silently use simulated results. Tests, evals, the API list and deploy notes are in [docs/DETAILS.md](docs/DETAILS.md#run-locally).

## Naming

| | |
|---|---|
| Product / team | **Unde Labs** — the product itself is *Unde* |
| Former name | Origin Proof (renamed 18 Oct 2026) |
| Repository / GCP project | `wassup-hackathon-2026` |

The rename was user-facing. Legacy technical identifiers are kept on purpose so
stored data and deployments still work: the `origin-proof-*` hash salts, Secret
Manager secret, corpus bucket and staging Cloud Run service, plus research paths
and recorded eval artefacts. See [docs/DECISIONS.md](docs/DECISIONS.md). The one
remaining env-var fallback, `ORIGINPROOF_RESEARCH_RESULTS`, is a deprecated alias
of `UNDE_RESEARCH_RESULTS` in a single opt-in parity test; it stays until no local
setup still depends on it.

## Repository

| Path | What's there |
|---|---|
| `backend/` | Go API, origin engine, planner, verifier, eval tools |
| `frontend/` | Next.js web app |
| `eval/` | Test sets and results ([eval/RESULTS.md](eval/RESULTS.md)) |
| `docs/` | [DETAILS.md](docs/DETAILS.md) (full write-up), [ROADMAP.md](docs/ROADMAP.md), [DECISIONS.md](docs/DECISIONS.md), [ARCHITECTURE.md](docs/ARCHITECTURE.md), [NUMBERS.md](docs/NUMBERS.md), [api/openapi.yaml](docs/api/openapi.yaml) |

Data: CBP rulings from [rulings.cbp.gov](https://rulings.cbp.gov) (U.S. government works, public domain).

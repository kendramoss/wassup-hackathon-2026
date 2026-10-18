# Unde Labs

**Plan the production change before you build it.**

*Unde* is Latin for "from where?", the question customs asks about every product.

- **Live app:** https://undelabs.dev (also at https://frontend-guxax2teha-uc.a.run.app)
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

Tested on **377 real CBP rulings** from January to September 2026 that were never used to build it:

| Method | Accuracy |
|---|---|
| Best simple rule (most-mentioned country) | 60.5% |
| **Unde** (Gemini 4 Argon on Vertex AI + past rulings) | **90.2%** |
| Unde, confident cases only (about 80% of cases) | 96.0% |

On the two demo products, the plan Unde shows matches what really happened:

- **Heat gun:** Unde's plan (move the motor-fan assembly, circuit board and switch to Thailand) matches CBP's later 2026 ruling holding Thai origin.
- **LED tub light:** Unde's plan (make the LED circuit board in Thailand) is exactly CBP's next ruling.

Across 182 precedent checks that cleared the bar, none was wrong. The bar is strict: about 30% of real changes clear it, and the rest go to a broker with their closest supporting ruling. Full results, confidence intervals and what didn't work are in [docs/DETAILS.md](docs/DETAILS.md#results).

## Limitations

- **The planner is the least tested part.** A clean test needs CBP rulings published after 25 Sep 2026, and none with an origin holding existed yet on 18 Oct.
- **Plans count duty and the costs you enter only**, not feasibility, tooling, capacity or transition costs.
- **Three product families** have plan templates (motorised tools and small appliances, LiDAR modules, LED lights). Adding one is a template file.
- **Results vary a little run to run.** A fixed seed and a verdict cache keep repeat searches stable.
- **It's a hackathon prototype.** Company sign-in and data hosting would need work before production use.

All limitations: [docs/DETAILS.md](docs/DETAILS.md#limitations).

## How it's built

```
Next.js frontend (Cloud Run)  ──►  Go backend (Cloud Run)  ──►  Gemini on Vertex AI
                                     │                          (4 Argon, 3.8 Flash)
                                     ├─ origin engine + 2,790 past CBP rulings
                                     ├─ planner (moves steps between plants)
                                     ├─ verifier (ADK agent that searches CBP rulings)
                                     └─ cases in Firestore
```

## Run it locally

You need Go 1.27 and `make`. Fetching the public CBP rulings takes about 50 minutes, once.

```bash
cd backend && go run ./cmd/corpus fetch && go run ./cmd/corpus build && cd ..
gcloud auth application-default login   # for Vertex AI
make run                                # API on :8080
```

To work on the UI with no model calls and no cost, set `LLM_PROVIDER=fake`. Tests, evals, the API list and deploy notes are in [docs/DETAILS.md](docs/DETAILS.md#run-locally).

## Repository

| Path | What's there |
|---|---|
| `backend/` | Go API, origin engine, planner, verifier, eval tools |
| `frontend/` | Next.js web app |
| `eval/` | Test sets and results ([eval/RESULTS.md](eval/RESULTS.md)) |
| `docs/` | [DETAILS.md](docs/DETAILS.md) (full write-up), [DECISIONS.md](docs/DECISIONS.md), [ARCHITECTURE.md](docs/ARCHITECTURE.md), [NUMBERS.md](docs/NUMBERS.md), [api/openapi.yaml](docs/api/openapi.yaml) |

Data: CBP rulings from [rulings.cbp.gov](https://rulings.cbp.gov) (U.S. government works, public domain).

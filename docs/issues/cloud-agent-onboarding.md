# Onboarding: Origin Proof (AI Builder Cup 2026)

You're joining a two-person team, Anas (GitHub: Mizore66) and Kendra (kmthere), building **Origin Proof** for the Google Cloud AI Builder Cup 2026 (JAPAC, theme: Manufacturing). **Code freeze: 17 Oct 2026, 14:00 SGT.** The output is a 2:45 demo video plus finals Q&A, so the judges are the first audience.

## Where you're working
- The real repo, `kendramoss/wassup-hackathon-2026`, is **private** and you can't reach it. You're on a **fork** that mirrors its branches.
- Push your work as branches on the fork. Anas moves them into the real repo and opens the PRs.
- Never push to `main` and never deploy: a push to `main` in the real repo deploys to Cloud Run.

## What the product does
Origin Proof predicts the country of origin U.S. Customs (CBP) would likely assign to a manufactured product, under the substantial-transformation test, citing only earlier CBP rulings. It then searches which production steps could move to which plant to reach a target origin, for example China → Thailand to avoid China tariffs. Each plan is checked against precedent, and the output is a draft ruling request for a customs broker.

**Users:** trade-compliance or industrialisation leads at China-plus-one contract manufacturers.

**Pipeline** (Go backend in `backend/`, Next.js frontend in `frontend/`):
- **Engine:** Gemini 4 Argon plus 5 earlier rulings retrieved by BM25 (strictly dated before the case). Returns origin, confidence and the rulings used.
- **Router:** Gemini 3.8 Flash. Labels the origin regime; textile, USMCA and pharmaceutical cases go to review.
- **Triage:** auto-accept when confidence ≥ 0.92 and in scope; otherwise expert review. Below the bar, a clarify review asks follow-up questions (the `needs_info` route).
- **Planner:** single or double moves of operation groups to capable plants. The engine predicts each variant.
- **Verifier:** an ADK agent with ruling search and read tools. A plan is shown only if it's precedent-supported with confidence ≥ 0.92.
- **Other features:**
  - draft export (Markdown)
  - saved cases
  - scenarios side by side with yearly duty and net
  - ruling watch (re-check with newer rulings)
  - streamed plan search (SSE)
  - BOM import (CSV/Excel)
  - compare target countries
  - ask-the-case agent (ADK: get_case, search and read rulings, what_if)
- **LLM providers by `LLM_PROVIDER`:**
  - `vertex`: deployment and official numbers
  - `litellm`: development proxy; refuses to run on Cloud Run
  - `fake`: tests

## Branches (stacked; each PR targets the branch before it)
| # | Branch | Contents | PR |
|---|---|---|---|
| 1 | `feat/origin-proof-skeleton` | config, LLM providers, domain, CI, docs | open, not merged |
| 2 | `feat/origin-proof-engine` | corpus, BM25, engine, router, triage, eval CLI | open |
| 3 | `feat/origin-proof-planner` | planner, ADK verifier, duty sources | open |
| 4 | `feat/origin-proof-api` | HTTP API, OpenAPI, demos, bundled corpus | open |
| 5 | `feat/origin-proof-accuracy` | clarify, cases, scenarios, watch, streaming, BOM import, accuracy experiments | open |
| 6 | `feat/origin-proof-compare-agent` | compare countries, ask-the-case agent, multi-country demo | open |
| 7 | `feat/origin-proof-gcp` (Kendra) | Firestore case store, GCP setup done (#37) | to open |
| 8 | `feat/origin-proof-tuning` (Kendra) | tuning split, Vertex tuning data and script (#38), not launched | to open |

**Frontend design** sits directly on `main`:
- `feat/origin-proof-frontend-design`: `docs/design/DESIGN.md` plus HTML key frames.
- `feat/origin-proof-design-compare-ask` (Kendra, stacked on it): design for compare and ask (#46).

**Rule:** nothing under `docs/design/` or `frontend/` goes on the backend branches, and no backend files go on the frontend branches.

**Nothing is merged yet.** The plan is to merge bottom-up once GCP and Vertex work. Merging is Anas's call.

## Issues
- **#37 GCP setup:** done by Kendra except IAM (setup steps 4–5).
- **#38 Fine-tune the engine on Vertex:** data built, job not launched.
- **#46 Design for compare and ask:** done on Kendra's branch.
- **Build the frontend:** drafted, not posted yet. The full checklist is `docs/issues/build-frontend.md` on the frontend design branch (Anas's local copy). It's blocked until the #46 PR is merged.

## Numbers so far (all through the LiteLLM proxy: "proxy, development only", not official)
- **Dev (463 after scoring adjustments):** 90.9% overall. Fast path at 0.92: 79% of cases at 95.1%.
- **Held-out (377 rulings, Jan–Sep 2026):** 89.9% overall. Fast path at 0.92: 78.5% at 96.6% (93.9–98.2).
- **Baselines:** best simple baseline 60.5% (most-mentioned country).
- **Accuracy experiments:** every in-context accuracy change landed within ±0.5 points (voting, hybrid retrieval, written rules, CBP reasoning in context). Fine-tuning is the next lever.
- **Verifier variance:** results vary between proxy runs. The heat gun's motor-fan + PCBA move checked 0.92 one run and 0.65–0.90 others. A verdict cache (`VERIFIER_CACHE_DIR`) makes repeat searches identical.

## Read first
- `README.md`
- `docs/DECISIONS.md`: D1–D38, every decision with numbers
- `eval/RESULTS.md`
- `docs/NUMBERS.md`: the source of every on-screen number
- `docs/api/openapi.yaml`
- `docs/DUTY-SOURCES.md`
- `docs/SETUP-GCP.md`
- `docs/design/DESIGN.md`, on the design branch

## Conventions
- **Commits:**
  - As `Mizore66 <anasqumhiyeh@gmail.com>`, signed if that key is available to you.
  - Small and reviewable, with a body that says why.
  - End each message with `Co-Authored-By:` for the AI.
- **Secrets:** never commit or log credentials, keys or tokens. No `.env` files in git.
- **Held-out split** (`-split final`): run it once per frozen config with `-confirm-final`. Never tune on it.
- **Wording:**
  - Say "precedent-supported" and "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling."
  - Banned: "guarantee", "approved", "verified", "cheapest", "proof" (outside the product name).
  - Economics are "illustrative — editable" unless sourced.
- **Proxy labels:** label proxy numbers as proxy. Record every decision as a new D entry with its numbers. Keep negative results.
- **Checks:** `make test lint` from the repo root must pass (gofmt, vet, staticcheck, tests).

## Pitfalls
- The raw corpus isn't in git. Evals and the server use the bundled facts in `backend/data/facts` with `backend/data/labels.jsonl` (eval: `-facts data/facts`).
- Directory settings take `off` to disable (`CASES_DIR=off`, `VERIFIER_CACHE_DIR=off`); an empty value means the default.
- Cloud Run deploys set `CASES_STORE=firestore` and `VERIFIER_CACHE_DIR=/tmp/verifier-cache`, because the image's `/data` isn't writable.
- `eval summary` currently hard-codes the `*-litellm.jsonl` result files.
- The heat-gun demo: `backend/testdata/heatgun_2022.json` (single country) and `heatgun_multi.json` (Thailand, Vietnam, Malaysia). The demo predict body is at `GET /v1/demos/heatgun-2022`.

## What needs doing before the freeze, in priority order
1. **GCP IAM, Vertex smoke test, official Vertex numbers.** The separate brief, `cloud-agent-gcp-vertex.md`, comes next.
2. **Demo seed and recording runbook:** one command creates the heat-gun cases, runs the plan searches with the verdict cache on, and writes a click-by-click script, so the video doesn't depend on proxy luck.
3. **Frontend build:** the checklist in `build-frontend.md`, after the #46 PR is merged. A separate frontend session owns the design; coordinate through Anas.
4. **Tuning job (#38):** only after Vertex works and the model and cost are confirmed. Paid; ask Anas before launching.

Ask Anas when a decision is his: merging, paid jobs, deploying, product wording.

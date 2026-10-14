# Decisions

Short ADR-style entries: decision, alternatives, why. Newest last.

## D1 — Build inside the existing team repo's `backend/` and `frontend/` (15 Oct)
- **Decision:** Origin Proof's Go API lives in `backend/` (Cloud Run service `backend`), the Next.js app in `frontend/` (service `frontend`). The template deploy-agent code there was removed.
- **Alternatives:** new `api/` and `web/` folders as in the build prompt; a fresh `origin-proof` repo as in BUILD-PLAN.md.
- **Why:** CI, Workload Identity Federation deploys, service accounts and IAM (frontend may invoke backend; backend has Vertex AI User) already exist for these two services. New services need IAM changes the CI deployer can't make. The human owner confirmed the template code may be overwritten. The repo's first commit (13 Oct) is after the hackathon start (12 Oct).

## D2 — Health check at `/health`, not `/healthz`
- **Why:** Cloud Run reserves paths ending in `z` and returns 404 for them.

## D3 — Two LLM providers behind `llm.Model`; LiteLLM is development-only
- **Decision:** `LLM_PROVIDER=litellm|vertex|fake`. LiteLLM (OpenAI-compatible proxy) is used now because the team has no Vertex access yet. `config` refuses `litellm` and `fake` when `K_SERVICE` is set, and the deploy workflow sets `LLM_PROVIDER=vertex`.
- **Why:** keeps the research proxy usable for development while guaranteeing the deployed service and official numbers come from Vertex. Every output records `llm_provider` and `model_id`.

## D4 — Real API names differ from the build prompt
- ADK for Go is imported as `google.golang.org/adk` (v1.8.0), not `github.com/google/adk-go`.
- Gen AI SDK: `google.golang.org/genai` v1.74.0 with `Backend: genai.BackendVertexAI`; JSON answers use `ResponseMIMEType: application/json` + `ResponseJsonSchema`; tools use `FunctionDeclaration.ParametersJsonSchema`.
- Gemini 3+ returns thought signatures on function calls that must be sent back unchanged. Vertex: `Part.ThoughtSignature` (carried in `llm.ToolCall.Signature`). LiteLLM encodes the signature into the tool-call `id`, so the ID is passed back unchanged.
- Vertex location defaults to `global` (Gemini 3.x Flash models are not served from `us-central1`, per the repo's earlier README).

## D5 — Temperature 0 and JSON schema on every call
- The research harness used temperature 0 with JSON *mode*. We use a response JSON *schema*, which constrains decoding slightly differently; the Gate B reproduction check (within ~2 points of 89.1%) covers this change.

## D6 — Country normalisation ports `norm()` with one fix
- `domain.NormalizeCountry` reproduces `norm()` from `predict.py` (including its quirks, e.g. "Bosnia And Herzegovina") so eval numbers are comparable. Exception: `norm("Türkiye")` returns "Trkiye" because the regex strips "ü"; we return "Turkey".

## D7 — Triage rule (human decision, 15 Oct)
- Fast path = engine confidence ≥ 0.95 **and** in scope. Scenario cases are **not** excluded. The matching research statistic is "confidence ≥ 0.95 covers 72.4% at 97.8%" (held-out, n=273 of 377); the routed-pipeline "65% at 98.0%" describes a different rule and must not be shown for this one.

## D8 — Duty rates: sourced for the heat gun, illustrative elsewhere
- Heat gun: China ~41.7% vs Thailand ~16.7% ad valorem (HTS 8419.89.95, classification inferred and unverified for this model; Section 301 List 2 25% is the gap). Sources, dates and caveats in `docs/DUTY-SOURCES.md` (checked 15 Oct 2026). Show the source next to any duty number.
- Other fixtures (LiDAR) keep illustrative placeholder rates; annual volume and group costs are always user inputs labelled "illustrative — editable".

## D9 — CI smoke test without calling the private backend
- The backend accepts only `frontend@`; the CI deployer can't mint a token for it. The deploy smoke test checks the backend's latest revision is Ready and serving all traffic (startup config validation makes a bad config fail here) and that the frontend returns 200. Becomes an end-to-end predict call once the frontend proxies it.

## D10 — BM25 ported exactly, including IDF over the whole corpus
- `retrieval.BM25` mirrors predict.py: tokens `[a-z]{3,}` minus a stop list, first 3,000 characters of facts, k1 1.5, b 0.75, de-duplicated query terms, ties broken by higher corpus index. IDF and average length are computed over the whole corpus (including rulings later than a case); only the candidate set is date-filtered. That leaks term statistics, not text or holdings, and is kept so precedents match the research run.
- Parity on the 377 held-out cases (opt-in `TestResearchParity`): 369 identical precedent lists, 6 same set in another order, 2 with a different 5th precedent. Cause: the labels file gained manual labels after the research run, shifting corpus statistics.

## D11 — Facts splitter port is byte-identical
- `corpus.SplitFacts` reproduces split_facts.py on all 2,790 rulings (`go run ./cmd/corpus compare`), with character offsets handled as runes.

## D12 — Router regime label (new prompt section), dev-validated
- ROUTER.md has no regime check, so router-v1 added a regime label (textile 102.21, USMCA/Part 102, pharma API, substantial transformation, unclear). v1 over-triggered on any Canada/Mexico/USMCA mention (3 of the first 35 dev cases, all decided by CBP under substantial transformation); v2 narrows the USMCA definition. Only dev data was used.
- Measured by `eval scope` on dev: false out-of-scope rate on substantial-transformation rulings, and recall on labelled Part 102 / pharma-API rulings from Jul–Dec 2025. Labels have no separate textile regime (textile rulings fall under part102_tariff_shift).

## D13 — Router runs separately from the engine in eval
- With the router in the loop a dev run took ~45 min (Flash calls are slow through the proxy). The engine run and the router labelling now run separately; `eval report -scope` joins router labels to engine rows to compute the fast path. In the product both run per request.

## D14 — Cited rulings are only rulings shown to the model
- `cited_rulings` = precedents named in the model's reasoning/decisive step/key factor, in retrieval order; ruling numbers the model mentions that it was not shown are dropped. All shown precedents are returned separately as `precedents`.

## D15 — Planner moves operation groups; families are template files
- `planner/templates/*.json` define each family's operation groups (motorised tools & small appliances; LiDAR modules as a second family). Plants carry a capability matrix. The planner enumerates single and double group moves to capable plants in the target country; final assembly is not movable.

## D16 — Facts renderer matches CBP's restatement style (changed after the first heat-gun run)
- First version rendered processing detail for every group. On the 2022 heat gun the engine then predicted Thailand (0.78), though on CBP's own 2022 facts it predicts China (0.78) and CBP held China. The renderer now lists components by country, adds processing detail only for the groups a plan moves (as CBP did in N357771), and the final assembly.
- Consequence: the heat-gun acceptance run is a demo check on a renderer adjusted after seeing that fixture, not an unbiased evaluation. Unbiased planner evaluation needs fresh rulings (finals work).

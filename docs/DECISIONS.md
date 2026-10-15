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

## D17 — Plans start from CBP's holding on the current setup when the user has one
- Even with the corrected renderer, the engine called the 2022 heat gun Thai (0.78; CBP held China; on CBP's own facts the engine says China at 0.78 — a borderline case). With the wrong starting origin the verifier saw no change and rejected every move.
- A plan request may carry `current_origin` (country + ruling). It sets the starting origin for economics and is stated to the verifier ("CBP has already ruled on variant A: origin China"), the research "A's ruling known" setting (85.5% on 76 pairs). The engine's own baseline prediction is still shown next to CBP's holding. Verifier prompt version `verifier-v1+known-a` when used; dev-evaluated in both modes.

## D18 — Gate D runs on an in-memory store; corpus deployment is an open decision
- `store.Cases` has an in-memory implementation; Firestore drops in behind the same interface once the GCP project is available (SETUP-GCP.md). Cases are lost on restart and not shared between instances until then.
- The server needs the facts corpus (12 MB derived from public CBP rulings) at startup. The deploy builds from `backend/` only and respects `.gitignore`, so the corpus is not in the image. Options for the humans: (a) upload to the GCS bucket and load at startup (needs GCP access); (b) commit the facts files under `backend/data/` (derived public-domain text; the build prompt only rules out committing the 27 MB raw texts); (c) fetch from CROSS at startup (~50 min, not viable). Recommendation: (b) for the hackathon, (a) after.
- **Decided (human, 16 Oct): (b).** `backend/data/` holds `labels.jsonl` and the 2,790 facts files (13 MB); the Dockerfile copies them to `/data` and the server's defaults (`data/labels.jsonl`, `data/facts`) resolve from `backend/` locally and from `/` in the image. `make verify-data` checks the copy.
- Verifier evidence in production comes from live CROSS search with `toDate` (VERIFIER_SEARCH=cross); eval and the demo checks use the local corpus for reproducibility.

## D19 — Duty rates are inputs, not a package
- The build plan listed a `duty` package. Rates are product- and date-specific and the demo needs one sourced product; they are carried as request inputs (`economics.duty_rates` + a source note) and the demo fixture cites `docs/DUTY-SOURCES.md`. A dated HTS lookup can be added behind an interface later.

## D20 — Pasted facts are normalised to the benchmark format; the heat-gun prediction is borderline
- Corpus facts files, and therefore every benchmarked prompt, end with one newline. The API now appends it to pasted facts. Found because the heat-gun demo (CBP's own 2022 facts text) flipped with it: without the newline Thailand 5/5 runs (0.78); with it China 4/5 (0.75–0.85), Thailand 1/5. CBP held China.
- The change restores the evaluated input format; it was not chosen per case. The heat-gun current setup stays a borderline case for the engine: always below the 0.95 fast path (routed to expert review), and the demo should present it that way. Measured 16 Oct, gemini-4-argon via LiteLLM (proxy).

## D21 — Plan display bar stays at 0.95; near misses are shown greyed (human decision, 16 Oct)
- The heat-gun "motor-fan + PCBA" move is precedent-supported on every run (supporting N347421), but the verifier's confidence lands on 0.90 or 0.95, so it was shown in 1 of 3 runs. Dev data (54 pairs) showed every precedent-supported verdict correct at ≥ 0.90 and ≥ 0.95 alike; 0.95 only cuts recall (known-A: 13/46 vs 21/46). Small negative class (8).
- Kept 0.95 as the brief requires. Rejected plans now carry `rejection_kind`; `below_confidence_bar` rows (precedent-supported, confidence below 0.95) sort right after the current setup, and the UI shows them greyed with the label "Precedent-supported — below confidence bar" and their supporting ruling.

## D22 — Accuracy options tried on dev; none adopted (16 Oct)
- Hybrid/embedding retrieval and precedent-order voting were built (kept in code behind flags: `-retrieval`, `-votes`; engine defaults unchanged) and evaluated on dev only. None beat the baseline (eval/RESULTS.md). The configuration is unchanged, so the held-out split was not re-scored.
- Remaining error types need judgment the model lacks, not better retrieval. Family playbooks, the obvious next lever, overfit in research (dev 91.9% → test 88.3%) and are not retried before the deadline.

## D23 — Scoring adjustments: verified, mechanical, applied to every run
- `eval/data/scoring_adjustments.json`: 6 dev exclusions (4 multi-scenario holdings from `eval multiscenario`; H325607, a revocation covering two hose rulings with different origins; N354982, held "the country where the blanks are worked") and 1 alternative gold (N354448, CBP held "Indonesia/Malaysia"). Found by a mechanical scan of both splits (non-country labels, A/B holdings, revocations), each verified against the ruling text. Held-out: one flag (H360070), label correct; no change.
- Six labels the learnability judge called wrong (N353581, N354156, N356481, N350517, H331375, N348376-manual) were checked: CBP's holdings match the labels. The judge, being the same model, disagreed with CBP — not a label problem.
- `NormalizeCountry` keeps "A/B" answers as "A/B" (was glued into one word). Old result rows keep their stored prediction, so N354448 stays wrong in existing runs.
- Effect: dev 90.0% → 90.9% (421/463); held-out unchanged 89.9%.

## D24 — Clarify review: useful questions and conflicts, not an error detector
- `clarify` (clarify-v1, one extra call ~$0.009, only for in-scope cases below 0.95) reports whether an earlier ruling is on point, conflicting comparable rulings with the deciding difference, and up to 3 questions. Triage routes "no precedent on point + questions" to `needs_info`; `POST /v1/cases/{id}/answers` appends answers to the facts and re-analyses.
- Dev (127 below-threshold cases): the flags do **not** separate errors — engine error rate 26% with no precedent on point vs 22% with one; 24% where a conflict was found; needs_info 28% vs needs_review 21% (overlapping CIs). Reviewing the questions by hand, they target the facts CBP relied on (helmet: is the shell moulded to final shape; seamless pipe: are green tubes at final dimensions; filters: does the paper arrive in rolls or cut). Whether answers improve accuracy cannot be measured: the benchmark has no requester answers, and simulating them from ruling text would leak the holding.
- So it ships as broker-facing help, not as a claimed accuracy gain. No held-out run: clarify never changes the predicted origin, and its routing showed no error signal on dev.

## D25 — Cases persist as JSON files; Firestore before deploy (15 Oct)
- `GET /v1/cases` (Cases home page) needs cases to survive a restart. `store.File` writes one JSON file per case to `CASES_DIR` (default `../data/cases`, gitignored), atomically (temp file + rename); IDs are checked against `^[A-Za-z0-9_-]{1,64}$` so a crafted ID cannot escape the directory. `CASES_DIR=` (empty) keeps the in-memory store.
- Every write sets `updated_at`; export also records `exported_at`, which drives `step = export`. The list is ordered by `updated_at` descending.
- On Cloud Run the container disk is per-instance and lost on restart, so this is local persistence only. Before deploy, add a Firestore implementation of the same `store.Cases` interface (blocked on GCP setup).

## D26 — Follow-up answers do not settle split votes toward CBP; "unknown" answers stay out of the facts (15 Oct)
- On the 14 dev cases where the engine's 5 votes split between two origins, simulated-requester answers to the clarify questions moved the majority away from CBP more often than toward it (57% → 36% agreement; 1 fixed / 4 broken with "Not stated." lines, 0 / 3 with answered questions only). Small sample, but negative in both variants (eval/RESULTS.md).
- Mechanism: the engine treats "Not stated." and hedged answers as evidence against the processing country, and gives extra weight to facts restated in Q&A form. The simulated requester is also a weak stand-in for a real one: for NY rulings it had no facts beyond the input.
- Product changes: `POST /v1/cases/{id}/answers` keeps "unknown"/"not stated" answers on the case but does not add them to the facts, and refuses a set with no factual answer. Clarify stays broker-facing help (D24); a re-analysis after answers is not claimed to be more CBP-like. Not adopted: re-voting after answers as a confidence signal.
- Follow-up (answers-v2, adopted): `clarify.AppendAnswers` removes hedge sentences from answers and adds a header telling the engine the answers weigh the same as the facts and that unaddressed points are not evidence for any country. On the same 14 cases: 7/14 agree with CBP (0 fixed, 1 broken, vs 1/4 and 0/3 before). The remaining break is a judgment error on factual answers, not a format effect. The engine system prompt (engine-v1) is unchanged, so cases without answers are unaffected. Responses record `prompt_versions.answers`.
- Not tested: answers from real requesters with new facts, which the benchmark cannot supply.

## D27 — Written CBP rules for the confident errors: no gain, not adopted (15 Oct)
- The 12 high-confidence dev errors (96.4% accuracy at ≥ 0.95) split into about 5 real judgment errors (chemical change, preforms, essential character), 4 cases where CBP departed from a closely matching earlier ruling, and 3 scoring or label issues.
- rules-v1 (three short statements of CBP practice, appended to engine-v1) moved dev from 90.9% to 91.1%: 10 fixed, 9 broken, p = 1.0. The rules fixed 2 of the 5 targets and broke cases where CBP weighed other facts. Kept behind the eval flag `-rules`; the product prompt stays engine-v1. No held-out run.
- Conclusion with D22–D26: prompt-level changes do not move accuracy; the remaining confident errors need either examples of CBP's reasoning in training (fine-tuning on Vertex, finals) or human review, which the confidence bar already routes.


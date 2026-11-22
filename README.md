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

## Who uses it and who benefits

Five roles meet in one production-move decision. These are the roles Unde is designed around. It has no customers or pilots yet, and none of these is a named firm.

| Role | Who | What they get |
|---|---|---|
| **Uses it** | The trade-compliance or industrialisation lead at an ASEAN contract manufacturer | The origin CBP would likely assign, with the earlier rulings behind it, before work moves; moves that earlier rulings support; a draft ruling request |
| **Pays for it** | The COO or VP of supply chain at that manufacturer, or the U.S. brand that sources from it | A check before capital moves: the duty effect and added cost of each move side by side, with the moves no ruling supports set apart |
| **Works with it** | Licensed customs brokers and trade lawyers | A draft with the facts laid out and the cited rulings linked. They still confirm the classification and file the request; Unde does not replace them |
| **Benefits** | The U.S. importer of record | It carries the duty and the penalty risk if the origin is wrong. It gets an earlier warning when a planned move would leave the product Chinese, and a record of which rulings were read |
| **Benefits** | The ASEAN plant that would take on the work | A clearer case for which operations to bring in, and early notice when a smaller move would not change the origin |

What one such decision looks like in numbers (time, cost and duty, each sourced in [docs/NUMBERS.md](docs/NUMBERS.md#impact-story-landing-fig4-illustrative)) is fig.4 on the [landing page](https://undelabs.dev), an illustrative composite labelled as such.

## Why it matters

- **Duty:** for our demo heat gun, China vs Thailand origin is about a **25-point** difference in duty. Unde uses live U.S. rates, including Section 301 duties, for every HTS line.
- **Time:** CBP estimates **20 hours** of preparation per ruling request, and recent origin rulings took a median of **21 days**. Unde gives a first, non-binding answer: one origin prediction takes 4.6 s (median; 5.7 s for 95%), and origin, plan and draft together take about a minute (estimated, not measured). The binding answer still comes from CBP.
- **Risk:** moving production and still getting China origin is the costly mistake. Every plan is checked against past rulings, including ones where a similar change did *not* work.

## Other features

- **Follow-up questions** when the prediction is unsure, then a re-check with your answers
- **Side-by-side scenarios** with yearly duty, savings and added cost
- **Compare target countries** (e.g. Thailand vs Vietnam), ranked by the best plan's tier (clear margin first, then at the bar), then net savings; on a Part 102 case by tier, then the best plan's 102.20 rule check (shifts, then undetermined, then another country), then net savings (D500)
- **Ruling watch:** re-check saved cases against rulings CBP issued later, with a box on the case when a ruling it relies on is revoked or modified
- **Ask the case:** an agent that explains the prediction and runs what-ifs
- **API** for ERP and sourcing tools ([docs/api/openapi.yaml](docs/api/openapi.yaml))
- **MCP server** so Gemini CLI, Google ADK agents and Claude can use Unde as tools, with a runnable ADK agent in `examples/adk-agent` (see below)

## Checks beside the model

Gemini gives the answer. These parts are code, and each says when it cannot decide. Every figure below is in [docs/NUMBERS.md](docs/NUMBERS.md) and recomputed by `make reproduce`.

- **The 19 CFR 102.20 rule check (built).** For goods from Canada or Mexico, the origin rule is a tariff shift: each foreign part must change HTS heading. A deterministic check reads the encoded rules for 17 HTS chapters and says "undetermined" rather than guess. It never changes the answer. With CBP's own codes from the full ruling text it decides 8 of 72 development and 4 of 32 held-out rulings, all as CBP held. From the facts the model reads it decides almost none (0 of 72 and 1 of 45), because they rarely give codes. Run over every combination of each part's suggested codes, 15 of 16 development and 13 of 14 held-out decisions give the same answer. Five of the 17 chapters (15, 21, 40, 71, 95) were picked from a pool that wrongly included held-out rulings; they stay encoded, and without them the held-out full-text figure stays 4 of 32 and the last becomes 11 of 12 (D560, [#619](https://github.com/kendramoss/wassup-hackathon-2026/issues/619)). Recorded example: [/examples/rule-check](https://undelabs.dev/examples/rule-check). ([#541](https://github.com/kendramoss/wassup-hackathon-2026/pull/541), [#544](https://github.com/kendramoss/wassup-hackathon-2026/pull/544), [#588](https://github.com/kendramoss/wassup-hackathon-2026/pull/588), [#594](https://github.com/kendramoss/wassup-hackathon-2026/pull/594); D360-D363, D430, D460, D461, D560)
- **HTS suggestions for each part (built).** The parts table suggests up to three codes per part, marked as from an earlier CBP ruling or the model's own reading, and fills one in only when you accept it. A plausibility check flags a code in an unlikely chapter; it never removes one. Where CBP states a part's code, the top suggestion matched it for 20 of 27 development parts (2 of 3 held out). With these codes and CBP's code for the good, the rule check decides 16 of 69 development rulings (16 agree with CBP) and 14 of 29 held out (13 agree). The codes are unverified: check them with a broker. ([#570](https://github.com/kendramoss/wassup-hackathon-2026/pull/570), [#599](https://github.com/kendramoss/wassup-hackathon-2026/pull/599); D380-D382, D471)
- **The rule check on each plan move (built).** Each move gets a chip: shifts under 102.20, does not shift, or undetermined and why. When an earlier ruling supports a move but the rule gives another country, the move is flagged for a broker. On a substantial-transformation case it is display only. On a case routed to the Part 102 rules it also orders the shown plans within each tier (moves it says shift first, then undetermined, then another country), and the broker draft quotes it (D500, D502). On the committed development plan searches with suggested codes: 25 of 57 moves decided; 21 of 23 agree with the precedent check; 2 flagged (N354220: the good is still finished in China). Three Part 102 plan searches on development rulings were scored: on the actuator (N348465) the rule picked the 6 moves that bring the motor to Canada, CBP's own reason for holding China, and the one shown plan is among them (D501, D502).
- **The Part 102 engine in plan searches (built).** On a case routed to the Part 102 rules, the Part 102 engine predicts the current setup and every move, so plans and the rule check reason under one test. Re-run on the same three development searches, its prediction agrees with the rule on 13 of 14 moves (the general engine on 9); one error is left, on the compressors (D540, D541). Quoting the 102.20 rule text to the Part 102 engine was tested on development under a rule written before the run and did not pass (50 of 72 correct against 53); it was not adopted, and no held-out ruling was read (D550, D551). Recorded examples: [/examples/part102-plan](https://undelabs.dev/examples/part102-plan) (the actuator search with the Part 102 engine) and [/examples/plan-rule-check](https://undelabs.dev/examples/plan-rule-check). ([#590](https://github.com/kendramoss/wassup-hackathon-2026/pull/590), [#613](https://github.com/kendramoss/wassup-hackathon-2026/pull/613), [#623](https://github.com/kendramoss/wassup-hackathon-2026/pull/623), [#625](https://github.com/kendramoss/wassup-hackathon-2026/pull/625); D440-D442, D500-D502, D540, D541, D550, D551)
- **Revoked rulings (built).** A map of 143 rulings CBP revoked or modified, from the corpus and 28 Customs Bulletins, stops Unde citing a ruling CBP had already changed at the case date. A saved case shows a "Re-check recommended" box when CBP later changes a ruling it relies on. On the committed results this fires rarely: 2 of 13,107 result rows, and none of the demo cases. Each alert says whether the change bears on what the case relies on that ruling for: it compares the origin the changed ruling held, and the one the changing ruling holds, with the case's answer for a cited ruling, with a shown plan's target for the plan's supporting ruling, and with CBP's holding on the current setup for that ruling, and says when a holding is not known (D531, D561). Nobody is notified by email yet. Recorded example: [/examples/revocation-alert](https://undelabs.dev/examples/revocation-alert). ([#506](https://github.com/kendramoss/wassup-hackathon-2026/pull/506), [#579](https://github.com/kendramoss/wassup-hackathon-2026/pull/579), [#598](https://github.com/kendramoss/wassup-hackathon-2026/pull/598); D320, D370, D400, D401, D480, D481, D490)
- **A Google ADK agent, evaluated (built, measured once per product).** [`examples/adk-agent`](examples/adk-agent/) runs a whole case over MCP. On 12 development products, with rules written before the run, 10 of 12 runs passed every check, for $0.47 of model calls. Both misses are reported, not rescored ([eval/agent/SUMMARY.md](eval/agent/SUMMARY.md)). It measures tool use, not origin accuracy. The median run took 313 s. A second evaluation of the changed agent (checks written first, D510): all 8 runs the live server's anonymous budget did not touch passed every check. The budget cut the other 4 short; scored as the rules say, 9 of 12 pass, one of them (a09) because the agent reported the refused plan call and stopped. The planned second run, which would measure repeatability, has not run yet (D511). ([#574](https://github.com/kendramoss/wassup-hackathon-2026/pull/574), [#592](https://github.com/kendramoss/wassup-hackathon-2026/pull/592), [#599](https://github.com/kendramoss/wassup-hackathon-2026/pull/599); D390, D450, D451, D470-D472, D510, D511)

## Use Unde from Claude, Gemini or ADK

Unde is also an MCP server: an agent can predict origin, search production moves, look up rulings and duty rates, draft a ruling request and send it to a broker for review as tools. Endpoint: `https://undelabs.dev/mcp`. Full setup, tool list, limits and privacy: [docs/MCP.md](docs/MCP.md).

```bash
# Gemini CLI (anonymous tier; add --header "Authorization: Bearer op_..." with an API key)
gemini mcp add --transport http unde https://undelabs.dev/mcp

# Claude Code, with an API key (keys are issued by the Unde admin)
claude mcp add --transport http unde https://undelabs.dev/mcp --header "Authorization: Bearer op_..."
```

A Google ADK agent connects with `McpToolset` and `StreamableHTTPConnectionParams(url="https://undelabs.dev/mcp")`; the snippet is in [docs/MCP.md](docs/MCP.md#google-adk-agent-python). [`examples/adk-agent`](examples/adk-agent/) is a full ADK trade-compliance agent: from a product description it extracts the parts, predicts the origin with cited rulings, plans production moves to a target country and drafts the broker request. It was evaluated on 12 development products against the live server, with the rules written first: 10 of 12 runs passed every check, and the two misses are kept ([eval/agent/SUMMARY.md](eval/agent/SUMMARY.md), D450, D451). After the D470 fixes, a second evaluation with stricter checks passed all 8 runs the live server's anonymous per-client budget did not touch; the budget cut the other four, and 9 of 12 pass as scored. Repeatability is not measured yet (D510, D511). The cases are also in ADK's own evaluation format, ready for `adk eval` ([eval/agent/adk](eval/agent/adk/)). An earlier full run is in the [transcript](docs/mcp-transcripts/adk-full-flow.md). In Claude.ai or Claude Desktop, add a custom connector with that URL and no sign-in (anonymous tier). For a local stdio process, run `node mcp/stdio/index.js` with `UNDE_API_KEY` set. Results are predictions, not rulings: confirm with a licensed customs broker.

## How a business could use it

1. **Start with the web app.** Upload a bill of materials, review the origin and plans, and send the draft to your broker.
2. **Make it a step before any production move.** Check the plan before approving a supplier change, a new plant or a country shift.
3. **Connect your systems.** Send bills of materials from your ERP through the API.
4. **Keep watching.** Re-check saved cases on a schedule as CBP issues new rulings.
5. **Keep the broker in the loop.** Send the draft to your customs broker as a review link that expires and that you can revoke. The broker opens it without an account and approves it, asks for changes with a comment, or marks it to file as a binding ruling request; you see the answer on the case. Unde speeds up the decision; a broker or a CBP ruling still confirms it.

## Does it work?

Unde is not a trained or fine-tuned model: it prompts Gemini with earlier CBP rulings it retrieves, and its prompts, retrieval settings and calibration map were chosen on development rulings (the page names the confidence bars that were not). [How it's built and tested](https://undelabs.dev/method) answers, in order, what the model is, what data it sees, how the test sets are kept clean (and where they are not), what we tried that did not work, and how stable the results are, each linked to its dated entry in [docs/DECISIONS.md](docs/DECISIONS.md).

Scored once on a **377-ruling temporal test split** of cleaned, single-origin 2026 substantial-transformation rulings. Prompts were not tuned on it; earlier split rulings may serve as precedents, BM25 statistics use the whole corpus, and pretraining exposure cannot be excluded (a case's own ruling and later rulings are never retrieved as precedent):

| Method | Accuracy |
|---|---|
| Best simple rule we recomputed (first country named in the facts; no model) | 63.4% (239 of 377) |
| Most-mentioned country (recomputed; the research figure was 60.5%, unverified) | 60.7% (229 of 377) |
| **Unde** (Gemini 4 Argon on Vertex AI + past rulings), all 377 cases | **90.2%** (340 of 377) |
| Unde, confident cases only: calibrated confidence at least 0.90 (the same cases as a stated 0.92) and in scope, 79.6% of cases | 96.0% (288 of 300) |

The 96.0% covers a subset the model's own confidence selected, so it is not comparable with the all-case baselines, and a gap in percentage points is not evidence that the confidence scores are calibrated. See [docs/claims-reconciliation.md](docs/claims-reconciliation.md). The baselines are recomputed by `go run ./cmd/eval baselines` from the committed facts ([D155](docs/DECISIONS.md)).

**A second test set, built after the first was spent.** 83 CBP rulings published 28 Sep to 13 Nov 2026, found and labelled with committed tools and frozen before anything was scored on it. The same engine, scored once: **92.8%** (77 of 83, interval 85.1–96.6); both test sets together 90.7% (417 of 460). These new runs went through an OpenAI-compatible gateway to the same Gemini model, not Vertex AI. Gateway and Vertex AI agree on the development set (same engine and model, exact McNemar p = 0.58, D151); these runs were not repeated on Vertex AI. The new set is small and easier for simple rules, so it shows the engine holds up after its cutoff, not that it improved. Newer Gemini models (4 Carbon, 3.1 Pro preview) and a prompt change were tried on the development set and none beat Argon ([docs/EVALUATION.md](docs/EVALUATION.md#6-the-second-held-out-split-21-nov-2026)).

On the two demo products, the plan Unde shows matches what really happened:

- **Heat gun:** Unde's plan (move the motor-fan assembly, circuit board and switch to Thailand) matches CBP's later 2026 ruling holding Thai origin.
- **LED tub light:** Unde's plan (make the LED circuit board in Thailand) is exactly CBP's next ruling.

In the historical D86 proxy pool, 182 verdicts cleared the precedent check's 0.92 bar and none was wrong. Those are proxy runs, 159 on 54 development pairs and 23 on 21 pairs from 2026 that are not a clean held-out test, and repeated runs of the same pairs inflate the count. Counted once per pair, 17 real-change pairs were shown in every run and no no-change pair was ever shown ([D86](docs/DECISIONS.md)). In later post-cutoff gateway scoring, the check was right 9 of 9 times with the starting origin and 5 of 6 without it; one wrong plan is recorded in [eval/RESULTS.md](eval/RESULTS.md). These are proxy and gateway runs, not a guarantee. The bar is strict: about 30% of real changes clear it, and the rest go to a broker with their closest supporting ruling. Full results, confidence intervals and what didn't work are in [docs/DETAILS.md](docs/DETAILS.md#results). How the test set is built, what we trade off and how we benchmark: [docs/EVALUATION.md](docs/EVALUATION.md).

## Reproduce our numbers

The headline evaluation counts above are recomputed from committed result rows, with no model calls and no credentials. Other figures are labelled measured, reported/unverified, external, illustrative, or one-run demo:

```bash
make reproduce
# which runs:
make verify-data                                           # data files match eval/SHA256SUMS
cd backend && go run ./cmd/claimscheck && cd ..           # figure ledger and provenance, from committed artifacts
python3 -I -B eval/charts/build_evaluation.py . --check           # calibration, accuracy-coverage curve, split funnel
python3 -I -B eval/charts/build_trust_data.py . --check           # failure cases, corpus counts
python3 -I -B eval/charts/build_accuracy.py . --check             # model sweep, second test set, calibration layer
python3 -I -B eval/charts/build_repeatability.py . --check        # run-to-run repeatability on dev
python3 -I -B eval/charts/build_repeatability_heldout.py . --check # two held-out runs, answers that changed
python3 -I -B eval/planner/summarize.py . --check                 # planner summaries
python3 -I -B eval/charts/build_adjudicator.py . --check          # adjudicator metrics
python3 -I -B eval/planner/summarize_addendum.py . --check        # planner addendum
```

It needs Go 1.27, Python 3.8+, `make` and `shasum`; the Python scripts use only the standard library. Each check fails if a published figure has drifted from its source. `claimscheck` reconciles a frozen figure ledger and provenance; it does not rerun the model or recompute reported confidence intervals, costs and latencies. See [docs/claims-reconciliation.md](docs/claims-reconciliation.md).

**What it gets wrong.** All 37 held-out misses are listed by ruling, with links to CBP, on the [Accuracy page](https://undelabs.dev/accuracy#misses). It is weakest where CBP held China (77.2%, 44 of 57, interval 64.8–86.2), which is the costly direction for a move out of China. The patterns are in the [model card](docs/MODEL-CARD.md#6-known-failure-modes); the corpus, its sources and its biases are in the [datasheet](docs/DATASHEET.md).

## Limitations

- **The planner is still the least tested part.** Its precedent check now has a clean test on **18 frozen CBP flip pairs** from rulings published after 25 Sep 2026: 12 real origin changes and 6 no-change controls (D170 + D192), each frozen before scoring and read by one person, and scored through the gateway in three runs (D353, D354) that showed the same moves when the starting origin was given. With CBP's starting origin given, 9 of 9 moves shown were real changes (interval 70.1–100) and 9 of 12 real changes were shown (75.0%, interval 46.8–91.1); none of the 6 controls was shown (0 of 6, interval 0–39.0). Denominators matter: precision is over shown moves (9), recall over real-change pairs (12), not all 18. It tests the check, not plan quality. Recompute the frozen summaries with `python3 -I -B eval/planner/summarize.py . --check`, `python3 -I -B eval/planner/summarize_addendum.py . --check` and `python3 -I -B eval/planner/summarize_repeats.py . --check`. Gateway and Vertex AI agree on the development set (same engine and model, exact McNemar p = 0.58, D151); these runs were not repeated on Vertex AI. Without the starting origin the check showed one wrong plan. The plan search reached CBP's move in both real changes it could express, but can rank a cheaper move CBP never ruled on above it ([docs/EVALUATION.md](docs/EVALUATION.md#7-planner-21-nov-2026), D170, D192).
- **Benchmark claims are reconciled, with limits.** Every headline number is recomputed from committed files by `cd backend && go run ./cmd/claimscheck`; the research 60.5% baseline cannot be reproduced from this repository (our recomputation gives 60.7%; D155). See [docs/claims-reconciliation.md](docs/claims-reconciliation.md).
- **U.S. origin only.** The tool answers U.S. CBP country-of-origin questions under four regimes (substantial transformation, textile 19 CFR 102.21, Part 102 marking origin, pharmaceutical active ingredient). EU Binding Origin Information, USMCA regional value content and preferential-origin claims are not supported, and nothing detects or blocks a question about another country's rules. See [docs/scope-and-jurisdiction.md](docs/scope-and-jurisdiction.md).
- **Plans count duty and the costs you enter only.** An optional operating-cost mode adds labour, logistics, one-time capex and retooling spread over a horizon you choose, plus lead-time and minimum-order checks. It prices only what you typed, lists what it leaves out (capacity, time value, tax, yield, subsidies) and never calls a plan the cheapest or best possible; see [docs/modelled-operational-costs.md](docs/modelled-operational-costs.md).
- **Plans need one of 20 product families.** Eleven were added from CBP precedent (D138, D212): electronics built on a PCBA, vehicle parts, hand tools, cables and harnesses, medical devices, power supplies, furniture, displays and cameras, water filters, gas grills and electric motors, pumps and compressors, beside motorised tools, LiDAR modules and LED lights. Six more were written from Part 102 marking rulings, for goods common on Part 102 cases (D580): food and drink preparations, metal products, plywood and wood flooring, plastic articles, electrical control panels and machinery parts. On the 2026 held-out rulings, 59.4% match a family with a template with the first 14 (57.0% with 13, 4.2% with the first three); the six were not scored on that set. On the Part 102 development rulings 68.1% match with 20 against 27.8% with 14, and of the 18 frozen held-out Part 102 rulings 8 match against 6 (D581). A match means a template exists, not that a plan will be found. Other products get an origin but no plan. Plan quality is checked, not evaluated: on 23 more products from development and train rulings, 15 got a precedent-supported plan and 8 went to a broker; the ruling cited by a shown plan, read by hand, supports the move in 9 of 15 (D171, [eval/RESULTS.md](eval/RESULTS.md)).
- **Results vary a little run to run.** Three fresh runs of the same engine on the 463 development rulings scored 91.8%, 91.1% and 91.1% (mean 91.3%); 4 of 463 rulings (0.9%) got a different country in one run than in another (D190, `eval/results/repeatability-dev-litellm.json`). Those runs went through the gateway. On held-out rulings, two more runs scored 89.9% and 90.2% on the 377 (official 90.2%; 7 rulings changed answer) and 92.8% and 95.2% on the 83 later rulings (official 92.8%; 2 changed), pre-registered in D240 and reported in D241 (`eval/results/repeatability-heldout-litellm.json`). A fixed seed and a verdict cache keep repeat searches stable.
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

You need Go 1.27, `make` and Node 24 for the web app (Node 26 also works: the script tests transpile their TypeScript through the repo's own `typescript` dependency, so no removed `--experimental-transform-types` flag is needed). To reproduce the published numbers you also need Python 3.8+ and `shasum`, and an LF checkout (`.gitattributes` sets this; on Windows set `core.autocrlf=input` before cloning). Run `make doctor` first: it checks every prerequisite and the line endings and prints exactly what is missing. The public-CBP-rulings corpus is already committed and bundled in the image, so you can start without fetching it (fetching is only needed to rebuild the corpus, ~50 minutes).

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
| `docs/` | [DETAILS.md](docs/DETAILS.md) (full write-up), [EVALUATION.md](docs/EVALUATION.md), [MODEL-CARD.md](docs/MODEL-CARD.md), [DATASHEET.md](docs/DATASHEET.md), [ROADMAP.md](docs/ROADMAP.md), [DECISIONS.md](docs/DECISIONS.md), [ARCHITECTURE.md](docs/ARCHITECTURE.md), [NUMBERS.md](docs/NUMBERS.md), [api/openapi.yaml](docs/api/openapi.yaml) |

Data: CBP rulings from [rulings.cbp.gov](https://rulings.cbp.gov) (U.S. government works, public domain).

Licence: the project source is [MIT](LICENSE); bundled third-party assets and their licences are listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

# Recording the demo

Video: 3:00. Judges first. Every number on screen must trace to `docs/NUMBERS.md`, and model outputs shown are what the system produced on the recording day. Anything not measured is said as not measured.

## Video timeline (3:00)

There is no deck, so the video carries every judging line. The official weights are Technical Merit & Gen AI 40%, Problem Alignment & Impact 25%, Innovation & Creativity 25%, and UX & Solution Design 10%. The main story is the production heat-gun case in section 3; the newer capabilities (photo import, 28 product families, the Claude connector) follow it, and the video closes on trust: the misses, and how to recompute the headline counts.

The story follows one decision through the five roles on the landing page ("Who uses it, and who benefits"). They are roles UndeLabs is designed around, not customers: it has none yet.

### Plan screen: one canonical instruction, two branches

The landing's "check 0.96" is a **recorded 18 Oct 2026 production run**, committed in section 6 below. As of 21 Nov 2026 the pinned case (`case_878d76540b91`, updated 18 Oct) shows the same plan at 0.96; earlier pinned seeds (section 4) showed no plan above the bar, so check before the talk.

- **If the Plan screen shows a plan:** "This is the recorded 18 Oct run: motor-fan assembly, PCBA, switch, cord and minor parts in Thailand, check 0.96 on N345938, $285k a year on illustrative economics. The bar held the motor-fan-only move (0.90) back."
- **If it shows no plan above the bar (an earlier pinned seed):** "No move cleared the 0.92 bar in this pinned run. The closest below-bar move is PCBA in Thailand at 0.75 on N345938. The landing shows a separate recorded 18 Oct run that did clear it at 0.96; the bar is deliberately conservative, so a below-bar result is not a failed search."

Check which branch applies with `GET /v1/cases` (pinned `case_878d76540b91`) before the talk. The landing copy labels the 0.96 as recorded and says the pinned case shows the same plan; change that copy if the case is re-seeded.

### Which demo cards are safe to click (#412)

Only the **five pinned cards** (heat gun, LiDAR, both dryer latches, LED tub light) have a seeded case on this site; open those. The newer-family cards under "More demos · not run on this site yet" are **unpinned**: loading one starts a live case and model run. During the talk, do not click them — point at their visible "Development run · not run on this site" chip, then open the linked committed JSON record instead. `GET /v1/demos` marks a seeded card with `pinned_case_id`; `GET /v1/cases` marks its row with `demo_id`. Check both before the talk; a card without those fields is a live run.

| Time | Topic (judging line) | Screen | Say |
|---|---|---|---|
| 0:00–0:15 | **Problem** (Impact) | Landing page, hero | "When a factory moves out of China, US Customs decides where the product is *from*. For this heat gun, that's about 41.7% duty from China against 16.7% from Thailand." |
| 0:15–0:30 | **Who, and why it's hard** (Impact) | Landing page, "Who uses it, and who benefits" | "The compliance lead at a Thai contract manufacturer asks the question. The COO pays for the move, a customs broker files the ruling request, and the U.S. importer carries the duty risk. Each ruling request takes about 20 hours to prepare, and recent origin rulings took a median of 21 days. UndeLabs answers first. *Unde* is Latin for 'from where?'." |
| 0:30–0:50 | **Predict** (Gen AI) | "Try the heat-gun demo" opens the tour on Plan (D290); click "See the origin step first" in the tour bar → Origin | "Gemini reads CBP's own facts with the most similar earlier rulings. China, calibrated 0.60 (the model stated 0.85): under our 0.90 auto-accept bar, so it asks three questions instead of guessing." Open the cited rulings. |
| 0:50–1:15 | **Plan and agent check** (Innovation, Gen AI) | Plan: shown plan → "How the check found this" → closest below-bar row | "It tries moving each group of work to the Thai plant. An ADK agent then searches real CBP rulings for and against each move. Use the two-branch instruction above: quote 0.96 only if the screen shows the recorded 18 Oct plan; if the pinned case shows no plan above the bar, say so and name its closest below-bar move." |
| 1:15–1:25 | **Backtest** (Innovation, Impact) | Backtest panel (open by default; tour step 2) | "The manufacturer really made this change. In 2026, CBP ruled Thailand, N357771. Six parts switched from China to Thailand, and the plan moves all six. It also moves the switch, wires and fasteners, which the real change kept in China, so a smaller move may have been enough. This is not a blind test: we added three-group moves after seeing this change." (This is the stored production plan, made before the template split of D330; it stays until an owner re-pins the demo.) |
| 1:25–1:35 | **Hand-off** (UX, Impact) | Draft for broker (it opens on the shown plan) | "One click: a draft ruling request for the broker to check and file. Precedent-supported, not a ruling." |
| 1:35–1:45 | **Photo import** (Gen AI, UX) | New case → Import BOM → click "Photo (synthetic)" (it loads in place) → Parts and plants | "A parts list can come in as a photo or a scan. Gemini reads it into a draft table, and the user checks every row before anything runs. On 20 synthetic images it read 151 of 170 part names, with no wrong country; real photos and scans are not measured yet, so every row is treated as unverified." |
| 1:45–1:55 | **28 product families** (Impact, Innovation) | Cases → "More demos" → gas grill → Origin | "Plans now cover 28 product families, from heat guns to cables, chargers, pumps, hand tools, gas grills, wood flooring, tissue paper and chips. This gas grill is CBP's own case, N335085: CBP held China." Cut (visible "⏩") to the result the site gives on the day; say it is a live run. |
| 1:55–2:05 | **Use it from Claude** (Technical merit, UX) | Terminal: `claude mcp add --transport http unde https://undelabs.dev/mcp`, then ask Claude | "The same tools work from Claude over MCP: 14 tools, one line to add. Claude calls predict_origin and plan_production_moves, with the same limits as the API." |
| 2:05–2:25 | **Does it work** (Technical merit) | Accuracy | "On 377 CBP rulings not used to tune it: 90.2%, against 63.4% for the best simple rule we recomputed. It auto-accepts 79.6% of cases at 96.0%. In the historical D86 proxy pool, 182 shown verdicts (17 distinct product pairs) cleared the bar and none was wrong; in later post-cutoff gateway scoring it was right 9 of 9 with the starting origin and 5 of 6 without. These are proxy and gateway runs, not a guarantee." |
| 2:25–2:45 | **Trust** (Technical merit) | Accuracy → "Known failure cases" → "Reproduce these numbers" | "Every held-out miss is listed by ruling: all 37 of the 377, 12 of them auto-accepted. And the headline evaluation counts are recomputed from committed rows with one command, `make reproduce`: no model calls, no account. Other figures are labelled reported, external, illustrative or one-run demo." |
| 2:45–2:55 | **How it's built** (Gen AI, technical merit) | Accuracy → "How it's built" | "Next.js and Go on Cloud Run, Gemini 4 Argon and 3.8 Flash on Vertex AI, an ADK agent for the precedent check, and cases in Firestore." |
| 2:55–3:00 | **Close** | Landing page | "Plan the production change before you build it. UndeLabs." |

Every judging line is covered:
- Problem & Impact: 0:00–0:30 (the problem and the roles), 1:15, 1:25, 1:45
- Gen AI: 0:30, 0:50, 1:35, 2:45
- Innovation: 0:50–1:25, 1:45
- Technical merit: 1:55–2:55
- UX: the whole flow, plus 1:25, 1:35 and 1:55

Rehearse to 2:55. If a screen loads slowly, cut with a visible "⏩"; never splice in another run.

**What to check on the recording day for the new beats:**
- Photo import: the sample is `frontend/public/samples/heatgun-bom-photo.png`, a synthetic, tilted rendering of the heat-gun BOM (`eval/media/images/heatgun-photo.png`). It is not a real photo; say "a sample image". Field accuracy on images is measured only on 20 synthetic images (docs/NUMBERS.md, "Image and scan import"); say "on synthetic images, illustrative" with the counts (151 of 170 part names, no wrong country), as the Accuracy page does under "Import from a photo or scan".
- Gas grill: the demo is `gas-grill-2023` (backend/internal/demos/fixtures). Unless it has been seeded as a pinned case, loading it runs live on production, so the result is that day's run. A development check through an OpenAI-compatible LiteLLM gateway to Gemini predicted China 0.95 and showed one plan at 0.96 on N333420 (eval/results/families-plan-gas-grill-2023-litellm.json). That run is a record, not the production result; say only what the screen shows.
- Claude connector: run it from a clean Claude Code session. Without an API key it uses the anonymous tier; `predict_origin` returns a `case_access_key` that the next tool calls need.

## 1. Prepare (the day before, never during the talk)

Talk day uses the committed cases and caches; **do not seed on stage** (#417). Seeding is a separate, credentialed operation that can take far longer than the old "3–5 min" note claimed.

1. Start the backend on Vertex, the only model provider (D49). Local development needs Application Default Credentials (`gcloud auth application-default login`).
   ```bash
   make run   # GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 GOOGLE_CLOUD_LOCATION=global
   ```
   Keep the verdict cache on (the default: `VERIFIER_CACHE_DIR=../data/verifier-cache` locally; on Cloud Run it is the Firestore collection `verifier_cache`, shared by every instance, D50). Engine predictions are cached beside it (`engine_cache`, D54), so a replayed case and its plan search make no model calls; delete both collections (or both directories locally) to start fresh. Cases are saved under `data/cases`.
2. **Read-only pre-flight first** (no token, no model call): confirm the pinned cases are present and the committed production record says the repeat search agreed.
   ```bash
   curl -fsS https://undelabs.dev/api/v1/cases | python3 -c 'import json,sys; d=json.load(sys.stdin); print([c["id"] for c in d["cases"]])'
   grep -F 'repeat plan search: verdicts identical=true' eval/results/demo-seed-vertex-prod.json
   ```
   `eval/results/demo-seed-vertex-prod.json` is the committed 17–18 Oct production record; `eval/results/demo-seed.json` is **not committed** and only appears if you run the seed locally. Write down what the plan screens show before the talk.
3. Seeding is optional and needs credentials. `make run` and `make demo-seed` share a local admin token, so the seed is not rate-limited and pins the demo cases (listed first; visitors cannot delete them, D45). Against Cloud Run, set `ADMIN_TOKEN` to the service's token and `ID_TOKEN=$(gcloud auth print-identity-token)`, then `go run ./cmd/demo seed -api <URL>`. The seed now **stops before any model call** if `ADMIN_TOKEN` is missing, unless `-allow-unpinned` is passed deliberately; without it `pin()` used to be a silent no-op and leave the cases unprotected (#417).
   ```bash
   make demo-seed   # writes the untracked eval/results/demo-seed.json; remove it before a clean-checkout check
   ```
   It creates a case for each demo, runs the plan searches, and fills the heat gun's scenarios, ruling watch, one ask and the multi-country comparison. It then repeats the heat-gun plan search and prints `verdicts identical=true`. If it prints a WARNING, the cache is off.

If the model provider fails partway (Vertex 429/504 surface as 502 `model_unavailable`), the seed waits and retries each request (1, 2, 4 then 8 minutes; `-retries`). With the 10-minute HTTP timeout, the worst case is roughly 65 minutes for one request (5 attempts × 10 min + 15 min of sleeps), so the old "3–5 min" estimate only holds when the provider is healthy. Run it the day before and leave time to verify; do not start it during the talk. To restart a seed that stopped, delete the cases it created (admin token, `DELETE /v1/cases/{id}`) and run it again: every finished prediction and precedent check is cached (D50, D54), so a rerun repeats those answers exactly and only computes what never finished. That is not a re-roll.

## 2. What the first run decides, and what not to do

The precedent check is not fully repeatable from run to run (D31, D33). The heat gun's motor-fan + PCBA move has checked 0.92 in one run and 0.65–0.90 in others. A fixed seed on every model call (D76) makes runs agree much more often, but not always. The cache records the first verdict; it does not choose one.

- **Don't re-seed until a plan clears the bar.** Picking a favourable run would misrepresent the system. If no plan clears 0.92, record the "No plan cleared the bar" screen and "Draft for the closest move"; it shows the bar doing its job.
- To start fresh for another reason (new provider, changed code), delete `data/verifier-cache` and `data/cases`, then seed again. Note the reason in the video log.
- The scenarios screen shows the engine's answer for a move and, when the plan search tried the same move, its precedent check (D220). Use it to show "move X → the engine says Y", not as a plan.

## 3. Click path (main story and new capabilities, ~2:30 of the 3:00)

Checked against production (https://undelabs.dev, also `frontend-88903435351.us-central1.run.app`; heat gun `case_878d76540b91`) on 18 Oct after the rename. Every number below is what that case shows; nothing re-runs a model, because predictions and verdicts are cached.

1. **Cases home.** The pinned demo cases, newest first. Open "Heat gun, 2022 setup (Kobalt 12.5-amp)". Say: "The header reads UndeLabs; *unde* is Latin for 'from where?'."
2. **Product.** CBP's own 2022 facts for the Kobalt heat gun, as pasted (N328954). Point at "Find the likely origin".
3. **Origin.** **China, calibrated 0.60 (the model stated 0.85)**, below the 0.90 auto-accept bar, so the route is **needs info**: "no earlier ruling on point: answer the questions first". CBP held China (N328954), and the page shows "same as this prediction". Show the three follow-up questions (how the heating element and housing are made in Thailand, and the cost split). Open "Earlier CBP rulings read" (cited N348083 and N351922). The beat to say: *the model is unsure about today's setup, and it says so instead of guessing.*
4. **Ask the case** (optional, ~15 s; the answer is already recorded): "Why this origin, and which earlier ruling is closest?"
5. **Scenarios.** Three columns, with illustrative economics on $2M a year:
   - **As today:** Thailand, calibrated 0.60 (stated 0.78), needs review, $0. The engine reads the structured product, not CBP's text; CBP's holding is China, which is why the plan starts from China. The card says this on screen (D220), and so does the Origin step.
   - **Motor-fan assembly in Thailand:** the engine says Thailand, calibrated 0.99 (stated 0.98), net **$380k/yr**, but the card also shows Plan's verdict on the same move: "△ Below the bar", not precedent-supported (0.90 < 0.92). It is not marked highest net (D220).
   - **Motor-fan + PCBA in Thailand:** the engine says Thailand, calibrated 0.99 (stated 0.99), net **$320k/yr**; Plan found no supporting ruling for it (check 0.85).
   - Above the cards, the plan search's recommendation: the three-group move at $285k. Say: *the what-if is the engine's answer; a move becomes a plan only when the precedent check reaches 0.92.*
6. **Plan.** In order:
   1. Show the estimate line under the button: 7 moves; a replay costs nothing.
   2. Run the search. It replays from the cache in seconds.
   3. Show the plan: **make the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand: check 0.96, supported by N345938, no counter-ruling, net $285k/yr.**
   4. Open "How the check found this" to show the agent's searches and the rulings it read.
   5. Open the backtest (tour step 2): CBP's real 2026 change (N357771, Thailand) switched 6 parts from China to Thailand, and the plan moves all 6. The plan also moves the switch, wires and fasteners, which the real change kept in China. The outer shells N357771 lists as Thai were already Thai in 2022, so they are not counted. The page says this is not a blind test (D84). It does not claim an exact match, and you shouldn't either. Since D330 the template keeps the power cord, knobs and covers apart from the switch and wiring, so a fresh search can propose CBP's smaller move; the pinned plan predates that and does not change until it is re-pinned. "Next: the broker draft" at the foot of the backtest goes to tour step 3.
   6. Point at the closest below-bar row, **motor-fan assembly alone, 0.90 on N325810**, and its advice box: "Would match N325810 if: make the PCBA in Thailand…". The beat: *the bar held a 0.90 back, and the advice points to the parts that cleared it.*
   7. Optional, if a judge asks about money: the duty panel. China 41.7% (MFN 4.2% + Section 301 List 2 25% + 2026 Section 301 12.5%) against Thailand 16.7%.
7. **Draft for broker.** It opens on the shown plan ("Draft for today's setup instead" is the second choice): a 19 C.F.R. § 177.2 ruling request, in the requester's voice (CBP's reply wording in the pasted facts is left out, D180). Its banner says "generated by UndeLabs… check every fact and citation before filing". Print or download.
8. **Photo import.** "+ New case" → Import BOM → click the "Photo (synthetic)" sample under the drop zone; it loads straight into the importer. It opens in Parts and plants as a draft to check. Point at the note that photos are read by AI into a draft. If you quote accuracy, say "on 20 synthetic images, illustrative: 151 of 170 part names, no wrong country"; real photos are not measured.
9. **A newer product family (do not click live).** Cases → "More demos · not run on this site yet" → gas grill. Do **not** press "Load demo": point at the "Development run · not run on this site" chip and open its committed JSON record. CBP held China (N335085, scenario 1); the recorded development plan targets Indonesia and the backtest is the same ruling's scenario 2 (lids and fireboxes formed in Indonesia: Indonesia). Say plainly that this is a recorded development run, not this site's answer.
10. **Claude connector.** /developers ("API · MCP" in the top bar) shows the one-line setup and the 14 tools. In a terminal: `claude mcp add --transport http unde https://undelabs.dev/mcp`, then ask "Where would CBP say a heat gun assembled in Thailand from Chinese motor and PCBA is from?".
11. **Accuracy.** Say these numbers:
   - **90.2%** on 377 held-out CBP rulings (Vertex, official), against 63.4% for the best simple rule we recomputed (first country named; D155). On 83 newer rulings (Sep–Nov 2026, gateway, scored once): 92.8%. If asked about the gateway: it agrees with Vertex AI on the development set (p = 0.58, D151).
   - The fast path auto-accepts 79.6% of cases at 96.0%.
   - The "Precedent check on older pairs" section: read its headline, **182 checks passed, across all runs. None was on a pair where CBP kept the origin.** The proxy-pool wording ("in the historical D86 proxy pool…", 54 development pairs and 21 pairs from 2026, 17 pairs counted once) is in that section's Details fold; open it if a judge asks. Then point at "Plan checks on newer rulings": 9 of 9 moves shown were real changes with the starting origin given (5 of 6 without it, shown on /method). These are not guarantees.
   - The "Known failure cases" section: every held-out miss, by ruling. Scroll to it; showing the misses is part of the claim.

**Before recording:**
- Run the smoke test once. The backend is private, so on production it runs through the frontend's `/api` proxy (`scripts/smoke.sh <frontend URL>/api`, health checks skipped).
- Open the demo URL a minute early so the first load is warm.

**Side stories if time allows:**
- **LED tub light** (section 5): its LED PCBA plan is shown at 0.94, exactly CBP's next ruling. Its duty rates are labelled illustrative.
- portfolio triage (`backend/testdata/portfolio_sample.csv`)
- BOM import from a spreadsheet (`frontend/public/samples/heatgun-bom.xlsx`)
- **Scenarios** (step 5 above): move X, see the origin and the illustrative net per year
- compare target countries
- ruling watch
- the LiDAR template (a second product family)

**Sections 4–5 below are earlier runs, kept as a record. Record from this section and section 6.**

## 4. Seeded outcomes (LiteLLM proxy, 16 Oct, run 1, fresh cache; replace with the recording-day run)

| Demo | Prediction | CBP held | Plan |
|---|---|---|---|
| Heat gun (CBP facts) | China 0.85, needs info | China (N328954) | none shown; closest PCBA in Thailand, check 0.75 |
| LiDAR | Thailand 0.92, auto | Thailand (N359371) | none shown; closest boards to Vietnam, check 0.75 |
| Dryer latch, moulded | Thailand 0.96, auto | Thailand | n/a |
| Dryer latch, assembled | China 0.99, auto | China | n/a |

Heat gun extras: scenarios as today $0 (Thailand 0.75, differs from CBP), motor-fan in Thailand 0.95 net $380k, motor-fan + PCBA 0.98 net $320k; watch unchanged; ask cited N351922 and N348083; countries: Thailand below the bar, Vietnam and Malaysia no supported plan. Repeat search: identical verdicts, 6/6 cached, 8 s.

### Run 2 (LiteLLM proxy, 16 Oct, clarify-v2, empty cache for these moves)
Re-seeded to save full responses for the frontend (`make demo-seed` with `-dump`; `eval/results/demo-seed-heatgun-{predict,plan,scenarios}-litellm.json`, summary `eval/results/demo-seed-litellm.json`), not to look for a better result. Run 1's cache was in a scratch directory, so every check ran fresh.

| Demo | Prediction | Plan |
|---|---|---|
| Heat gun | China 0.85, needs info (no ruling on point; 3 questions; a conflict, no lean) | **motor-fan + PCBA in Thailand shown: check 0.92 (exactly the bar), net $320k/yr** |
| LiDAR | Thailand 0.95, auto | none shown; closest boards to Vietnam, check 0.90 |
| Dryer latches | Thailand 0.96 / China 0.99, auto | n/a |

Heat-gun scenarios: as today China 0.65 (now matches CBP; run 1 gave Thailand 0.75), motor-fan in Thailand 0.96 net $380k, both 0.98 net $320k. Compare countries: Thailand plan, Vietnam and Malaysia no plan. Repeat search: identical, 6/6 cached, 1.3 s.

The two runs disagree on the heat gun's plan (0.75 vs 0.92) and on the as-today scenario. That is the run-to-run variation in section 2: the 0.92 sits exactly on the bar. Design both the shown-plan and no-plan screens; the recording-day run (Vertex) decides which one the video shows, and neither proxy run should be presented as the official outcome.

### Vertex (16 Oct, staging, one seed)
One seed of `origin-proof-api-staging` revision `origin-proof-api-staging-00004-62n` (engine `gemini-4-argon`, `LLM_PROVIDER=vertex`). Not repeated. Files: `eval/results/demo-seed-heatgun-{predict,plan,scenarios}-vertex.json`, summary `eval/results/demo-seed-vertex.json`. The four cases are pinned.

| Demo | Prediction | CBP held | Plan |
|---|---|---|---|
| Heat gun (CBP facts) | China 0.78, needs info (no ruling on point; 3 questions; a conflict, no lean) | China (N328954) | none shown; closest motor-fan assembly + PCBA in Thailand, check 0.90, net $320k/yr |
| LiDAR | Thailand 0.95, auto | Thailand (N359371, scenario I) | none shown; boards to Vietnam at check 0.85 (see note) |
| Dryer latch, moulded | Thailand 0.98, auto | Thailand | n/a |
| Dryer latch, assembled | China 0.98, auto | China | n/a |

### Production (Vertex, recording day)
One seed of production revision `backend-00023-mxc` (engine `gemini-4-argon`, `LLM_PROVIDER=vertex`) on 16 Oct. A first attempt stopped on a LiDAR plan 502; the cases from that attempt were deleted and this seed was resumed. Finished predictions and precedent checks came back from the Firestore caches (`engine_cache`, `verifier_cache`; D54, D50), so the heat-gun plan is the same answer as the first attempt, not a re-roll. Files: `eval/results/demo-seed-heatgun-{predict,plan,scenarios}-vertex-prod.json`, summary `eval/results/demo-seed-vertex-prod.json`. Pinned cases: heat gun `case_878d76540b91`, LiDAR `case_089ff641030e`, dryer latches `case_b4c60b15a9a9` and `case_c1bcdb3a17bf`, compare-targets `case_2762301d37f6`.

| Demo | Prediction | CBP held | Plan |
|---|---|---|---|
| Heat gun (CBP facts) | China 0.85, needs info (no ruling on point; 3 questions; a conflict, leans A) | China (N328954) | none shown; closest PCBA in Thailand, check 0.75 |
| LiDAR | Thailand 0.95, auto | Thailand (N359371, scenario I) | none shown; closest main boards and RX/TX PCBAs in Vietnam, no supporting precedent, check 0.45 |
| Dryer latch, moulded | Thailand 0.98, auto | Thailand | n/a |
| Dryer latch, assembled | China 0.99, auto | China | n/a |

Heat gun extras: as today Thailand 0.78 net $0 (differs from CBP), motor-fan in Thailand 0.98 net $380k, motor-fan + PCBA 0.99 net $320k; watch unchanged; ask cited N328954, N351922 and N348083; countries: Thailand below the bar, Vietnam no plan, Malaysia no plan. Repeat search: verdicts identical=true, 6/6 from cache, 0.1 s. The heat-gun plan is below the bar, so the video uses the no-plan screen (closest move: Make PCBA (SMT) in Thailand, check 0.75). The compare-targets call returned 502 `model_unavailable` three times and succeeded on the next try. Neither proxy run above is the official outcome; this 16 Oct run is kept as a record, and the later 17–18 Oct production runs below are what the video now uses.

**verifier-v2 (D74), LiteLLM proxy, 17 Oct, one run on a fresh cache:** the heat gun shows one plan clearing 0.92: motor-fan assembly + switch, cord and minor parts to Thailand, check 0.92, N347421 for, no counter-ruling, net $345k/yr (`eval/results/plan-heatgun-2022-verifier-v2-litellm.json`). Motor-fan + PCBA checked 0.90 and motor-fan alone 0.85. The landing named this LiteLLM plan until production's run (below) showed none over the bar; it now states production's closest move. The production run below is the demo's stored result.

### Production (Vertex, 17 Oct, verifier-v2, one run each)
Revision `backend-00038-zw2` after merge `41ce826` ([PR #186](https://github.com/kendramoss/wassup-hackathon-2026/pull/186)). Prompt `verifier-v2+known-a`, model `gemini-4-argon`. Each search ran once and was not repeated. Engine token counts were 0 (predictions reused from the engine cache). Verifier checks were not.

**Heat gun** `case_878d76540b91`, plan body from `GET /v1/demos/heatgun-2022`. HTTP 200 in 17.7 s. Stats: 6 moves searched, 7 engine calls, 6 verifier runs, **verifier_cached 0**, 20 verifier model calls, 36 tool calls, 17.4 s. **No plan cleared 0.92.** The Plan screen's closest row is motor-fan assembly in Thailand, check 0.90, N325810, no counter-ruling, net $380k/yr. The landing's move (motor-fan assembly + switch, cord and minor parts) checked 0.75 on N345859, so the landing was then changed to state production's result (closest move, motor-fan assembly, 0.90, N325810; #150).

| Move | Result | Check | For | Against | Net | Predicted origin |
|---|---|---|---|---|---|---|
| Current setup | rejected, current setup (CBP held China, N328954) | — | — | — | $0 | Thailand 0.78 |
| PCBA (SMT) in Thailand | below the bar | 0.75 | N347421 | none | $440k/yr | Thailand 0.92 |
| PCBA + switch, cord and minor parts in Thailand | below the bar | 0.78 | H309758 | none | $405k/yr | Thailand 0.95 |
| Motor-fan assembly in Thailand | below the bar (closest) | 0.90 | N325810 | none | $380k/yr | Thailand 0.96 |
| Motor-fan assembly + switch, cord and minor parts in Thailand | below the bar | 0.75 | N345859 | none | $345k/yr | Thailand 0.98 |
| Switch, cord and minor parts in Thailand | no supporting precedent | 0.55 | none | none | $465k/yr | Thailand 0.90 |
| Motor-fan assembly + PCBA in Thailand | no supporting precedent | 0.85 | none | H309758 | $320k/yr | Thailand 0.98 |

**LiDAR** `case_089ff641030e`, plan body from `GET /v1/demos/lidar-s1`. HTTP 200 in 17.9 s. Stats: 3 moves searched, 4 engine calls, 1 verifier run, **verifier_cached 0**, 3 verifier model calls, 6 tool calls, 17.8 s. **No plan cleared 0.92.** Closest: main boards (TMB and BMB) and RX/TX PCBAs in Vietnam, check 0.85, N322834, no counter-ruling, net $30k/yr, origin Vietnam. Main boards alone, and RX/TX boards alone, were predicted Thailand (not verified); nets $20k and $10k. Current setup stays Thailand.

**Compare countries** `case_2762301d37f6`, body `{"targets":["Thailand","Vietnam","Malaysia"]}`. HTTP 200 in 27.2 s. **No country cleared 0.92.**

| Rank | Country | Status | Searched | Closest | Check | For | Against | Net |
|---|---|---|---|---|---|---|---|---|
| 1 | Malaysia | below the bar | 15 | Motor-fan assembly and heating element in Malaysia | 0.90 | N347421 | none | $455k/yr |
| 2 | Thailand | below the bar | 6 | PCBA (SMT) in Thailand | 0.75 | N347421 | none | $440k/yr |
| 3 | Vietnam | no plan | 15 | — | — | — | — | — |

"Closest" means what the Plan screen shows: the below-bar move with the highest precedent check (ties keep list order). The seed summary used to print the first below-bar row instead, so the heat-gun row above was corrected on 17 Oct from `demo-seed-heatgun-plan-vertex.json`: the five below-bar moves checked 0.85, 0.85, 0.85, 0.90 (motor-fan + PCBA) and 0.85. `eval/results/demo-seed-vertex.json` still has the old summary line. The LiDAR plan was not dumped, so its row may name an equal-or-lower move than the screen; check the Plan page. Proxy run 1's "closest" lines have the same caveat.

Heat gun extras: as today Thailand 0.75 net $0 (differs from CBP), motor-fan in Thailand 0.98 net $380k, motor-fan + PCBA 0.98 net $320k; watch unchanged; ask cited N351922 and N348083; countries: Malaysia below the bar, Thailand below the bar, Vietnam no plan. Repeat search: identical verdicts, 6/6 cached, 135.8 s. The heat-gun plan is below the bar, so the video uses the no-plan screen.

## 5. LED tub light: a plan that clears the bar (production, 17 Oct)
Demo `tublight-2025`, case `case_100715a11651` (pinned). CBP's July 2025 facts: all Chinese parts assembled in Thailand. CBP held China (N350085). A month later, the same manufacturer made the LED PCBA in Thailand and CBP held Thailand (N351143). Both rulings are excluded, and the planning date is 9 Jul 2025. This pair is among the dev flip pairs verifier-v2 was written from, so the demo is a demonstration, not a test (D77).

Prediction: China 0.98, auto. Plan, `backend-00045-hh2`, seeded (D76), 320 s, `verifier_cached` 0:

| Move | Result | Check | For | Against |
|---|---|---|---|---|
| LED PCBA (SMT) in Thailand | **shown** | 0.94 | N345899 | none |
| LED PCBA + base, harness and seals in Thailand | **shown** | 0.96 | N344188 | none |
| LED PCBA + lens moulding in Thailand | below the bar | 0.90 | H331515 | none |
| Lens, base/harness/seals, or both | engine predicts China | — | — | — |

The top plan moves exactly what CBP's real change moved (backtest: matches N351143). Duty rates are illustrative: no ruling classifies the tub light.

History, for the record: the first production search (`backend-00043-bvb`) lost all three checks to Vertex 504s. The second (`backend-00044-q8k`, unseeded) checked LED PCBA at 0.90 on H304910. With Anas's approval, those three verdicts were deleted and checked once more with the seed, giving the result above (D76). The heat gun's verdicts were never touched.

## 6. Heat gun: the plan that clears the bar (production, 18 Oct)
`case_878d76540b91`, revision `backend-00051-kmk`. Seven moves; one is shown.

| Move | Result | Check | For | Against |
|---|---|---|---|---|
| **Motor-fan assembly, PCBA and switch, cord and minor parts in Thailand** | **shown** | **0.96** | **N345938** | none |
| Motor-fan assembly in Thailand | below the bar (closest) | 0.90 | N325810 | none |
| The other five | below the bar or no supporting precedent | — | — | — |

- **Net:** $285k/yr (illustrative economics).
- **Backtest against N357771** (CBP, 29 Jan 2026, Thailand): the change switched 6 parts from China to Thailand (motor, fan, PCBA, knob, power cord, back cover), and the plan moves all 6. It also moves the switch, wires and fasteners, which the change kept in China. The outer shells, also listed as Thai in N357771, were already Thai in this setup and are not counted. Not a blind test: three-group moves were added after seeing this change (D84).

**How it got here, for the record:**
1. The first searches (pairs of groups only) never tried a three-group move (D84 added them).
2. The first production check on that move had 4 of its 6 searches come back empty, because CBP's search needs every word to match. It stored 0.96, "origin changes", with no supporting ruling.
3. After the relaxed search fix (D89), and with Anas's approval, that one stored verdict was deleted and checked once more: 0.96 on N345938, shown. The other six verdicts were never touched.


**Run again (D321, D322, 21 Nov):** three pre-registered runs with production's verifier settings through the gateway each showed a plan, and the same three moves: the three-group move at 0.95, 0.96 and 0.96, and two pairs at or just above the bar (0.92–0.94). None cited N345938; the three-group move cited N347421 once and N345859 twice. Table: eval/RESULTS.md, "Plan check run again".

**After the split (D330, D331, D332, 22 Nov):** the power cord, knobs and covers are now their own group, so the search can propose CBP's own move (motor-fan assembly, PCBA, and power cord, knobs and covers; switch, wiring and fasteners stay in China). Two pre-registered gateway runs with the same settings showed it both times, check 0.94 on N347421. It ranked 5th of 7 and 4th of 6 by net: cheaper moves that move fewer parts, at the 0.92 bar, ranked above it, and all four groups checked higher (0.96). So the top-ranked plan, which the backtest reads, moved 2 of 6 and 5 of 6 of the changed parts. This change was made after seeing N357771; it is not a blind test. The pinned production plan is unchanged until an owner re-pins it. Table: eval/RESULTS.md, "Plan after the split".

## 7. New-family demos with recorded runs (D191, 21 Nov)
Five demos come from the families added in D138. Each card on /cases shows CBP's holding and a recorded development run (one run each, through an OpenAI-compatible LiteLLM gateway to Gemini, local corpus search, plan bar 0.92). Four were picked by family, not by result (D191); one of them disagrees with CBP, and the card says so.

| Demo | CBP held | Engine (recorded run) | First plan shown | Cited ruling, read by hand |
|---|---|---|---|---|
| Gas grill (N335085, scenario 1) | China | China, stated 0.95, calibrated 0.90 | lid and firebox forming, burners and gas train, cooking surface and frame in Indonesia, check 0.96, N333420 | not spot-checked |
| Aluminum truck wheel (N349871) | China | **Thailand, stated 0.85, calibrated 0.60: disagrees** | housings and structural parts (the forging) in Thailand, check 0.94, N346803 | partial |
| Washing-machine speed sensor (N350727) | China | China, stated 0.98, calibrated 0.99 | PCBA in Thailand, check 0.96, N344809 | supports |
| Power bank (N352355) | China | China, stated 0.95, calibrated 0.90 | power conversion module in Vietnam, check 0.96, N352014 | supports |
| PCR detection instrument (N333453) | China | China, stated 0.98, calibrated 0.99 | PCBA and sensors in Japan, check 0.92, N330296 | partial |

Sources: `eval/results/families-plan-gas-grill-2023-litellm.json`, `eval/results/planner-families-{n349871,n350727,n352355,n333453}-litellm.json`, `eval/planner/spot_checks.json`.

**Before recording or judging:** run `make demo-seed` against the production API with the admin token so each of these has a pinned case on Vertex AI. That run is new and may differ from the table. Record what it shows, including a disagreement.

## 8. Try it: codes for each part, then the rule check (D380)

Not part of the tour: running it creates a new case of your own. To try it:

1. Open `/cases/new?demo=heatgun-2022`. The parts table has 13 parts and no part codes; the product already has 8419.89.95.
2. Press **Suggest codes for all parts**. Each part shows up to 3 codes, each marked "earlier CBP ruling" (with links) or "model's reading, no ruling found", with one sentence of reason. Nothing is filled in.
3. Press **Use** on a code, or **Accept all top suggestions** (it fills only parts without a code). Type a code for any part that got none. Check them: the suggestions are unverified.
4. Press **Find the likely origin**. On the origin step the **Rule check (19 CFR 102.20)** panel decides when every foreign part has a code the rule can read. The heat gun is a substantial-transformation case, so the panel is labelled "for comparison only".

Locally with `LLM_PROVIDER=fake`, each part gets the first code its nearest ruling lists and the reason says "Local fake model ... not judged"; on 22 Nov that run filled 12 of 13 parts, and with the 13th typed in the panel decided Thailand (the fake engine said China, so the panel showed "worth a broker's look"). Screens at 1280 and 390 px were checked that day. With a real model the codes and the outcome differ.

## 9. One demo for each of the six D660 families (#558, 22 Nov)

The six families added in D660 now have cards under "More demos". Their product facts come from development-pool CBP rulings, and the cards carry one recorded development run each (`eval/results/families-r3-live-checks.json` and its six result files). Duty rates, import values and group costs remain illustrative. Until they are seeded, they are unpinned and a click starts a live run.

| Demo | CBP held | Engine (recorded run) | First plan shown | Cited ruling, read by hand |
|---|---|---|---|---|
| Paper napkins (N341898) | Indonesia | Indonesia, stated 0.95 | parent rolls to China, check 0.98, N340077 | supports the parent roll deciding |
| Roll AGV (N336445) | China | **South Korea, stated 0.82: disagrees** | chassis and working frames to South Korea, check 0.96, N334276 | by analogy (scissor lifts); CBP's N337718 change was smaller |
| Integrated circuits (N333934) | South Korea | South Korea, stated 0.98 | bare wafers and front-end fabrication to China, check 0.95, N333677 | partial: a pressure sensor, not an IC |
| Golf club heads (N346303) | Vietnam | Vietnam, stated 0.90 | club heads to China, check 0.97, N340536 | supports the forming country deciding |
| Plush dinosaur (N343777) | China | China, stated 0.98 | **no plan above 0.92**; closest cutting and sewing to Vietnam, 0.90 on N342448 | honest send-to-broker result; CBP's scenario 2 made that move |
| Vent-free gas heater (N345976) | China | China, stated 0.92 | burners, oxygen sensor and ignitor to Vietnam, check 0.94, N346005 | supports the burner and sensor assemblies deciding |

After the owner seeds production, replace this paragraph with the live case IDs and what each screen actually shows. Do not present a recorded development run as this site's answer.

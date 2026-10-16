# Recording the demo

Video: 2:45. Judges first. Every number on screen must trace to `docs/NUMBERS.md`, and model outputs shown are what the system produced on the recording day.

## 1. Prepare (about 10 minutes, the day of recording)

1. Start the backend on Vertex, the only model provider (D49). Local development needs Application Default Credentials (`gcloud auth application-default login`).
   ```bash
   make run   # GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 GOOGLE_CLOUD_LOCATION=global
   ```
   Keep the verdict cache on (the default: `VERIFIER_CACHE_DIR=../data/verifier-cache`). Cases are saved under `data/cases`.
2. Seed every demo:
   ```bash
   make demo-seed   # ~3–5 min; writes eval/results/demo-seed.json
   ```
   `make run` and `make demo-seed` share a local admin token, so the seed is not rate-limited and pins the demo cases (listed first; visitors cannot delete them, D45). Against Cloud Run, set `ADMIN_TOKEN` to the service's token and `ID_TOKEN=$(gcloud auth print-identity-token)`, then `go run ./cmd/demo seed -api <URL>`.
   It creates a case for each demo, runs the plan searches, and fills the heat gun's scenarios, ruling watch, one ask and the multi-country comparison. It then repeats the heat-gun plan search and prints `verdicts identical=true`. If it prints a WARNING, the cache is off.
3. Open `eval/results/demo-seed.json` and write down what the plan screens will show. From now on, replays return these verdicts (in seconds, not ~30 s).

## 2. What the first run decides, and what not to do

The precedent check is not repeatable from run to run (D31, D33): the heat gun's motor-fan + PCBA move has checked 0.92 in one run and 0.65–0.90 in others. The cache records the first verdict; it does not choose one.

- **Don't re-seed until a plan clears the bar.** Picking a favourable run would misrepresent the system. If no plan clears 0.92, record the "No plan cleared the bar" screen and "Draft for the closest move"; it shows the bar doing its job.
- To start fresh for another reason (new provider, changed code), delete `data/verifier-cache` and `data/cases`, then seed again. Note the reason in the video log.
- The scenarios screen does not depend on the precedent check, so it is the stable way to show "move X → saves $Y" (heat gun: motor-fan assembly to Thailand, Thailand 0.95, net $380k/yr, illustrative economics).

## 3. Click path (main story, ~2:00 of the 2:45)

1. **Cases home.** The seeded demo cases. Open the heat gun.
2. **Product.** CBP's 2022 facts for the Kobalt heat gun, as pasted. "Find the likely origin".
3. **Result.** Verdict, confidence ruler against the 0.92 bar, the earlier rulings with what CBP held, the check against CBP's own 2022 ruling (China). Below the bar: the follow-up questions.
4. **Ask the case** (optional, ~15 s; the answer is recorded already): "Why this origin, and which earlier ruling is closest?"
5. **Scenarios.** As today vs motor-fan assembly in Thailand vs motor-fan + PCBA: origin, confidence and net per year.
6. **Plan.** The streamed search (replay: fast), then the best plan, or "No plan cleared the bar" with the closest move.
7. **Draft for broker.** The letter, with the below-bar banner if it applies. Print or download.
8. **Accuracy.** The results page: overall and fast-path accuracy with CIs, labelled official (Vertex) or proxy.

Before recording, run `scripts/smoke.sh <API>` (with `ID_TOKEN` and `ADMIN_TOKEN` against Cloud Run): deep health, a predict, the case list and one plan search; it deletes its case afterwards.

Side stories if time allows: portfolio triage (`backend/testdata/portfolio_sample.csv`: 2 auto, 1 needs info, 2 review including an out-of-scope textile, D47), LiDAR (second product family), BOM import (`backend/testdata/heatgun_bom.csv`), compare target countries (all below the bar in the last run, D36), ruling watch.

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


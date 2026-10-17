Stacked on #<feat/origin-proof-api PR>. Base: `feat/origin-proof-api`. Don't merge until the backend is finished and the GCP/Vertex setup is done.

## What this adds

**Product (API, contract in `docs/api/openapi.yaml`)**
- **Bars:** auto-accept and plan bars are both 0.92 (D32, D35).
- **Clarify and needs-more-information route:** below the auto-accept bar, a clarify review says whether an earlier ruling is on point, shows conflicting earlier rulings with the deciding difference, and asks up to 3 follow-up questions. Cases with no ruling on point and with questions route to `needs_info`. `POST /v1/cases/{id}/answers` adds the answers and re-analyses (answers-v2 drops "not stated" sentences; D24, D26).
- **Saved cases:** `GET /v1/cases` lists cases newest first. Cases persist as JSON files under `CASES_DIR` (local only; Firestore before deploy, D25).
- **Scenarios side by side:** `POST /v1/cases/{id}/scenarios` predicts 1–3 alternative setups. Heat gun: as today 0.75 needs review; motor-fan in Thailand 0.96 auto; motor-fan + PCBA 0.98 auto (D29).
- **Ruling watch:** `POST /v1/cases/{id}/watch` and `POST /v1/watch/run` re-run a case with every precedent available today and flag a changed origin (D29).
- **Streamed plan search:** `POST /v1/cases/{id}/plan` with `Accept: text/event-stream` sends start, baseline, move_started, move_done, done/error, plus keep-alives. The baseline now runs in parallel and concurrency defaults to 8: heat gun 33 s → ~25 s (D30).
- **Ruling held origin:** `precedents_detail` on cases and `origin` on `GET /v1/rulings/{id}`.
- **Export:** the draft's "Requester's position" sentence is now grammatical. `move_key` drafts a shown or below-bar plan, with a below-the-bar notice (D31).
- **Repeatable verdicts:** the verifier cache gives the same verdicts on repeated plan searches (D31). Also fixes `CASES_DIR` not being read, which meant cases weren't actually persisted.

**Evaluation (dev only, LiteLLM proxy; labelled "proxy, development only")**
- Scoring fixes applied to every split: A/B holdings normalisation and verified label adjustments (D23).
- Accuracy experiments, none adopted. All within ±0.5 points of engine-v1's 90.9% on dev:
  - voting, hybrid and embedding retrieval (D22)
  - written CBP rules: 91.1%, 10 fixed / 9 broken (D27)
  - CBP reasoning with each precedent: 91.4%, 7 fixed / 5 broken (D28)
- Follow-up answers on split votes: no move toward CBP; led to answers-v2 (D26).
- New eval commands: `compare` (exact McNemar), `clarify`, `torn`, `learnability`, `multiscenario`. Flags `-rules` and `-reasoning`, plus hybrid/vector retrieval and voting, are kept for later tests.

## Numbers (dev, proxy)
- engine-v1: 90.9% overall (n = 463). Fast path: 336 cases (73%) at 96.4%.
- No held-out runs in this PR: no change was adopted, so the frozen held-out result stands. Official numbers wait for Vertex.

## Known issues
- The precedent check is not deterministic through the proxy: the heat-gun plan cleared the bar in one run and checked 0.92 in another (D30).
- Cloud Run case persistence and the scheduled watch need the GCP setup (issue: GCP setup).
- Fine-tuning is tracked separately (issue: fine-tune the engine on Vertex).

## Checks
- `make test lint` passes (gofmt, vet, staticcheck, tests including `-race` on the handlers and planner).
- Live checks through LiteLLM for scenarios, watch and streaming on the heat-gun demo. Samples are in `eval/results/`.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

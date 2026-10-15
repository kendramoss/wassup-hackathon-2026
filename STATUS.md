# Status — feat/origin-proof-flash-staging

15 Oct 2026. Branch is stacked on `feat/origin-proof-demo` (restacked 16 Oct; the decision is D44, after D43 economics).

## Done

- Untuned `gemini-3.5-flash` dev baseline on Vertex, location `global`. Engine-v1, BM25, K = 5. No held-out run. Rows: `eval/results/dev-gemini-3.5-flash-prec-engine-v1-vertex.jsonl`. Decision: D44.

## Numbers (dev, scoring adjustments applied, n = 463)

| | Gemini 4 Argon | Gemini 3.5 Flash, untuned |
|---|---|---|
| Accuracy (95% CI) | 91.1% (88.2–93.4) | 90.5% (87.5–92.8) |
| Fast path, ≥ 0.92 and in scope | 79.5% of cases at 96.5% (94.1–97.9) | 91.6% of cases at 92.0% (89.0–94.2) |
| Latency p50 / p95 | 4.8 s / 8.1 s | 12.1 s / 51.2 s |
| Cost per case | $0.0079 promo | $0.017 standard global |

McNemar against Argon: Flash fixed 16, broke 19, p = 0.736. The gap is 0.6 points, inside the ~3-point band, so a tuned Flash could plausibly match Argon's overall accuracy. Flash's fast path is under the 95% accuracy the 0.92 bar was chosen for. The result file's `cost_usd` is 0; the $0.017 uses Vertex global standard $1.50 / $9.00 per 1M tokens (output includes reasoning), checked 15 Oct 2026. This run was about $8, not the ~$2 estimate.

## Blocked

- Staging Cloud Run service `origin-proof-api-staging` was not created. The brief says to deploy `feat/origin-proof-demo` and to ask before a billable service. The demo branch (seed, audit trail, extract) is not on the fork.
- `roles/datastore.user` is still missing on `agent-backend`. Only a project owner can grant it. Case saves on Firestore would fail until then.
- Tuning job was not launched. `gemini-4-argon` cannot be tuned. Gemini 3.5 Flash supervised fine-tuning is $0.01 per 1,000 training tokens ($10 per 1M). A rough training bill at 632 examples, ~3,000 tokens, 3 epochs is on the order of $60, plus about $12 to build the train and tune_check targets. Needs a yes before either spend.

## Next

- Anas: say whether to spend the tuning-data run and the job. If yes, base model `gemini-3.5-flash`, dev comparison first, held-out only if dev improves.
- Put `feat/origin-proof-demo` on the fork, then deploy `origin-proof-api-staging` from that commit. Not the production service `backend`.
- A project owner grants `roles/datastore.user` to `agent-backend` before the staging seed can save cases.

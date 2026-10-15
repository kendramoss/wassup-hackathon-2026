# Status — feat/origin-proof-flash-staging

15 Oct 2026. Branch is stacked on `feat/origin-proof-demo` (restacked 16 Oct; the decision is D44, after D43 economics).

## Done

- Untuned `gemini-3.5-flash` dev baseline on Vertex, location `global`. Engine-v1, BM25, K = 5. No held-out run. Rows: `eval/results/dev-gemini-3.5-flash-prec-engine-v1-vertex.jsonl`. Decision: D44. Anas: no Flash tuning before the freeze. D44 stays open. No tuning job was launched.
- Staging Cloud Run service `origin-proof-api-staging` deployed from this branch (`76b1720`), revision `origin-proof-api-staging-00001-sgq`. Production service `backend` was not changed (still `backend-00014-dt5`). Engine is Gemini 4 Argon. Timeout 300 s. Service account `agent-backend`. Env: `LLM_PROVIDER=vertex`, `GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026`, `GOOGLE_CLOUD_LOCATION=global`, `CASES_STORE=firestore`, `VERIFIER_CACHE_DIR=/tmp/verifier-cache`.

## Numbers (dev, scoring adjustments applied, n = 463)

| | Gemini 4 Argon | Gemini 3.5 Flash, untuned |
|---|---|---|
| Accuracy (95% CI) | 91.1% (88.2–93.4) | 90.5% (87.5–92.8) |
| Fast path, ≥ 0.92 and in scope | 79.5% of cases at 96.5% (94.1–97.9) | 91.6% of cases at 92.0% (89.0–94.2) |
| Latency p50 / p95 | 4.8 s / 8.1 s | 12.1 s / 51.2 s |
| Cost per case | $0.0079 promo | $0.017 standard global |

McNemar against Argon: Flash fixed 16, broke 19, p = 0.736. The gap is 0.6 points, inside the ~3-point band, so a tuned Flash could plausibly match Argon's overall accuracy. Flash's fast path is under the 95% accuracy the 0.92 bar was chosen for. The result file's `cost_usd` is 0; the $0.017 uses Vertex global standard $1.50 / $9.00 per 1M tokens (output includes reasoning), checked 15 Oct 2026. This run was about $8, not the ~$2 estimate.

## Staging

URL: `https://origin-proof-api-staging-guxax2teha-uc.a.run.app`

The service requires authentication. This account can call it with `gcloud auth print-identity-token`. It cannot change the service IAM (`run.services.setIamPolicy` denied), so the URL stays closed to anonymous callers. Startup log: `provider=vertex engine=gemini-4-argon`, `cases=firestore`, corpus loaded (2,553 precedents), listening on 8080. No read-only filesystem error.

| Call | HTTP | Latency | Result |
|---|---:|---:|---|
| `GET /health` | 200 | 0.1 s | `{"status":"ok","llm_provider":"vertex","version":"origin-proof-api-staging-00001-sgq"}` |
| `GET /v1/demos` | 200 | 0.1 s | demo list |
| `GET /v1/demos/heatgun-2022` | 200 | 0.1 s | predict and plan bodies |
| `POST /v1/cases/predict` (heat-gun body) | 500 | 10.7 s | model ran, then the save failed |
| `GET /v1/cases` | 500 | 0.1 s | list failed |
| `POST /v1/cases/{id}/plan` | not run | | no case id |
| plan SSE | not run | | no case id, so keep-alives were not observed |
| `PUT /v1/cases/{id}/economics` | not run | | no case id |

Exact log lines:

- `store: write case_35ba40d0487d: rpc error: code = PermissionDenied desc = Missing or insufficient permissions.`
- `store: list: rpc error: code = PermissionDenied desc = Missing or insufficient permissions.`

That is `agent-backend` missing `roles/datastore.user`. Only Kendra (project owner) can grant it. `CASES_STORE` was left on `firestore`.

`go run ./cmd/demo seed -api https://origin-proof-api-staging-guxax2teha-uc.a.run.app` exited 1 before any prediction. Cloud Run returned 403 on `GET /health` because the seed client sends no identity token. No seed summary, and it did not print `verdicts identical=true`.

## Blocked

- Firestore case saves and lists, until `roles/datastore.user` is granted to `agent-backend`.
- Anonymous access to staging, until someone who can `setIamPolicy` grants `roles/run.invoker`.
- Plan search, SSE keep-alives, economics, and the demo seed, because they need a saved case and (for the seed command) an open URL.
- Flash tuning stays a finals item. No job launched.

## Next

- Kendra grants `roles/datastore.user` to `agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com`, and `roles/run.invoker` on `origin-proof-api-staging` to whoever will call it without a Google token (the seed command and the browser UI).
- Re-run predict, plan, the SSE plan call, economics, and `demo seed` against the same URL. Do not redeploy production.

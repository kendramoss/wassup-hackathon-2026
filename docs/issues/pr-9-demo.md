Ninth stacked PR. Base: `feat/origin-proof-tuning` (#<PR 8>). Don't merge until the backend is finished and the GCP/Vertex setup is done.

This PR also carries the **official Vertex AI numbers** (commits `ce338eb`, `cc3063e`, `39eda27` by the cloud agent, D39): held-out accuracy 90.2% (86.8–92.8), fast path 79.6% of cases at 96.0% (93.1–97.7); dev 91.1%. No significant difference from the proxy run (McNemar p = 1.0). The results page now prefers the Vertex files; the Firestore emulator test passed. Gemini 3.5 Flash (tunable; Argon isn't) was tried untuned on dev: 90.5% vs Argon 91.1% (p = 0.74), but its fast path is only 92.0% accurate and p95 latency is 51 s. Tuning is a finals item (D44, on the cloud agent's `feat/origin-proof-flash-staging`, not in this PR); the product stays on Argon. 

**Before merging (deploy needs these, or the live demo breaks):**
1. A project owner grants `roles/datastore.user` to `agent-backend`. Staging (`origin-proof-api-staging`, deployed by the cloud agent from this branch) reached Vertex but every case save returned PermissionDenied.
2. An `ADMIN_TOKEN` of 16+ characters in Secret Manager, set on the backend service. Without it the demo seed is rate-limited and can't pin the demo cases.
3. The frontend proxy forwards the browser's address as `X-Client-IP`. Without it every visitor shares one rate-limit bucket.

After deploy: `ID_TOKEN=$(gcloud auth print-identity-token) ADMIN_TOKEN=... scripts/smoke.sh <backend URL>` must end with "all checks passed" (deep health, predict, list, one plan search; it deletes its case).

## What this adds
- **`make demo-seed`** (`backend/cmd/demo`, D40): prepares a running server for the video through the real API. `-dump DIR` also saves the full heat-gun predict, plan and scenarios responses.
  - A case per demo (heat gun, LiDAR, two dryer latches), plus each one's plan search.
  - For the heat gun: scenarios, a ruling watch, one ask and the multi-country comparison.
  - Repeats the heat-gun plan search and checks the verdicts match, so recording replays are identical with the verdict cache on.
- **`docs/DEMO.md`:** the recording runbook (prepare, click path, seeded outcomes).
  - The first verdicts are recorded as they come; re-rolling until a plan clears the bar would misrepresent the system.
  - If no plan clears the bar, the video shows "No plan cleared the bar" and a draft for the closest move.
- **Case audit trail** (D41): each case's `history` records predicted, answered, plan_searched, scenarios, targets_compared, watched, asked and exported events. Each has a one-line outcome and the provider, model and prompt version.
- **Draft a product from pasted text** (`POST /v1/products/extract`, D42): parts with their stated countries, assembly steps and plants, each citing its paragraph, with warnings and capability questions. Heat gun: 15 parts (3 Thailand, 12 China) exactly as stated.
- **Money inputs on the case** (D43): `POST /v1/cases/predict` takes optional `economics`, and `PUT /v1/cases/{id}/economics` edits them later with no model calls. The edit is recorded in the history. Plan, scenarios and compare-targets fall back to the case's economics. The plan search didn't fall back before, so economics given at predict were ignored. Rates must be fractions 0–1 (catches "41.7" typed for 41.7%).
- **Rate limits, pinned demos, delete** (D45): model-calling endpoints are limited per client (20/min light, 4/min plan and compare-targets, plus a global cap), with a 429 and `Retry-After`. `ADMIN_TOKEN` (`X-Admin-Token`, since `Authorization` carries Cloud Run's identity token) bypasses them and is needed to pin, delete a pinned case or run the batch watch. `DELETE /v1/cases/{id}` and `PUT /v1/cases/{id}/pin`; pinned cases list first. The seed pins its cases and sends `ID_TOKEN` for a private Cloud Run service.
- **Cost on every step** (D45): history events carry tokens, estimated cost and latency. Heat-gun predict with clarify: $0.0188, 13.6 s.
- **Which conflicting ruling the case leans to** (D46, clarify-v2): `clarify.conflict.leans` is "a", "b" or null, for the Result screen's conflict panel. Cabin filter N351082 leans "b" (the filter-media ruling), matching the engine's reasoning.
- **Deep health check** (D47): `GET /health?deep=1` writes, reads and deletes a probe case and makes one model call; 503 with the failing check's error.
- **Portfolio triage** (D47): `POST /v1/cases/batch` for up to 20 products (JSON or CSV/xlsx), each saved as a case, with a per-row error and a summary by route. Sample `backend/testdata/portfolio_sample.csv`: 2 auto, 1 needs info, 2 review (one out-of-scope textile), 15.9 s, $0.068.
- **Rename a case** (D48): `PATCH /v1/cases/{id}` sets `display_name`; `product_name` stays as entered because the engine read it.
- **BOM import:** `plant` rows for plants that make nothing yet. New LiDAR sample BOM (`backend/testdata/lidar_bom.csv`), which gives the same planner moves as the LiDAR demo.

## Live checks (LiteLLM proxy, development only)
- Two seed runs, both recorded in `docs/DEMO.md`; a repeated heat-gun search gave identical verdicts in both (6/6 from cache).
  - Run 1: no heat-gun or LiDAR plan cleared 0.92; closest checks 0.75.
  - Run 2 (`eval/results/demo-seed-litellm.json` and `demo-seed-heatgun-*-litellm.json`): heat-gun motor-fan + PCBA in Thailand shown at exactly 0.92, net $320k/yr; LiDAR closest 0.90.
  - The disagreement is the run-to-run variation of the precedent check (D31). The recording-day Vertex run decides which screen the video shows.
- Rate limits, pin, delete and the admin-only batch watch checked against a running server (429 on the third predict in a minute with a limit of 2).

## Checks
`make test lint` passes, including `-race` on the handlers. New tests cover economics, the rate limiter (per client, admin bypass, global weights), delete and pin, history cost, conflict leans, batch triage (JSON, CSV, out of scope, per-row errors), deep health and rename.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

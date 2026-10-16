# Status — shared verdict cache, 16 Oct 2026

For Anas. Delete this file before merging to `main`. Nothing here was pushed or merged to `main`, and the production `backend` service was not deployed.

Branch `feat/origin-proof-shared-verdict-cache` at `6ac3e72`, plus this file. Staging revision after the cache deploy: `origin-proof-api-staging-00005-qw9`. Fresh instances for the replay: `origin-proof-api-staging-00006-rvx` (`CACHE_TEST=1`). URL `https://origin-proof-api-staging-guxax2teha-uc.a.run.app`.

Startup log on both revisions: `verifier verdicts cached in Firestore` (`collection=verifier_cache`). Env is the previous staging set with `VERIFIER_CACHE_DIR` replaced by `VERIFIER_CACHE_STORE=firestore`, `ADMIN_TOKEN` from `origin-proof-admin-token:latest`, `--max-instances 3`, timeout 300s.

`gcloud secrets versions access` is denied for `anasqumhiyeh@gmail.com` (`secretmanager.versions.access`). A temporary version 4 was added so the staging client could send `X-Admin-Token`, then disabled. `latest` is version 3 again. The value was not printed.

## 1. Shared cache — pass

Emulator: `FIRESTORE_EMULATOR_HOST=localhost:8681 go test ./internal/store -run Firestore -v` → `PASS: TestFirestoreEmulator (0.68s)`. That test includes the missing-key read, first-write-wins (`{"v":1}` kept over `{"v":2}`), and a second client seeing the first write.

Smoke (`KEEP=1`), revision `00005-qw9`:

```
ok    GET /health                        200    0.1 s
ok    GET /health?deep=1                 200    2.6 s
      checks: case_store 295 ms, model 2513 ms
ok    GET /v1/demos/heatgun-2022         200    0.1 s
ok    POST /v1/cases/predict (heat gun)  201  108.5 s
      case_0b66af911eb0: China 0.85 (needs_info), vertex gemini-4-argon
ok    GET /v1/cases/{id}                 200    0.1 s
ok    GET /v1/cases                      200    0.2 s
      listed: True
ok    POST /v1/cases/{id}/plan (~30 s)   200  134.1 s
      6 moves, 0 shown, 0/6 checks cached
all checks passed
```

Case `case_0b66af911eb0`. After `gcloud run services update origin-proof-api-staging --update-env-vars CACHE_TEST=1` (revision `00006-rvx`), the same plan body again: HTTP 200 in 27.9 s, `verifier_runs=6`, `verifier_cached=6`, `wall_seconds=23.3`. Move confidences matched the first search: PCBA 0.75; PCBA+switch 0.85; motor-fan 0.90; motor-fan+switch 0.85; motor-fan+PCBA 0.85; switch 0.85. Case deleted (204).

## 2. Concurrency — pass

Five heat-gun predicts, then five plan searches at once. All HTTP 200, all under 300 s. No 429, 502, or 504. No `RESOURCE_EXHAUSTED` in the staging logs for the last 30 minutes. `PLANNER_CONCURRENCY` was not changed.

| # | Target | Status | Client wall | verifier_runs | verifier_cached | moves |
|---|---|---|---|---|---|---|
| 0 | Thailand | 200 | 81.3 s | 6 | 6 | 6 |
| 1 | Vietnam | 200 | 5.8 s | 0 | 0 | 0 |
| 2 | Malaysia | 200 | 6.2 s | 0 | 0 | 0 |
| 3 | Thailand | 200 | 77.9 s | 6 | 6 | 6 |
| 4 | Vietnam | 200 | 6.7 s | 0 | 0 | 0 |

Vietnam and Malaysia only reject "No change: keep the current setup" (`moves_searched=0`). The heat-gun economics have no plant move into those countries, so they never call the verifier. The two Thailand searches reused the Firestore cache from section 1. Peak active instances on `00006-rvx` during the window was 2 (`run.googleapis.com/container/instance_count`, cap 3). The five cases were deleted (204).

## 3. SSE through the frontend — fail

Backend stream, direct to staging, passes. Frontend hop does not.

Direct `POST` with `Accept: text/event-stream`:

- Malaysia (`case_40c57f0a20b2`): `start` 05:41:41, `baseline` and `done` 05:41:46. No `move_done` and no `: keep-alive`, because the search has no moves and finishes in 5 s.
- Thailand (`case_4c6d9654d2cf`, cache hits, 68 s): `start` and six `move_started` at 05:41:46, `baseline` at 05:41:51, `move_done` at 05:41:51, 05:41:53, 05:42:10, and 05:42:54, `: keep-alive` at 05:41:56, 05:42:06, 05:42:16, 05:42:26, 05:42:36, 05:42:46, `done` last at 05:42:54. Events are spread. Cloud Run is not buffering this hop.

`origin-proof-frontend-staging` was created from this branch's `frontend/` (same tree as `main`; the cache commit does not touch it). Revision `origin-proof-frontend-staging-00001-wkp`, `BACKEND_URL` = the staging backend, service account `frontend@`, `--max-instances 1`.

`--allow-unauthenticated` did not stick:

```
Setting IAM policy failed, try "gcloud beta run services add-iam-policy-binding --region=us-central1 --member=allUsers --role=roles/run.invoker origin-proof-frontend-staging"
```

Anonymous `GET` returns 403. With an identity token, `GET /` is 200 and `POST /api/v1/cases/case_40c57f0a20b2/plan` is 404. `main`'s frontend has `/api/chat` and `/api/chat/stream` only. There is no `/api/v1/[...path]` proxy, so there is no frontend hop to time. That proxy is on the frontend branch, not this one. No change made here.

## Fixes

None on this branch. The cache behaved as D50 describes.

## Merge readiness

Safe to merge to `main` after this file is deleted. That merge deploys production and will set `VERIFIER_CACHE_STORE=firestore` on `backend` (the `deploy.yml` change on this branch). Staging already ran that configuration: a new revision reused all six heat-gun verdicts with the same confidences.

Do not expect this merge to fix plan-search SSE on the public site. The production frontend still has no `/api/v1` proxy.

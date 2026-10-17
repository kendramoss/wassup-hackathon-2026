# Staging checks before the freeze (17 Oct 2026, 14:00 SGT)

All backend PRs are on `main`. There is one new branch to verify: `feat/origin-proof-shared-verdict-cache` (`6ac3e72`, D50). On Cloud Run the verdict cache was in `/tmp`, so each instance had its own and a restart lost it. A replayed plan search could therefore verify afresh on another instance. This branch moves the cache to the Firestore collection `verifier_cache` (`VERIFIER_CACHE_STORE=firestore`, which is the default whenever cases are in Firestore). `deploy.yml` on that branch now sets `VERIFIER_CACHE_STORE=firestore`.

**Rules:**
- Staging only. Never push to `main`, merge, or deploy the production `backend` service.
- No credentials in git or in text. Report exact errors.
- Commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## 1. Verify the shared cache (~20 min)
1. Run the emulator test on the branch:
   ```bash
   cd backend && FIRESTORE_EMULATOR_HOST=... go test ./internal/store -run Firestore -v
   ```
   It now also covers verdicts, including first-write-wins across two clients.
2. Deploy `origin-proof-api-staging` from the branch:
   - env: same as before, but replace `VERIFIER_CACHE_DIR=/tmp/verifier-cache` with `VERIFIER_CACHE_STORE=firestore`
   - keep `--set-secrets ADMIN_TOKEN=origin-proof-admin-token:latest`
   - `--max-instances 3`
   - startup log must say `verifier verdicts cached in Firestore`
3. Run `scripts/smoke.sh <staging URL>` with `ID_TOKEN` and `ADMIN_TOKEN` set and `KEEP=1`. Note the case ID.
4. Force a new revision so you get fresh instances with nothing in memory:
   ```bash
   gcloud run services update origin-proof-api-staging --update-env-vars CACHE_TEST=1
   ```
5. Run the same plan search on that case again (`POST /v1/cases/{id}/plan` with the heat-gun demo's plan body). **Pass:** `stats.verifier_cached == stats.verifier_runs`, and every move's verdict confidence is identical to step 3. Delete the case afterwards.

## 2. Concurrency (~15 min)
- Create 5 heat-gun cases through `POST /v1/cases/predict` (admin token, so you aren't rate-limited).
- Fire 5 plan searches at once, one per case, with different targets: Thailand, Vietnam, Malaysia, Thailand, Vietnam.
- Use 5 `curl` commands in the background and `wait`.
- **Pass:** all return 200 within the 300 s request timeout.
- **Report** for each search:
  - status
  - wall time
  - `stats.verifier_runs` and `stats.verifier_cached`
  - any 429, 502 `model_unavailable` or 504
- From the logs, report any Vertex quota error (`RESOURCE_EXHAUSTED` or 429 from Vertex). If there are any:
  - try `PLANNER_CONCURRENCY=4` and report whether that clears it
  - check the instance count during the burst (Cloud Run metrics, or `gcloud run services describe`)
- Delete the cases afterwards.

## 3. SSE through the frontend proxy (~10 min)
- The frontend proxies `/api/v1/*` to the backend. If no staging frontend exists, deploy one as `origin-proof-frontend-staging`:
  - source: `main`'s `frontend/`
  - `BACKEND_URL` = the staging backend
  - `--allow-unauthenticated`, `--max-instances 1`
  - the `frontend@` service account (it can invoke the backend)
- Through the frontend URL, time the event arrivals:
  ```bash
  curl -N -H 'Accept: text/event-stream' -H 'Content-Type: application/json' -d @plan.json <frontend URL>/api/v1/cases/<id>/plan | while IFS= read -r l; do printf '%s %s\n' "$(date +%T)" "$l"; done
  ```
  - Use a case created through `<frontend URL>/api/v1/cases/predict`.
  - Use a target with uncached moves (e.g. Malaysia), so the search takes ~30 s.
- **Pass:**
  - `event: start` and `baseline` arrive within a few seconds
  - `move_done` events arrive spread over the search
  - `: keep-alive` lines arrive every ~10 s
  - `done` comes last
- **Fail:** everything arrives at once at the end. Those are buffered responses. Report which hop buffers (frontend route handler vs Cloud Run). Compare with the same call made directly to the backend.

## Report back
Write `STATUS.md` on `feat/origin-proof-shared-verdict-cache` (Anas deletes it before merging) with:
- **Pass/fail** for each of 1–3, with numbers.
- **Errors:** the exact error for anything that failed.
- **Fixes:** any code change you think is needed, on that same branch, with tests passing:
  ```bash
  cd backend && go test -race ./... && go vet ./... && staticcheck ./...
  ```
- **Merge readiness:** whether the branch is safe to merge to `main`, which deploys production.

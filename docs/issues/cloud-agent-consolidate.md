# Consolidate everything into kendramoss/wassup-hackathon-2026 (code freeze 17 Oct 2026, 14:00 SGT)

You now have access to the main repo, `kendramoss/wassup-hackathon-2026`. Origin Proof is spread across stacked PRs in the main repo, your fork's branches and Kendra's newer branches. Bring it all into the main repo as reviewable PRs, ready for one final merge to `main`. Kendra has granted `roles/datastore.user` to `agent-backend`, so Firestore saves should work now.

## Rules
- **Every push to `main` deploys production** (`.github/workflows/deploy.yml`). Never push or merge to `main` yourself. Do everything on feature branches; then stop and report to Anas. He approves the final merge.
- Never force-push a branch someone else authored (Kendra = `kmthere`). Make a new branch instead and comment on her PR.
- Merge stacked PRs with **"Create a merge commit"**. Never squash or rebase a branch in the stack; that breaks every PR above it.
- No credentials in git or in PR text. `ADMIN_TOKEN` goes in Secret Manager only.
- Don't re-run held-out evals. Don't launch tuning (D44 stays a finals item).
- Commit messages end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Report what you couldn't do, with the exact error.

## 0. Inventory first (~10 min)
Run `gh pr list --state open --json number,title,headRefName,baseRefName,author` and check it against the map below. If it disagrees, report the difference before changing anything.

**Backend stack** (each branch contains the one before it; I verified this with `git merge-base --is-ancestor`):

| # | Branch | Author | Base |
|---|---|---|---|
| 1 | `feat/origin-proof-skeleton` | Anas | `main` |
| 2 | `feat/origin-proof-engine` | Anas | 1 |
| 3 | `feat/origin-proof-planner` | Anas | 2 |
| 4 | `feat/origin-proof-api` | Anas | 3 |
| 5 | `feat/origin-proof-accuracy` | Anas | 4 |
| 6 | `feat/origin-proof-compare-agent` | Anas | 5 |
| 7 | `feat/origin-proof-gcp` (#37) | Kendra | 6 |
| 8 | `feat/origin-proof-tuning` (#38) | Kendra | 7 |
| 9 | `feat/origin-proof-demo` | Anas | 8 |

- PR 9 also contains your own `feat/origin-proof-vertex-eval` (Vertex numbers, D39). That branch is already merged into the stack and needs no separate PR.
- `feat/origin-proof-demo` (head `5b2cee4`) is the newest backend: decisions D40–D48, `docs/api/openapi.yaml` and `scripts/smoke.sh`.

**Outside the stack:**
- Your fork's `feat/origin-proof-flash-staging`: commits `76b1720` (untuned Flash baseline, D44) and `1ec1191` (staging deploy notes, `STATUS.md`). They were stacked on an older `feat/origin-proof-demo`.
- Kendra's `feat/origin-proof-vertex` (`59f96b4`): removes the LiteLLM proxy so Vertex is the only provider. It sits on top of `feat/origin-proof-tuning`, not on demo, and records itself as **D39**, which clashes with your D39 (Vertex numbers).
- Kendra's `feat/origin-proof-frontend` (#44, based on `main`): the real Next.js app (Cases home, product, origin + ask, scenarios, plan stream, compare, draft, accuracy) plus design docs.
- Design-doc branches, all `docs/design` only and overlapping:
  - `feat/origin-proof-frontend-design` (Anas's frontend session, newest frames up to portfolio triage, `4e660aa`)
  - `feat/origin-proof-design-compare-ask` (#46/#53)
  - `feat/origin-proof-palettes` (Kendra)

## 1. Bring the Flash baseline onto demo (~10 min)
- From `feat/origin-proof-demo`, make branch `feat/origin-proof-flash-baseline` and cherry-pick `76b1720` and `1ec1191`.
- D44 is already the right number.
- Fold anything in `STATUS.md` worth keeping into `eval/RESULTS.md` or D44, then delete `STATUS.md`; it shouldn't land on `main`.
- Open the PR with base `feat/origin-proof-demo`.

## 2. Vertex-only, rebased onto the newest backend (~30–45 min)
- Make branch `feat/origin-proof-vertex-only` from `feat/origin-proof-flash-baseline`. Re-apply Kendra's `59f96b4` there (cherry-pick, then resolve conflicts).
- **Renumber her decision from D39 to D49.** D39 is taken by the Vertex numbers, and D40–D48 already exist.
- Code written after her branch still references LiteLLM, so finish the removal there:
  - `backend/internal/config/config.go` and its test (now include rate limits and `ADMIN_TOKEN`; keep those)
  - `backend/cmd/eval/main.go`, `torn.go`, `summary_test.go`
  - `backend/internal/extract/live_test.go`
  - `backend/internal/evalkit/report.go`
  - `Makefile`, `README.md`
  - `docs/DEMO.md`: `make run` and `make demo-seed` should describe Vertex, not the proxy
  - Recorded results labelled "litellm" / "proxy, development only" stay exactly as they are; they are history.
- `cd backend && go test -race ./... && go vet ./... && staticcheck ./...` must pass.
- Open the PR with base `feat/origin-proof-flash-baseline`. Comment on Kendra's original PR that it's superseded by this one (rebased onto the newest backend, D49). Leave hers open for her to close.
- **Note for Anas in your report:** after this lands, local development on his laptop needs Vertex credentials. He avoided logging in to gcloud there, so flag it; don't work around it.

## 3. Frontend fixes against the newest backend API (~45 min)
Make branch `feat/origin-proof-frontend-api` from Kendra's `feat/origin-proof-frontend` and open a PR with base `feat/origin-proof-frontend`. Use the contract in `docs/api/openapi.yaml` on `feat/origin-proof-demo`.

**Proxy** (`frontend/src/app/api/v1/[...path]/route.ts` and `frontend/src/lib/backend.ts`):
- It only forwards GET and POST, and `callBackend` always sends POST. Forward PUT, PATCH and DELETE too, with the right method. These are used by `PUT /v1/cases/{id}/economics`, `PATCH /v1/cases/{id}` (rename), `DELETE /v1/cases/{id}` and `PUT /v1/cases/{id}/pin`.
- Set `X-Client-IP` on every backend request to the browser's address: the rightmost entry of the incoming `X-Forwarded-For` on Cloud Run. Without it every visitor shares one rate-limit bucket (D45).
- 429 already passes through as a user error. Keep the `Retry-After` header and `details.retry_after_s`.
- A 204 from DELETE has no body; don't try to parse one.
- Keep SSE streaming for `POST /v1/cases/{id}/plan` with `Accept: text/event-stream`.

**Types:** regenerate `frontend/src/lib/api/schema.d.ts` from the newest `openapi.yaml`. New since #44:
- `display_name`, `pinned`, `history[].cost_usd`/`latency_ms`/tokens
- `clarify.conflict.leans`
- `POST /v1/cases/batch`, `GET /health?deep=1`

Wire the cheap ones if the screens exist:
- show `display_name` on Cases home
- a "Demo" tag on pinned cases, with no delete on them
- the conflict lean marker

Report the rest; don't build new screens. Anas's frontend session is adding the newest frames.

Checks: `cd frontend && npm ci && npm run build`, and the lint script if there is one.

## 4. One set of design docs (~20 min)
- Make branch `feat/origin-proof-design-docs` from `main`. Merge, in this order: `feat/origin-proof-design-compare-ask`, `feat/origin-proof-palettes`, `feat/origin-proof-frontend-design`.
- On a conflict in `docs/design`, keep the newest version of each file by commit date, and list every conflicted file in the PR.
- No code changes here. Open the PR with base `main`.
- Note: #44 also adds `docs/design`. Report any file that differs between #44 and this branch; don't resolve it yourself.

## 5. Staging deploy of the integrated backend (~20 min)
1. **`ADMIN_TOKEN`:** create the Secret Manager secret `origin-proof-admin-token` with a random value of 32+ characters. Grant `agent-backend` `roles/secretmanager.secretAccessor` on that secret only. Never print the value.
2. **Deploy staging:** redeploy `origin-proof-api-staging` from `feat/origin-proof-vertex-only`, with the same env as before plus `--set-secrets ADMIN_TOKEN=origin-proof-admin-token:latest`.
3. **Run the smoke test:**
   ```bash
   ID_TOKEN=$(gcloud auth print-identity-token) ADMIN_TOKEN=$(gcloud secrets versions access latest --secret origin-proof-admin-token) scripts/smoke.sh <staging URL>
   ```
   It must end with "all checks passed": deep health (store and model), predict, list, one plan search. Paste its output in your report.
4. **Seed the demos:** with the same two env vars, `cd backend && go run ./cmd/demo seed -api <staging URL> -dump ../eval/results -out ../eval/results/demo-seed-vertex.json`. Then rename the three dump files to `demo-seed-heatgun-{predict,plan,scenarios}-vertex.json`. Commit them to `feat/origin-proof-vertex-only` and fill in the Vertex row in `docs/DEMO.md` section 4. Seed **once**. Don't re-seed to get a better plan verdict (DEMO.md section 2).
5. **If anything fails** (permissions, timeouts, SSE buffering), report the exact error. Don't switch the store or the provider.

## 6. Production deploy settings, prepared on a branch (~10 min)
`.github/workflows/deploy.yml` deploys the backend with `gcloud run deploy backend`. On `feat/origin-proof-vertex-only`, add `--set-secrets ADMIN_TOKEN=origin-proof-admin-token:latest` to that command, then check what else production needs:
- Firestore: `CASES_STORE=firestore` is the default on Cloud Run
- `VERIFIER_CACHE_DIR=/tmp/verifier-cache`
- a request timeout of at least 300 s for plan search and SSE

Don't deploy production.

## 7. Report back, then stop
Before the final merge, give Anas:
- **PR links** in merge order:
  1. Backend: merge top-down so `main` receives one push.
     - `feat/origin-proof-vertex-only` → `feat/origin-proof-flash-baseline` → `feat/origin-proof-demo`
     - then PR 9 → 8 → … → 2, each into its base
     - then PR 1 → `main` last
  2. Then the design docs PR → `main`.
  3. Then #44 with `feat/origin-proof-frontend-api` merged into it → `main`.

  Say which merges need Kendra's approval.
- **Results:** the staging smoke-test output and the seed summary.
- **Your report as `STATUS.md`** on `feat/origin-proof-vertex-only`: done, numbers, blocked, next. Anas reads it there and deletes it before the final merge.
- **Open items:** anything blocked or unresolved, conflicts you weren't sure about, and the gcloud note from step 2.

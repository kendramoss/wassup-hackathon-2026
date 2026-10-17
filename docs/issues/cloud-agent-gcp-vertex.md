# Brief: finish the GCP setup and produce the official Vertex AI numbers for Origin Proof

Repo: `kendramoss/wassup-hackathon-2026`. GCP project `wassup-hackathon-2026`, region `us-central1`. The backend runs on Cloud Run as service account `agent-backend@wassup-hackathon-2026.iam.gserviceaccount.com`. Go backend in `backend/`.

## Context
- **Branch stack:** `main` → `feat/origin-proof-skeleton` → `-engine` → `-planner` → `-api` → `-accuracy` → `-compare-agent` → `-gcp` → `-tuning`. Work from **`feat/origin-proof-tuning`**, which contains everything.
- **Already done by Kendra (D37 in `docs/DECISIONS.md`):** Firestore, Storage and Vertex AI APIs enabled; Firestore `(default)` database created; corpus bucket `gs://wassup-hackathon-2026-origin-proof-corpus` created and filled. `docs/SETUP-GCP.md` steps 1–3 are done.
- **Where the numbers stand:** every accuracy number so far came through a LiteLLM proxy and is labelled "proxy, development only". The README results table shows *pending the Vertex AI re-run*. Your job is to replace that with official Vertex numbers.

## Rules
- Never commit or print credentials, keys or tokens. No `.env` files in git.
- **Held-out split (`final`, 377 rulings): run it once, with the frozen config.** Never change prompts, thresholds or settings after looking at held-out results. The frozen config is engine-v1, BM25 retrieval, K = 5, no `-rules`, no `-reasoning`, no `-votes`.
- Don't push to `main` and don't deploy. Commit to a new branch stacked on `feat/origin-proof-tuning`, named `feat/origin-proof-vertex-eval`. Commit as Mizore66 <anasqumhiyeh@gmail.com>, signed if that key is available to you.
- Report anything you couldn't do; don't work around a permission you lack.

## 1. IAM (needs project owner or IAM admin)
```bash
PROJECT=wassup-hackathon-2026
SA=agent-backend@$PROJECT.iam.gserviceaccount.com
gcloud projects add-iam-policy-binding "$PROJECT" --member "serviceAccount:$SA" --role roles/datastore.user --condition=None
gcloud storage buckets add-iam-policy-binding "gs://$PROJECT-origin-proof-corpus" --member "serviceAccount:$SA" --role roles/storage.objectViewer
# Vertex for the service: grant if the check below doesn't list roles/aiplatform.user
gcloud projects add-iam-policy-binding "$PROJECT" --member "serviceAccount:$SA" --role roles/aiplatform.user --condition=None
gcloud projects get-iam-policy "$PROJECT" --flatten "bindings[].members" \
  --filter "bindings.members:serviceAccount:$SA" --format "value(bindings.role)"
```
- **Done when:** the last command lists `roles/datastore.user` and `roles/aiplatform.user`, and the bucket grant succeeded.
- Then update the status line in `docs/SETUP-GCP.md` to say steps 4–5 are done.

## 2. Vertex smoke test
```bash
cd backend
export LLM_PROVIDER=vertex GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 GOOGLE_CLOUD_LOCATION=global
go run ./cmd/server -smoke
```
- **Done when:** one engine-model call succeeds.
- If `gemini-4-argon` or `gemini-3.8-flash` is not available on Vertex in this project, stop and report the exact error and the model IDs Vertex offers. Don't substitute models.

## 3. Official runs (same env as step 2, from `backend/`)
The local corpus isn't in git; use the bundled one with `-facts data/facts -labels ../eval/data/labels.jsonl`.
```bash
# Dev split (469), engine only. Resumes if interrupted.
go run ./cmd/eval run -split dev -facts data/facts
# Router labels for the fast path (router-v2 on Vertex); final is labelled once
go run ./cmd/eval scope -split dev -facts data/facts -out ../eval/results/scope-dev-router-v2-vertex.jsonl
go run ./cmd/eval scope -split final -confirm-final -facts data/facts -out ../eval/results/scope-final-router-v2-vertex.jsonl
# Held-out (377): ONCE, frozen config
go run ./cmd/eval run -split final -confirm-final -facts data/facts
# Reports with the scoring adjustments
go run ./cmd/eval report -in ../eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl -scope ../eval/results/scope-dev-router-v2-vertex.jsonl -adjust ../eval/data/scoring_adjustments.json
go run ./cmd/eval report -in ../eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl -scope ../eval/results/scope-final-router-v2-vertex.jsonl -adjust ../eval/data/scoring_adjustments.json
go run ./cmd/eval compare -a ../eval/results/final-gemini-4-argon-prec-engine-v1-litellm.jsonl -b ../eval/results/final-gemini-4-argon-prec-engine-v1-vertex.jsonl -adjust ../eval/data/scoring_adjustments.json
```
- Check the result filenames the runs actually wrote and use those.
- The fast path is confidence ≥ 0.92 and in scope (`config.DefaultFastPath`).

## 4. Results page and README
- `go run ./cmd/eval summary` builds `backend/internal/evalsummary/summary.json` (served by `GET /v1/eval/summary`). It currently hard-codes the `*-litellm.jsonl` file names (`summaryCmd` in `backend/cmd/eval/main.go`). Make it use the `*-vertex.jsonl` files and Vertex scope files when they exist, and mark those items official rather than "proxy, development only". Keep the proxy fallback and add a test. Then regenerate.
- In `README.md`, fill the *pending* rows: engine accuracy and fast path (coverage and accuracy, 95% CI), held-out, Vertex AI. Keep the proxy dev numbers labelled as proxy.
- Add the Vertex results to `eval/RESULTS.md`, and a decision entry (next D number) in `docs/DECISIONS.md` with the numbers, the McNemar comparison against the proxy run, cost per case and latency.

## 5. Firestore check (optional, if Java is available)
```bash
gcloud emulators firestore start --host-port=localhost:8085 &
FIRESTORE_EMULATOR_HOST=localhost:8085 go test ./internal/store/ -run Firestore -v
```
- Report pass or fail; it hasn't been run yet (D37).

## Checks before you push
- `make test lint` from the repo root passes.
- Commit result files under `eval/results/`, the regenerated `summary.json`, README, RESULTS and DECISIONS.
- Push `feat/origin-proof-vertex-eval`.

## Report back
- The IAM roles now on the service account.
- Smoke-test outcome.
- Dev and held-out accuracy (95% CI), fast-path coverage and accuracy, the McNemar result against the proxy run, cost per case and p50/p95 latency.
- Anything skipped and why. Branch name and commit.

# Next tasks (code freeze 17 Oct 2026, 14:00 SGT)

Good work on the Vertex numbers; they're in the real repo now. Gemini 4 Argon can't be tuned on Vertex. Tunable models: Gemini 3.5 Flash, 3.1 Flash-Lite, 2.5 Pro, 2.5 Flash and 2.5 Flash-Lite.

Base new work on **`feat/origin-proof-demo`**, which sits on top of your `feat/origin-proof-vertex-eval`. It adds:
- `make demo-seed` and `docs/DEMO.md` (D40)
- the case audit trail (D41)
- `POST /v1/products/extract` (D42)

Same rules as before: no credentials in git, held-out at most once per frozen config, no push to `main`, and report what you couldn't do.

## 1. Is tuning worth it? An untuned Gemini 3.5 Flash dev baseline (do first, ~15 min, ~$2)
```bash
cd backend
export LLM_PROVIDER=vertex GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 GOOGLE_CLOUD_LOCATION=global
ENGINE_MODEL=<Vertex ID for Gemini 3.5 Flash> go run ./cmd/eval run -split dev -facts data/facts
go run ./cmd/eval compare -a ../eval/results/dev-gemini-4-argon-prec-engine-v1-vertex.jsonl -b <the new file> -adjust ../eval/data/scoring_adjustments.json
```
- Report accuracy (95% CI), fast-path accuracy, cost and latency against Argon's 91.1% dev.
- **Decision rule:** if untuned Flash is within ~3 points of Argon on dev, tuning could plausibly reach Argon. If it's further below, don't tune before the freeze; record it as a finals item (a new D entry). Dev only; don't run held-out.

## 2. Staging deploy (ask Anas before deploying; it creates a billable service)
- Build and deploy the `feat/origin-proof-demo` backend to a **separate** Cloud Run service, `origin-proof-api-staging`. Never the production service. Use the same image build and env as `.github/workflows/deploy.yml`: `LLM_PROVIDER=vertex`, `CASES_STORE=firestore`, `VERIFIER_CACHE_DIR=/tmp/verifier-cache`, service account `agent-backend`.
- Check against the staging URL:
  - `/health`
  - `GET /v1/demos`
  - `POST /v1/cases/predict` with the heat-gun demo body
  - `GET /v1/cases`
  - one plan search (`POST /v1/cases/{id}/plan` with the demo's plan body)
- Then `go run ./cmd/demo seed -api <staging URL>`.
- **If case saves fail:** `agent-backend` still lacks `roles/datastore.user` (only a project owner can grant it). Report the exact error and move on. Don't switch the store.
- **Report:** what worked, latencies, any Cloud Run-only failure (permissions, read-only filesystem, timeouts on the ~30 s plan search or SSE streaming), and the seed summary.

## 3. Only if step 1 says tuning is worth it: tuning data and job (ask Anas before launching; paid)
Follow D38 (on `feat/origin-proof-tuning`): engine runs on `train` and `tune_check` with `-splits ../eval/splits/splits_tune.json`, then `eval tunedata`, then `scripts/tune-vertex.sh` with `BASE_MODEL` set to Gemini 3.5 Flash. Check the price in the Vertex tuning docs first and quote it to Anas. When the job finishes: a dev run with the tuned endpoint plus `compare` against Argon; held-out only if dev improves.

## Push and report
- Push to a new branch on the fork, `feat/origin-proof-flash-staging`, stacked on `feat/origin-proof-demo`.
- Keep a `STATUS.md` at the repo root of that branch: done / numbers / blocked / next. Anas syncs it into the real repo.

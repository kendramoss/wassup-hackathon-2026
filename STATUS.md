# Status — consolidation, 16 Oct 2026

For Anas. Delete this file before the final merge to `main`. Nothing here was pushed or merged to `main`.

## Done

- Inventory matched the backend stack (ancestry checked with `git merge-base --is-ancestor`). Actual GitHub PR numbers, which the map gives as stack position or issue number: #34 skeleton, #35 engine, #41 planner, #40 api, #43 accuracy, #45 compare-agent, #47 gcp (issue #37), #48 tuning (issue #38), #50 demo. Authors match (Mizore66 / kendramoss).
- `feat/origin-proof-flash-baseline` is `feat/origin-proof-demo` plus cherry-picks `76b1720` and `1ec1191`. D44 kept that number and now includes the staging note from `STATUS.md`. That file is not on the branch tip.
- `feat/origin-proof-vertex-only` is the flash baseline plus Kendra's LiteLLM removal, renumbered **D39 → D49**. Rate limits and `ADMIN_TOKEN` kept. Recorded `*-litellm.*` results and the "proxy, development only" labels were not rewritten. Live call sites after her branch (`extract` live test, `make run` / `make demo-seed` in `docs/DEMO.md`) now say Vertex.
- `cd backend && go test -race ./...` passed. `go vet ./...` passed. `staticcheck ./...` passed when run as `GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...`. The same `go run` under the default toolchain switched to Go 1.26.9 and failed to compile (see Blocked).
- `feat/origin-proof-frontend-api` is Kendra's `feat/origin-proof-frontend` plus the proxy/type fixes. `npm ci`, `npm run build`, and `npm run lint` passed.
- `feat/origin-proof-design-docs` is `main` plus merge commits of `feat/origin-proof-design-compare-ask`, then `feat/origin-proof-palettes`, then `feat/origin-proof-frontend-design`. Only conflict: `docs/design/DESIGN.md`. Kept the newer file, `4e660aa` (2026-10-16 04:51 +0800, portfolio triage) over `09efec8` (2026-10-16 01:30 +0800). No code in that branch.
- Production workflow on this branch only: `deploy.yml` now passes `--set-secrets ADMIN_TOKEN=origin-proof-admin-token:latest` and `--timeout=300`. `CASES_STORE=firestore` and `VERIFIER_CACHE_DIR=/tmp/verifier-cache` were already there. Production was not deployed.

## Numbers (not re-run)

Untuned Gemini 3.5 Flash, dev only, adjustments applied, n = 463 (D44): 90.5% (87.5–92.8) vs Argon 91.1% (88.2–93.4). McNemar p = 0.736. Fast path 91.6% of cases at 92.0%. Latency p50 12.1 s / p95 51.2 s. Tuning was not launched.

Official held-out Vertex engine (D39, already on demo): 90.2% (86.8–92.8). Not re-scored.

## Blocked

- **GitHub write.** `git push` of these four branches failed: `fatal: could not read Username for 'https://github.com': terminal prompts disabled`. `gh` is not logged in, `GH_TOKEN` is unset, and `https://api.github.com/user` with the session token returns 401 Bad credentials. No PRs were opened. Kendra's #52 was not commented on. Her branch `feat/origin-proof-vertex` was not force-pushed.
- **Staging.** `gcloud: command not found`. No `~/.config/gcloud` and no Application Default Credentials file. `http://metadata.google.internal` failed with `Could not resolve host`. Secret `origin-proof-admin-token` was not created, `origin-proof-api-staging` was not redeployed, `scripts/smoke.sh` was not run, and the demo was not seeded. No Vertex row was filled in `docs/DEMO.md` section 4.
- **staticcheck toolchain.** First attempt: `go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...` downloaded Go 1.26.9 and exited 1 with `unknown field rfd in struct literal of type splicePipe` and `file requires newer Go version go1.27 (application built with go1.26)`. Re-run pinned to Go 1.27.0 exited 0 with no findings.

## Inventory differences (did not change the stack)

- Frontend PR #51 (`feat/origin-proof-frontend`, issue #44) has base `feat/origin-proof-design-compare-ask`, not `main`. Git says `main` is an ancestor and the design branch is not. Left #51's base alone.
- Palette PR #53 was already squash-merged into `feat/origin-proof-design-compare-ask` (`244d3c6`). The original branch `502eae2` is not an ancestor of that commit. Merging it again onto the design-docs branch was clean.
- Flash commits `76b1720` and `1ec1191` are not on a branch tip. They are reachable from the public repo by SHA. `Mizore66/wassup-hackathon-2026` still returns 404 without a GitHub login.

## Files that differ between this design branch and #44

Content differs (17): `docs/design/DESIGN.md`, `keyframes/p-ask-waiting.html`, `p-ask.html`, `p-bom-empty.html`, `p-bom-ok.html`, `p-bom-warn.html`, `p-cases.html`, `p-plan-q1.html`, `p-scen.html`, and the matching `png/` files for those eight keyframes except DESIGN.md. 151 other `docs/design` files are on this branch and not on #44 (palette frames and the later keyframes). Nothing under `docs/design` is only on #44. Not resolved.

## Not built

Proxy now accepts PUT, PATCH, and DELETE, but no screen calls rename, economics, pin, delete, `POST /v1/cases/batch`, or `GET /health?deep=1`. There is no delete control on Cases home, so pinned cases have a Demo tag and nothing that deletes them. History cost, latency, and token fields are in the generated types only.

## Next

1. Push the four branches and open the PRs below. Comment on #52 that `feat/origin-proof-vertex-only` supersedes it (rebased onto the newest backend, D49) and leave #52 open for Kendra.
2. Whoever has gcloud and Secret Manager access: create `origin-proof-admin-token` (32+ characters, do not print it), grant `agent-backend` `roles/secretmanager.secretAccessor` on that secret only, redeploy `origin-proof-api-staging` from this branch with the existing env plus `--set-secrets ADMIN_TOKEN=origin-proof-admin-token:latest`, run `scripts/smoke.sh`, seed once, and fill the Vertex row in `docs/DEMO.md` section 4.
3. **Laptop note:** after D49, local `make run` needs Vertex Application Default Credentials (`gcloud auth application-default login`) and `GOOGLE_CLOUD_PROJECT`. There is no LiteLLM fallback. Do not work around that with a proxy.

## Merge order, after the branches exist on GitHub

Top down, merge commits only, one push to `main` at the end. Do not squash or rebase.

1. `feat/origin-proof-vertex-only` → `feat/origin-proof-flash-baseline` → `feat/origin-proof-demo` (#50)
2. #50 → #48 → #47 → #45 → #43 → #40 → #41 → #35 → #34, each into its base
3. #34 → `main` last. That is the only push to `main`.
4. Then `feat/origin-proof-design-docs` → `main`
5. Then `feat/origin-proof-frontend-api` into #51, then #51 → `main`. Retarget #51 onto `main` first, or the merge lands on the design branch. Kendra has to approve that retarget and the merge of her frontend branch. She also has to approve merging #48 and #47 (her tuning and GCP PRs) and closing #52.

Kendra's approval: #52 (close, do not merge), #48, #47, #51 (and the frontend-api merge into it), and the design-docs PR only where it takes her frames. Anas approves the final merge to `main`.

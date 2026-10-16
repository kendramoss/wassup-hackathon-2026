# Status — consolidation, 16 Oct 2026

For Anas. Delete this file before the final merge to `main`. Nothing here was pushed or merged to `main`.

## Done

- Inventory matched the backend stack (ancestry checked with `git merge-base --is-ancestor`). Actual GitHub PR numbers, which the map gives as stack position or issue number: #34 skeleton, #35 engine, #41 planner, #40 api, #43 accuracy, #45 compare-agent, #47 gcp (issue #37), #48 tuning (issue #38), #50 demo. Authors match (Mizore66 / kendramoss).
- `feat/origin-proof-flash-baseline` is `feat/origin-proof-demo` plus cherry-picks `76b1720` and `1ec1191`. D44 kept that number and now includes the staging note from `STATUS.md`. That file is not on the flash-baseline tip.
- `feat/origin-proof-vertex-only` is the flash baseline plus Kendra's LiteLLM removal, renumbered **D39 → D49**. Rate limits and `ADMIN_TOKEN` kept. Recorded `*-litellm.*` results and the "proxy, development only" labels were not rewritten. Live call sites after her branch (`extract` live test, `make run` / `make demo-seed` in `docs/DEMO.md`) now say Vertex.
- `cd backend && go test -race ./...` passed. `go vet ./...` passed. `staticcheck ./...` passed when run as `GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...`. The same `go run` under the default toolchain switched to Go 1.26.9 and failed to compile (see Blocked).
- `feat/origin-proof-frontend-api` is Kendra's `feat/origin-proof-frontend` plus the proxy/type fixes. `npm ci`, `npm run build`, and `npm run lint` passed.
- `feat/origin-proof-design-docs` is `main` plus merge commits of `feat/origin-proof-design-compare-ask`, then `feat/origin-proof-palettes`, then `feat/origin-proof-frontend-design`. Only conflict: `docs/design/DESIGN.md`. Kept the newer file, `4e660aa` (2026-10-16 04:51 +0800, portfolio triage) over `09efec8` (2026-10-16 01:30 +0800). No code in that branch.
- Production workflow on this branch only: `deploy.yml` now passes `--set-secrets ADMIN_TOKEN=origin-proof-admin-token:latest` and `--timeout=300`. `CASES_STORE=firestore` and `VERIFIER_CACHE_DIR=/tmp/verifier-cache` were already there. Production was not deployed.
- Pushed the four branches and opened the PRs. Commented on #52. Left #52 open.
- `agent-backend` has `roles/datastore.user` (confirmed on the project IAM policy). Firestore saves should work. The README line that still says the grant is missing was not edited.

## Pull requests

- #57 `feat/origin-proof-flash-baseline` → `feat/origin-proof-demo`
- #58 `feat/origin-proof-vertex-only` → `feat/origin-proof-flash-baseline` (this branch)
- #59 `feat/origin-proof-frontend-api` → `feat/origin-proof-frontend`
- #60 `feat/origin-proof-design-docs` → `main`

## Numbers (not re-run)

Untuned Gemini 3.5 Flash, dev only, adjustments applied, n = 463 (D44): 90.5% (87.5–92.8) vs Argon 91.1% (88.2–93.4). McNemar p = 0.736. Fast path 91.6% of cases at 92.0%. Latency p50 12.1 s / p95 51.2 s. Tuning was not launched.

Official held-out Vertex engine (D39, already on demo): 90.2% (86.8–92.8). Not re-scored.

## Staging

Kendra granted `roles/secretmanager.secretAccessor` on `origin-proof-admin-token` to `agent-backend`. `anasqumhiyeh@gmail.com` still cannot read the secret (`secretmanager.versions.access` denied), so `gcloud secrets versions access` cannot supply `ADMIN_TOKEN`. A new version was added and the service redeployed so the seed and the revision share it. The value was not printed. Version 2 was disabled because its value was not kept. Version 1 is still enabled and is not what the serving revision uses.

Serving revision `origin-proof-api-staging-00004-62n` at `https://origin-proof-api-staging-guxax2teha-uc.a.run.app`. Env: `LLM_PROVIDER=vertex`, `CASES_STORE=firestore`, `VERIFIER_CACHE_DIR=/tmp/verifier-cache`, timeout 300s, `ADMIN_TOKEN=origin-proof-admin-token:latest`. Production was not deployed.

Smoke (`scripts/smoke.sh`, identity token and admin token):

```
ok    GET /health                        200    0.1 s
ok    GET /health?deep=1                 200    2.2 s
      checks: case_store 562 ms, model 2074 ms
ok    GET /v1/demos/heatgun-2022         200    0.1 s
ok    POST /v1/cases/predict (heat gun)  201   35.1 s
      case_5574475b9a24: China 0.90 (needs_info), vertex gemini-4-argon
ok    GET /v1/cases/{id}                 200    0.1 s
ok    GET /v1/cases                      200    0.1 s
      listed: True
ok    POST /v1/cases/{id}/plan (~30 s)   200   58.1 s
      6 moves, 0 shown, 0/6 checks cached
all checks passed against https://origin-proof-api-staging-guxax2teha-uc.a.run.app
```

That smoke case was deleted. One seed, not repeated. Heat gun China 0.78 needs info, plan below the bar (check 0.85). LiDAR Thailand 0.95 auto, plan below the bar (check 0.85). Dryer latches Thailand 0.98 / China 0.98, both auto. All four cases pinned. Vertex row is in `docs/DEMO.md` section 4.

## Blocked

- **staticcheck toolchain.** First attempt: `go run honnef.co/go/tools/cmd/staticcheck@2026.2.2 ./...` downloaded Go 1.26.9 and exited 1 with `unknown field rfd in struct literal of type splicePipe` and `file requires newer Go version go1.27 (application built with go1.26)`. Re-run pinned to Go 1.27.0 exited 0 with no findings.

## Inventory differences (did not change the stack)

- Frontend PR #51 (`feat/origin-proof-frontend`, issue #44) has base `feat/origin-proof-design-compare-ask`, not `main`. Git says `main` is an ancestor and the design branch is not. Left #51's base alone.
- Palette PR #53 was already squash-merged into `feat/origin-proof-design-compare-ask` (`244d3c6`). The original branch `502eae2` is not an ancestor of that commit. Merging it again onto the design-docs branch was clean.
- Flash commits `76b1720` and `1ec1191` are not on a branch tip. They are reachable from the public repo by SHA.
- **New since the map:** #55 `feat/origin-proof-ux` (kendramoss, opened 2026-10-16 02:37 UTC) bases `feat/origin-proof-frontend`. Title: "Frontend UX pass: Blueprint palette and Mizore66's locked designs". It edits the same proxy files as #59 (`route.ts`, `backend.ts`, `schema.d.ts`, Cases home, chrome, origin step). #59 was not retargeted onto it.

## Files that differ between the design branch and #44

Content differs (17): `docs/design/DESIGN.md`; keyframes `p-ask-waiting.html`, `p-ask.html`, `p-bom-empty.html`, `p-bom-ok.html`, `p-bom-warn.html`, `p-cases.html`, `p-plan-q1.html`, `p-scen.html`; and the matching `png/` for those eight keyframes. 151 other `docs/design` files are on the design branch and not on #44 (later keyframes and palette frames A–E). Nothing under `docs/design` is only on #44. Not resolved.

## Not built

Proxy now accepts PUT, PATCH, and DELETE, but no screen calls rename, economics, pin, delete, `POST /v1/cases/batch`, or `GET /health?deep=1`. There is no delete control on Cases home, so pinned cases have a Demo tag and nothing that deletes them. History cost, latency, and token fields are in the generated types only.

## Next

1. **Laptop note:** after D49, local `make run` needs Vertex Application Default Credentials (`gcloud auth application-default login`) and `GOOGLE_CLOUD_PROJECT`. There is no LiteLLM fallback. Do not work around that with a proxy.
2. Decide how #59 and #55 stack. They both start from `feat/origin-proof-frontend` and touch the proxy.
3. Delete this file before the final merge to `main`.

## Merge order

Top down, merge commits only, one push to `main` at the end. Do not squash or rebase.

1. #58 → #57 → #50 (`feat/origin-proof-demo`)
2. #50 → #48 → #47 → #45 → #43 → #40 → #41 → #35 → #34, each into its base
3. #34 → `main` last. That is the only push to `main` from the backend stack.
4. Then #60 → `main`
5. Then #59 into #51, then #51 → `main`. Retarget #51 onto `main` first, or the merge lands on the design branch.

Kendra's approval: close #52 (do not merge it), merge #48 and #47, retarget and merge #51, and merge #59 into her frontend branch. #60 takes her design frames. #55 is hers and overlaps #59. Anas approves the final merge to `main`.

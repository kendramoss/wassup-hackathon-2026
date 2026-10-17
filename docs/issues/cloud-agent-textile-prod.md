# Turn the textile engine on in production (D51), auto-accept at 0.95

Anas approved this: production gets `TEXTILE_ENGINE=on` and `TEXTILE_FAST_PATH=0.95`. The basis is your held-out result: 85.9% overall, 40/42 = 95.2% at confidence ≥ 0.95, Wilson lower bound 84.2%.

**Rules:**
- The merge to `main` deploys production. Anas approved this one merge and deploy; nothing else.
- Don't change `TextileSystem`, K or the split.
- Commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Steps
1. **Rebase onto `main`.** Rebase `feat/origin-proof-textile` onto the current `main`, which now has the shared verdict cache (D50). Resolve the `docs/DECISIONS.md` conflict by keeping both entries, D50 then D51.
2. **Set the deploy config.** In `.github/workflows/deploy.yml`, add `TEXTILE_ENGINE=on,TEXTILE_FAST_PATH=0.95` to the backend's `--set-env-vars`. Keep `VERIFIER_CACHE_STORE=firestore` and the `ADMIN_TOKEN` secret.
3. **Add a D51 paragraph** to `docs/DECISIONS.md` saying production turned on with 0.95 on 16 Oct, approved by Anas. Name the weak spot honestly: two held-out misses at ≥ 0.95, both China predicted as Slovakia.
4. **Check before merging:**
   ```bash
   cd backend && go test -race ./... && go vet ./... && staticcheck ./...
   ```
5. **Open the PR and merge it** with a merge commit, then watch the deploy run.
6. **After the deploy**, run `scripts/smoke.sh <backend URL>` with `ID_TOKEN` and `ADMIN_TOKEN` set; it must pass. Then predict the gaiter facts:
   ```json
   {"date":"2026-09-01","facts":"Knit polyester neck gaiters. The fabric is knit and dyed in China, then cut and sewn into finished gaiters in Vietnam."}
   ```
   Pass: the triage reason starts with "textile rules (19 CFR 102.21)" and `prompt_version` is `textile-v1`. Delete the check case afterwards.
7. **Report:**
   - the PR link and deploy run link
   - the serving revision
   - smoke output and the gaiter result
   - anything that failed, with the exact error

## 8. After the deploy: confirm textile-v2 on Vertex, dev only (~10 min, ~$1)
Branch `feat/origin-proof-textile-v2` (`4a87734`, D53) adds textile-v2: CBP's reasoning shown with each precedent, plus one paragraph on filled made-up articles. On the LiteLLM proxy it scored 85.9% on dev vs 78.8% for v1 (McNemar p = 0.039).
1. **Rebase** it onto `main` after step 5 has landed.
2. **Run dev on Vertex**, then compare against the Vertex v1 dev file:
   ```bash
   cd backend && go run ./cmd/eval run -regime textile -textile-prompt v2 -reasoning -split dev
   go run ./cmd/eval compare -a ../eval/results/textile-dev-gemini-4-argon-prec-textile-v1-vertex.jsonl -b <the new file>
   ```
3. **Don't run held-out.** It was spent on v1, and v2 was chosen on dev.
4. **Record the result** in D53 and `eval/RESULTS.md`.
5. **Open a PR to `main`.** It changes no defaults (`TEXTILE_PROMPT` stays v1), so merging it doesn't change production. Report the numbers. Anas decides whether to switch production to v2.

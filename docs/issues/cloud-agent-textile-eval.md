# Textile regime eval (D51), after the staging checks

Run this after the staging checks. Branch: `feat/origin-proof-textile` (`aa6fb4d`). It adds a textile engine for 19 CFR 102.21, which is off by default (`TEXTILE_ENGINE=off`). The split is frozen in `eval/splits/splits_textile.json`: 371 textile rulings, of which dev = 99 (Jul–Dec 2025) and final = 71 (2026).

**Rules:**
- Dev first.
- Run held-out **once**, only after dev, with the prompt frozen. Don't change `TextileSystem`, K or the split because of the held-out result.
- Never push to `main`.
- Commits end with `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Steps
1. **Dev run:**
   ```bash
   cd backend && LLM_PROVIDER=vertex GOOGLE_CLOUD_PROJECT=wassup-hackathon-2026 GOOGLE_CLOUD_LOCATION=global go run ./cmd/eval run -regime textile -split dev
   ```
   Report:
   - accuracy with its 95% CI
   - accuracy by confidence bin
   - the lowest confidence at which accuracy is ≥ 95%, and how many cases are at or above it
   - cost per case
   - p50 and p95 latency
   - the 10 errors: ruling, gold, predicted, and the rule the model named

   Compare against the substantial-transformation dev result (91.1%).
2. **If dev looks sound** (well above the base rate; most-common-origin and "where assembled" baselines are fine to state), run held-out:
   ```bash
   go run ./cmd/eval run -regime textile -split final -confirm-final
   ```
   Report the same numbers.
3. **Write it up:**
   - add both results to `eval/RESULTS.md`
   - add a dated paragraph to D51 in `docs/DECISIONS.md`
   - include the proposed `TEXTILE_FAST_PATH`: the lowest confidence where **held-out** accuracy is ≥ 95%, or 0 if none
4. **Staging check:** deploy staging with `TEXTILE_ENGINE=on` plus that `TEXTILE_FAST_PATH`. Predict the gaiter facts:
   ```json
   {"date":"2026-09-01","facts":"Knit polyester neck gaiters. The fabric is knit and dyed in China, then cut and sewn into finished gaiters in Vietnam."}
   ```
   Pass: the triage reason starts with "textile rules (19 CFR 102.21)" and the prediction's `prompt_version` is `textile-v1`.
5. **Commit and report:** commit the result files and docs to `feat/origin-proof-textile`, then report. Anas decides whether to turn it on in production.

# Tuned Gemini 3.5 Flash as a fallback engine when Argon fails (D44, D56)

Anas approved this, including the paid tuning job (about $72, D44). It is **not** part of the 17 Oct 14:00 freeze: build and evaluate it on branches, and don't change production until Anas sees the numbers.

**Why:** on 16 Oct, 7 of 37 production `gemini-4-argon` calls failed after all retries (429 Resource exhausted, 504 Deadline expired; D54). Flash kept answering in 1–4 s. Untuned Flash is close to Argon on accuracy (dev 90.5% vs 91.1%, p = 0.74) but overconfident (fast path 92.0% at the 0.92 bar) and slow (p95 51 s) (D44). Tuning on CBP's reasoning should fix the calibration. The fallback is the safety net: Argon stays the primary engine.

**Rules:**
- Never push to `main` or change production env without Anas's go-ahead. Work on branches; open PRs.
- Held-out (`final`) at most once, after dev, with everything frozen; don't tune on it.
- No credentials in git or in text.
- Report exact errors, costs and job IDs.
- Commits end with `Co-Authored-By: Claude Opus 5.5 (1M context) <noreply@anthropic.com>`.

## Part A: tuning data and job (D38 pipeline; ~2–4 h, mostly waiting)
1. **Check before spending.** Read the current Vertex AI tuning docs and confirm:
   - the exact tunable model ID for Gemini 3.5 Flash
   - the region to tune in
   - the price per training token

   Quote the expected bill before launching. If it exceeds $100, stop and report.
2. **Build the targets** (~700 Argon engine calls, ~$12). From `backend/`:
   ```bash
   go run ./cmd/eval run -split train -splits ../eval/splits/splits_tune.json
   go run ./cmd/eval run -split tune_check -splits ../eval/splits/splits_tune.json
   ```
   - Vertex Argon is crowded at times (D54). Use `-workers 4`. The runs resume if interrupted; re-run until every case has a row.
3. **Write the tuning data:** run `go run ./cmd/eval tunedata` → `data/tuning/{train,tune_check}.jsonl`.
   - D38 notes that corrected examples get a constant 0.9 confidence. That could teach a fixed confidence, which is exactly what we're trying to fix. Change it before building: the target confidence for a corrected example should be the engine's own confidence for CBP's origin if available, else 0.75.
   - Record the change in a new D56 entry in `docs/DECISIONS.md`.
   - Report the example counts, how many were corrected, and the token count.
4. **Launch** with `BASE_MODEL=<the Flash 3.5 tunable ID> scripts/tune-vertex.sh` (3 epochs). Report:
   - the tuning job name and its console link
   - the tuned model endpoint when done
   - the final training and validation loss

## Part B: evaluate the tuned model (~30 min after the job)
1. **Dev run:** `ENGINE_MODEL=<tuned endpoint> go run ./cmd/eval run -split dev`. Compare against the Argon Vertex dev file and the untuned Flash dev file with `eval compare` (McNemar). Report:
   - accuracy with its CI
   - the confidence-band table
   - fast-path coverage and accuracy at 0.92
   - the lowest confidence where dev accuracy is ≥ 95%
   - p50 and p95 latency
   - cost per case
2. **Held-out**, only if dev is within 2 points of Argon and calibration improved: run the final split once (`-confirm-final`) and report the same numbers. Otherwise skip it and say why.
3. **Record** both runs in `eval/RESULTS.md` and in D56.

## Part C: fallback in the backend (code, ~1 h; can start while the job runs)
Work on branch `feat/origin-proof-flash-fallback` off `main`.
1. **Fallback model in the LLM layer:** a wrapper `llm.Model` that calls the primary, and when the primary's final error (after its retries) is a 429, a 503/504, or a deadline, calls `FALLBACK_ENGINE_MODEL` once with the same request.
   - It never falls back on a 400 or a bad response.
   - The response's `ModelID` is the fallback model, so `trace.model_id` shows which model answered.
   - Off when `FALLBACK_ENGINE_MODEL` is empty (the default).
2. **Wrap only the engine's model** (`engine.New` in `cmd/server/main.go`), not the router, verifier, clarify or agent.
   - Add a short `ENGINE_ATTEMPTS` (or similar) so the primary gives up faster when a fallback exists. Today 4 attempts × 30 s plus backoff can take ~2 min before falling back. Propose a value, e.g. 2 attempts.
3. **Triage:** a prediction made by the fallback model never auto-accepts unless `FALLBACK_FAST_PATH` is set from Part B's held-out result. Default: always `needs_review`, with reason "answered by the fallback model (<id>) because the primary engine was unavailable".
4. **Engine cache:** the cache key includes the model, so a fallback answer is cached under the fallback model. Make sure a later Argon success for the same case isn't blocked by it: the Argon key differs. Add a test.
5. **Tests:**
   - fallback on a 429
   - fallback on a deadline
   - no fallback on a 400
   - the off-by-default path
   - the triage reason
   - the `trace.model_id`

   Then run `go test -race ./... && go vet ./... && staticcheck ./...`.
6. **Docs:** add to D56 and the OpenAPI triage reason description.
7. **Open the PR** and don't merge it.

## Part D: staging (~20 min)
1. Deploy staging from the PR branch with `FALLBACK_ENGINE_MODEL=<tuned endpoint>`.
2. Force the fallback by setting `ENGINE_MODEL` to a nonexistent model ID on staging only. Run the heat-gun predict and report:
   - which model answered
   - the triage route and reason
   - the latency
3. Restore staging env afterwards.

## Report
- Bill so far.
- Job ID and endpoint.
- Dev (and held-out, if run) tables against Argon and untuned Flash.
- PR link.
- Staging results.
- Your recommendation: should production set `FALLBACK_ENGINE_MODEL`, and what should `FALLBACK_FAST_PATH` be (0 = always review)?
- Anything blocked.

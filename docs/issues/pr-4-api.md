Fourth of five stacked PRs. Base: `feat/origin-proof-planner` (#<PR 3>).

## What this adds
- **OpenAPI contract:** `docs/api/openapi.yaml`.
- **Endpoints (/v1):**
  - `POST /v1/cases/predict` (facts text or structured product)
  - `GET /v1/cases/{id}`
  - `POST /v1/cases/{id}/plan`
  - `POST /v1/cases/{id}/export` (draft CBP ruling request in Markdown, for a broker)
  - `GET /v1/rulings/{id}`
  - `GET /v1/eval/summary`
  - `GET /v1/demos`, `GET /v1/demos/{id}`
- **Wiring:** corpus, engine, router, verifier and planner wired into the server. The precedent corpus is bundled into the backend image (this is why the file count is high: ~2,800 facts files).
- **Results page:** `/v1/eval/summary` serves numbers generated from the committed eval runs, each with its split, n, CI and source. Proxy runs are flagged.
- **Fixes:** pasted facts are normalised to the benchmark format (trailing newline, D20); plan lists are never null.
- **Rejection kinds:** current_setup, below_confidence_bar, no_supporting_precedent, engine_other_origin and check_failed. Precedent-supported near misses sort first.
- **Docs:** Origin Proof README, sources for every on-screen number (`docs/NUMBERS.md`), decisions D18–D19.

## Checks
`make test lint` passes. Handler tests use fakes; no model calls.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

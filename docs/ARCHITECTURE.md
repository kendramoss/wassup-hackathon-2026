# Architecture

```
frontend (Next.js, Cloud Run, public) ──HTTP/JSON──► backend (Go, Cloud Run, private)
                                                       ├─ router    Gemini 3.8 Flash: family label + regime check
                                                       ├─ engine    Gemini 4 Argon + 5 earlier rulings (BM25): origin, key factors, confidence, citations
                                                       ├─ triage    confidence ≥ 0.95 and in scope → auto; else needs_review
                                                       ├─ planner   single/double operation-group moves → variant facts → engine
                                                       ├─ verifier  ADK agent (Argon) + search_rulings / read_ruling → precedent-supported or not
                                                       └─ export    draft ruling-request document
                                                     stores: Firestore (cases, plans, runs) · GCS (corpus, index)
eval (Go CLI) → engine/verifier on frozen splits → eval/results/*.jsonl + eval/RESULTS.md
```

## Packages (`backend/`)

| Package | Status | Role |
|---|---|---|
| `cmd/server` | Gate A | wiring, `-smoke` |
| `internal/domain` | Gate A | pure types + validation, country normalisation |
| `internal/config` | Gate A | env config, defaults, validation |
| `internal/llm` | Gate A | `Model` interface; LiteLLM, Vertex, Fake; retry + logging decorators |
| `internal/httpapi` | Gate A | routing, request IDs, error JSON `{code, message, details}` |
| `internal/cross` | Gate B | CROSS client (`/api/ruling/<id>`) |
| `internal/corpus`, `cmd/corpus` | Gate B | facts splitter (byte-identical port), labels, splits, in-memory store; fetch/build/compare CLI |
| `internal/retrieval` | Gate B | `Retriever` interface; BM25 with strict earlier-date filter |
| `internal/engine` | Gate B | engine-v1 prompt, prediction, cited-ruling extraction |
| `internal/router`, `internal/triage` | Gate B | family + regime label (Flash); auto vs needs_review |
| `internal/evalkit`, `cmd/eval` | Gate B | eval rows, Wilson CIs, bins, cost; run / report / scope |
| `planner`, `verifier`, `cross`, `duty` | Gate C | |
| `export`, `store`, remaining endpoints | Gate D | |

Business packages depend only on `domain` and interfaces (`llm.Model`, `retrieval.Retriever`, `cross.Client`, `store.*`).

## Model calls
All calls: temperature 0, JSON schema when a JSON answer is expected, prompt version recorded in the output. `llm.WithLogging` logs provider, model, tokens, latency and tool calls per call; prompts and answers only at debug level.

## Data flow (eval and production)
1. `cmd/corpus fetch` → `data/corpus/raw/<id>.json` (CROSS JSON; gitignored). `cmd/corpus build` → `data/corpus/facts/<id>.txt`.
2. `corpus.Load(labels, factsDir)` → store of rulings with a single held origin (the precedent pool).
3. `engine.Predict(case)`: BM25 over the pool, filtered to rulings dated strictly before the case → prompt → Gemini → normalised prediction.
4. `router.Classify(facts)` → regime → `triage.Decide` (auto iff confidence ≥ 0.95 and in scope).

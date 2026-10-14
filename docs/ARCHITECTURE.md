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
| `corpus`, `retrieval`, `engine`, `router`, `triage`, `cmd/eval`, `cmd/corpus` | Gate B | |
| `planner`, `verifier`, `cross`, `duty` | Gate C | |
| `export`, `store`, remaining endpoints | Gate D | |

Business packages depend only on `domain` and interfaces (`llm.Model`, `retrieval.Retriever`, `cross.Client`, `store.*`).

## Model calls
All calls: temperature 0, JSON schema when a JSON answer is expected, prompt version recorded in the output. `llm.WithLogging` logs provider, model, tokens, latency and tool calls per call; prompts and answers only at debug level.

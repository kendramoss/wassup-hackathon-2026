First of five stacked PRs (skeleton → engine → planner → API → accuracy). Base: `main`. Don't merge until the backend is finished and the GCP/Vertex setup is done.

## What this adds
Replaces the template agent with the Origin Proof backend skeleton:
- **Domain types** with validation: product, plants, components, operations, case, ruling, prediction, plan, verdict.
- **Config:** read from the environment only, validated at start-up. Secrets are never logged.
- **LLM layer:** one `Model` interface with three providers.
  - `vertex`: google.golang.org/genai, for deployment and official numbers.
  - `litellm`: development only; refuses to start on Cloud Run.
  - `fake`: offline tests.
  - Retry and logging decorators. Temperature 0 everywhere.
- **Server:** provider chosen by config, request logging, `/health`, and a `-smoke` live call.
- **CI and deploy:** gofmt, build, vet, staticcheck and tests on every PR. Backend deploys with `LLM_PROVIDER=vertex` via Workload Identity Federation.
- **Docs:** `DECISIONS.md`, `ARCHITECTURE.md`, prompt index, and `SETUP-GCP.md` (the gcloud commands a project owner runs).

## Checks
`make test lint` passes. No model calls are needed to test.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

# Manual non-production smoke workflow (#276)

`.github/workflows/smoke-nonprod.yml` is a manually dispatched, **offline** check. This page says what it does, what it does not prove, and what blocks a real-model version. It is not native Vertex validation, and nothing here closes the staging criterion of #276; that criterion is **UNVERIFIED**. Use `Refs #276`, never a closing keyword.

## Manual pre-promotion requirement

Before production traffic moves to a new revision, a person must run a real-Vertex smoke of the model-dependent paths, including the family check, against `origin-proof-api-staging` or a tagged no-traffic revision, and record the revision, model, configuration and pass or fail evidence. That is a manual gate. This workflow does not perform it, does not target `origin-proof-api-staging`, and has no deploy, traffic, revision or promotion step.

## What the workflow runs

Trigger: `workflow_dispatch` only. The one input, `mode`, is a choice with a single option, `selfcheck` (the default). No input enables a model call. Permissions are `contents: read`; there is no `id-token`, no secret, no variable, no environment and no cloud credential. Checkout does not persist credentials. One run at a time (`concurrency`), 15 minute timeout. The job summary records the source SHA, mode, target and Go version, and nothing secret.

Steps, in order:

1. Refuse non-local targets and unexpected modes. The target is the literal `none` (a `local-harness` value is also accepted); anything else, and anything matching the production names and URLs in `deploy.yml` (`run.app`, `undelabs.dev`, `origin-proof`, the project number, the project id), fails the job.
2. Build `backend/cmd/readiness-eval` and run `readiness-eval selfcheck` (proves the process-wide HTTP lock-down; no connection is made) and `readiness-eval smoke --dry-run` (fake model, temporary ledger, no network). Bare `readiness-eval smoke` must be refused.
3. `go test -race -count=1` of `internal/readiness/smokecheck`, `internal/readiness/workflowcheck` and `cmd/readiness-eval`.

The `paid-generation` job is `if: ${{ false }}`. It only prints BLOCKED and fails. It has no secret, environment or input, and nothing depends on it, so a dispatch cannot run it. Enabling it needs a reviewed change to this file.

## What the tests exercise

`internal/readiness/smokecheck` starts the real `httpapi` handler on a loopback listener, wires the real `familycheck.Checker` to a scripted `llm.Fake`, and posts to `/v1/products/family-check` over real HTTP with a client that can reach only that listener. Cases: every supported family, `none`, invented and invalid-enum families (rejected, `502 model_bad_response`, no family in the body), malformed model output, provider errors (`502 model_unavailable`, upstream body not echoed, exactly one model call and no retry), malformed requests (`400`, no model call), the route absent when the checker is off, and the production selector still rejecting `LLM_PROVIDER=litellm`. Every failure must be the error body (`code`, `message`) with no `result`.

`internal/readiness/workflowcheck` parses the workflow with a small stdlib parser that fails closed, and checks: dispatch only, the single choice input, read-only permissions, no secrets or environments or network tools, expressions on an allow-list, pinned actions already used elsewhere in the repo, the blocked job cannot be enabled, every command exists in the tool, the tool's real output matches what the workflow greps for, and the target-refusal step is actually executed against the production URLs in `deploy.yml`.

Run locally from `backend/`:

```
GOFLAGS=-mod=readonly go test -race -count=1 -v ./internal/readiness/smokecheck/ ./internal/readiness/workflowcheck/
go run ./cmd/readiness-eval selfcheck
go run ./cmd/readiness-eval smoke --dry-run
```

## What it does and does not prove

It proves the handler and checker reject invented, malformed and failed answers without retrying, offline, and that the workflow file stays inside its offline boundary. It does not prove native Vertex behaviour, a deployed revision, model quality, gateway behaviour, or anything about `origin-proof-api-staging`. The tool's `smoke` command is generic (a synthetic ping through the budget guard); it does not call the family-check API. A fake or LiteLLM success cannot close the native-Vertex criterion.

## Real-model smoke: BLOCKED

No real-model path exists in the workflow. It stays BLOCKED until all of these exist, each reviewed:

- the budget runner and the frozen pricing proof (see `docs/evaluation-readiness.md`);
- egress and proxy facts for the single gateway host, and an available key;
- a sub-allocation from the same shared cap, recorded in the same authoritative `SHAREDBUDGET.json`, chained with the other runs' ledgers. The cap is the one shared total in `docs/evaluation-readiness.md`; it is not a fresh allowance per workflow, and the file must never be copied or reset;
- `calls_authorized` set in that file by main, and an explicit run handoff;
- a protected GitHub environment created by its owner, not by this change.

Missing secrets or endpoint are a documented blocker, not a reason to configure anything. A future live run reuses the reviewed guard and its `--allow-model-calls`; it must not add a second HTTP client that can spend outside the guard.

Cases to run in a future live smoke against a family-check endpoint (synthetic inputs only): a clearly supported product (expect its template family), an unsupported product (expect `none`), an input that yields an unusable model answer (expect `model_bad_response`), and a provider error (expect `model_unavailable`), each with the recorded revision, model and configuration. Native Vertex staging additionally needs the Vertex credentials the owner holds; this repo's tooling cannot supply them.

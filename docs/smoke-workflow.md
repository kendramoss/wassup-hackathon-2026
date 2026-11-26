# Smoke workflows (#276)

There are two separate checks. `.github/workflows/smoke-nonprod.yml` is a manually dispatched, **offline** workflow. `scripts/audit-m2-smoke.ps1` is the **live** real-Vertex runbook against a tagged no-traffic revision. The offline workflow alone does not close the staging criterion of #276; the live run does. Use `Refs #276`, never a closing keyword in the offline workflow.

## Manual pre-promotion gate

Before production traffic moves to a new revision, a person must run the live smoke against a tag on that revision and keep the generated evidence. The gate is not satisfied by mocked tests or by the offline workflow alone.

Deploy the candidate without traffic and tag it:

```powershell
gcloud run deploy backend --source backend --project wassup-hackathon-2026 --region us-central1 --no-traffic --tag audit-m2
```

or tag an existing revision without shifting traffic:

```powershell
gcloud run services update-traffic backend --project wassup-hackathon-2026 --region us-central1 --update-tags=audit-m2=<REVISION>
```

**Known gap:** `.github/workflows/deploy.yml` runs `update-traffic --to-latest` after each backend deploy, so a merge to `main` puts the new revision on 100% of traffic before anyone can run this gate. Until #845 changes the deploy, the gate only holds for revisions deployed by hand with `--no-traffic`.

Then run the smoke:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/audit-m2-smoke.ps1
```

The script resolves the tag URL, obtains an identity token, posts synthetic inputs to the family-check endpoint, verifies the real Vertex response, writes `docs/audit-m2-evidence.md`, and exits non-zero on failure. It never deploys, shifts traffic, or creates the tag. It refuses a revision that already serves traffic; pass `-AllowLiveRevision` to re-check a live revision, and the evidence then records the run as a post-deploy re-check, not as the gate. Supported products must return exactly `-ExpectedSupportedFamily` (default `motorised_tools_small_appliances`). Remove the tag afterwards with `--remove-tags=audit-m2`, or it keeps the old revision reachable. On a machine that blocks Go-built test binaries, run the guard tests through `smoke-nonprod.yml` and pass the passing run URL with `-SkipOfflineTests -OfflineTestEvidenceUrl <URL>`.

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

## Live no-traffic Vertex smoke

`scripts/audit-m2-smoke.ps1` covers the native Vertex path without adding a paid path to CI. It needs a signed-in `gcloud` with project access, `curl`, Go (unless the offline tests are skipped with CI evidence), and a no-traffic tag on the revision under test. It records the tag URL, revision, image, service account, provider, model and project/location in `docs/audit-m2-evidence.md`. Use synthetic inputs only.

Live cases: a clearly supported product (expect a known template family), an unsupported product (expect `none`), and four malformed requests: blank text, a body that isn't JSON, an unknown field, and text over 2,000 characters (each expects `400 invalid_input`, rejected before any model call). Native Vertex cannot be forced on demand to emit a bad enum, malformed JSON or a provider error, so those paths stay in the offline guard tests: invented or invalid-enum families and malformed model output return `502 model_bad_response` with no result and no retry, and provider errors return `502 model_unavailable` without echoing upstream details. The passing `smoke-nonprod.yml` run is linked from the evidence file.

The separate LiteLLM gateway evaluation remains blocked until the budget runner, frozen pricing proof, gateway egress facts, shared-cap sub-allocation, `calls_authorized`, run handoff and protected environment all exist (see `docs/evaluation-readiness.md`). Missing secrets or endpoint are a documented blocker, not a reason to configure anything. A future gateway run reuses the reviewed guard and its `--allow-model-calls`; it must not add a second HTTP client that can spend outside the guard. The live script relies on the owner's `gcloud` identity and the service account's Vertex access; it embeds no credentials.

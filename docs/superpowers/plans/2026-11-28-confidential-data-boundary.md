# Confidential-data boundary implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Deliver a reviewed-text, single-turn protected origin check whose actual model request is approved, tenant-bound and tested for seeded identifier leakage without silently changing legal facts or degrading into raw processing.

**Architecture:** A new confidential-context service owns policy, local preparation, encrypted jobs/token maps, approval and execution. A mandatory raw-client gate checks the immutable prepared envelope at each send; unsupported protected paths deny before external I/O. Existing standard prediction keeps its API and prompts; the first protected endpoint reuses factored single-turn engine preparation/interpretation, not a separate legal prompt.

**Tech stack:** Existing Go HTTP/auth/store/LLM interfaces, local public BM25 retrieval, `envelope.Sealer`, existing frontend Next.js/TypeScript and Go/Node tests. No additional cloud redaction model. Private OCR/media and multi-turn agent support are later, separate increments.

**Spec:** [Confidential-data-boundary design](../specs/2026-11-28-confidential-data-boundary-design.md).

**Planning status:** This file adds no settings, endpoints or runtime protection. All interfaces and filenames marked new below are proposed. Execution requires a separate implementation work item; this documentation change does not enable it.

## Global constraints

- Author/committer Mizore66; project attribution and PR footer apply.
- Do not edit deployment workflows, Terraform or provider settings in feature PRs. Owner key/recipient setup is a separately tracked prerequisite.
- No secrets, gateway host or credential-file locations in code, docs, logs or artifacts.
- Any paid utility evaluation uses only the approved gateway and synthetic/public authorized cases; check the existing programme ledger and register the run before scoring. Never use fake outputs as measured utility results.
- No foundation-model fine-tuning. No claim of universal PII detection, anonymization, zero provider retention or on-device/private inference from this MVP.
- Keep countries, tariff editions/codes, amounts/units/denominators, process order and missingness unchanged. Do not apply `logsafe` to inference payloads.
- Missing scope, changed approval, unsupported media/tool/embedding route, unavailable key/store or forbidden recipient must block protected processing. No fallback to raw standard mode, including during rollback.
- No protected result/vault in global caches, ordinary case JSON, model history, browser persistent storage or logs. Provider-side caching is a separate verified policy requirement.
- No force-push, reset, stash, clean or branch deletion. Tests first for behavior changes; preserve failed evaluations.

## File and interface map

| Unit | Proposed files / current seams | Responsibility |
|---|---|---|
| Scope/policy | New `backend/internal/confidential/types.go`, `policy.go`, `context.go`; current HTTP principal and config packages | Trusted classification, immutable policy/recipient and validation. |
| Compiler/crypto/store | New `compiler.go`, `canonical.go`, `vault.go`, `store.go`, `firestore.go` under `internal/confidential` | Preserve legal facts; prepare exact request; random stable tokens; sealed job state, grants and atomic transitions. |
| Model gate | New `model.go`, `guard_test.go`; current `llm` errors and raw constructors in `cmd/server/{main,japan}.go` | Mandatory recipient/request/lease check directly before every provider attempt. |
| Engine/HTTP | Current `engine/{engine,prompt}.go`, `httpapi/{principal,cases,metering}.go`; new `httpapi/confidential.go` | Reuse single-turn preparation/interpretation; admin policy and protected prepare/approve/execute/status/cancel routes. |
| UI | Proposed case protected-context screen/component, workspace policy controls and API types; read current `frontend/AGENTS.md` and installed Next docs before implementation | Truthful supported-path notice, exact preview, approval, execution and receipt/error states. |
| Evidence | New `eval/privacy-boundary/` protocol, manifests, fixtures, results and offline summarizer; corresponding tests | Deterministic seeded-secret proof plus real gateway utility/performance evidence, separately labelled. |

Before execution, confirm each current source path/signature still exists. The inspection baseline is main `2a879690`; do not reuse line numbers as an automatic patch.

## Task 1: trusted policy/scope and explicit request classification

**Create:** `types.go`, `policy.go`, `context.go`, `policy_test.go`, `context_test.go` in `backend/internal/confidential`. **Modify:** `backend/internal/config/config.go` and `config_test.go` for the proposed mode, TTL and key/recipient-reference settings; startup summaries report mode only, never key material, recipient hosts or private context.

**Proposed core contract:** define these types before other tasks consume them. Keep policy fields server-owned; do not deserialize a trusted scope from a user header.

```go
package confidential

import (
    "context"
    "errors"
    "time"
    "github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
    "github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

type Class string
const (
    Standard Class = "standard"
    Protected Class = "protected"
    PublicSystem Class = "public_system"
)
var (
    ErrDenied = errors.New("confidential policy denied")
    ErrExpired = errors.New("confidential approval expired")
    ErrStale = errors.New("confidential source or policy changed")
)
type Scope struct {
    Class Class
    Workspace, CaseID, JobID, ActorID string
    SourceVersion, PolicyVersion, Purpose string
}
type Recipient struct { ID, Provider, Model, Region string }
type Policy struct {
    Version string
    RecipientPolicyVersion string
    Protected bool
    Recipients []Recipient
    RequireApproval bool
}
type Entity struct { Kind, Value string }
type Amount struct { Kind, Value, Currency, Unit, Denominator string }
type Step struct { Country, Process string }
type LegalFacts struct {
    Jurisdiction string
    TariffCodes, TariffEditions []string
    Amounts []Amount
    Steps []Step
    OriginStatuses []string
}
type Input struct { Facts string; Legal LegalFacts; Entities []Entity }
type Preview struct {
    JobID, Digest, SourceVersion, PolicyVersion string
    Recipient Recipient
    Request llm.Request
    Warnings []string
    ExpiresAt time.Time
}
type Grant struct { ID, JobID, Digest string; ExpiresAt time.Time }
type Receipt struct { JobID, PolicyVersion, RecipientID, Outcome string }
type HumanResult struct {
    Prediction domain.Prediction
    CitationIDs, Warnings []string
    SourceVersion string
}
type Output struct { Result HumanResult; Receipt Receipt }
type PolicyStore interface {
    Load(context.Context, string) (Policy, error)
    CompareAndSwap(context.Context, string, string, Policy) error // workspace, expected version, replacement
}
func WithScope(context.Context, Scope) context.Context
func ScopeFrom(context.Context) (Scope, bool)
func ValidatePolicy(Policy) error
```

The final three declarations above describe the exported signatures to implement, not callable functions already in the repo. Add clock/crypto dependencies to constructors, not mutable global clocks. Keep source-snapshot MAC generation server-side. Inventory and tag existing standard/visitor/API-key/health/startup/background callers first; observe classification with protected activation prohibited. Do not install deny-by-default sends into standard traffic until the inventory and regression tests pass. After enforcement, disabling protected execution cannot remove the send guard.

- [ ] **Write red tests.** `TestScopeMissingIsNotStandard`, `TestProtectedPolicyRequiresApprovalAndRecipient`, `TestUnknownRecipientDenied`, `TestScopeIndependentOfUsageMetering`. Example validation contract:

```go
func TestProtectedPolicyRequiresApproval(t *testing.T) {
    p := Policy{Version: "p1", Protected: true, RequireApproval: false,
        Recipients: []Recipient{{ID:"origin-primary", Provider:"fake", Model:"test", Region:"test-region"}}}
    if !errors.Is(ValidatePolicy(p), ErrDenied) { t.Fatal("protected policy accepted an approval bypass") }
}
```

- [ ] **Run red:** `cd backend && go test ./internal/confidential -run 'Scope|Policy|Recipient'`. Initially package/functions are absent; after wiring tests must fail for the stated bypasses, not setup errors.
- [ ] **Implement:** validate nonempty versions, supported class/purpose and recipients; protected approval cannot be disabled. Use an unexported context-key type. Scope reconstruction consumes the authenticated principal even if usage metering is off. Explicit constant-data health/system calls get a narrowly validated `PublicSystem` scope; missing classification denies whenever reaching the gate, including after the protected feature switch is disabled.
- [ ] **Verify:** same tests plus `go test -race ./internal/confidential`. Confirm caller-owned policy slices cannot be mutated after load.
- [ ] **Commit** this unit with project attribution; no model/client construction yet.

## Task 2: legal-fact-preserving compiler, encrypted job store and approval

**Create:** `compiler.go`, `canonical.go`, `vault.go`, `store.go`, `firestore.go` and their tests. Reuse `envelope.NewSealer`, `Seal` and `Open`, with independent purposes and tenant/job authenticated context. Dedicated owner key missing means protected mode unavailable, not plaintext storage.

**Interfaces to implement and expose to later tasks:**

```go
type PreparedRecord struct {
    Scope Scope
    RecipientPolicyVersion string
    Recipient Recipient
    Request llm.Request
    Legal LegalFacts
    Interpretation []byte // versioned, tokenized engine interpretation metadata; sealed and MAC-bound
    Digest string
    ExpiresAt time.Time
}
type ExecutionCheck struct {
    SourceVersion, PolicyVersion, RecipientPolicyVersion, Digest string
}
type RecordStore interface {
    PutPrepared(context.Context, PreparedRecord, string) error // final arg is sealed private map
    LoadPrepared(context.Context, Scope, string) (PreparedRecord, string, error)
    Approve(context.Context, Scope, string, string, time.Time) (Grant, error)
    Acquire(context.Context, Scope, string, ExecutionCheck, time.Time) (PreparedRecord, error)
    SaveProviderReply(context.Context, Scope, string, llm.Response) error // sealed, tokenized internal reply only
    LoadProviderReply(context.Context, Scope, string) (llm.Response, error)
    Complete(context.Context, Scope, string, Output) error
    LoadCompleted(context.Context, Scope, string) (Output, error)
    Delete(context.Context, Scope, string) error
}
func Compile(Input, map[string]string) (Input, error)
func Digest([]byte, PreparedRecord) (string, error)
type Service interface {
    Prepare(context.Context, Scope, Input, Recipient) (Preview, error)
    Approve(context.Context, Scope, string, string) (Grant, error)
    Execute(context.Context, Scope, string) (Output, error)
    Status(context.Context, Scope, string) (Receipt, error)
    Result(context.Context, Scope, string) (Output, error)
    Cancel(context.Context, Scope, string) error
}
```

The HTTP dependency is the proposed `confidential.Service` interface. Its implementation loads policy/source independently, builds the request using the factored engine adapter, seals/stores it, verifies approval and executes through the protected model; handlers do not assemble or modify provider requests.

`Compile` consumes an explicit value→token map plus local detector findings; the service generates random tokens once per job, not per retry. The protected record and map are sealed at rest; the interface must not expose raw database keys or mappings to HTTP/MCP. Build `detector.go` and `detector_test.go`: deterministic supported email/phone/identifier detection with independently labelled spans, no remote detector and no automatic named-party NER promise. Ambiguous names/free text require declaration/review. Define the store implementation's state machine as `prepared → approved → running → provider_succeeded → completed`, with explicit `completion_pending`, `execution_uncertain`, `failed`, `cancelled` and `expired` terminal/recovery outcomes; only authenticated atomic compare-and-swap acquisition can enter `running`. `Acquire` compares the supplied prepared expectations with transactionally read current case/source MAC, workspace and case sticky requirements, policy and its required recipient-policy version, actor membership/API-key role/revocation and grant/job state. The required role is fixed by the service, never a client-selected minimum. The active recipient-policy config is immutable/versioned per process: compare that trusted in-process version before acquire and at raw send against both the prepared record and the workspace policy's required version. The required version is persisted with workspace policy, so an old pod cannot continue after that workspace moves to a newer version. Read the persistent records and transition the lease in one transaction; race tests change each record between prepare and acquire. Only retries owned by that running lease may reuse the approved envelope.

- [ ] **Write red tests:** legal-fact invariance, empty/unknown entity handling, original-input immutability, token consistency within job, token unlinkability across jobs, cross-tenant open denied, expiry before storage TTL, duplicate acquire denied, changed source/policy denied, no mapping in preview/receipt. A complete compiler invariant test:

```go
func TestCompilePreservesLegalFacts(t *testing.T) {
    in := Input{Facts:"Supplier Example Contact makes goods in Thailand; code 8479.89; value 40.00 USD.",
        Entities:[]Entity{{Kind:"party",Value:"Example Contact"}},
        Legal:LegalFacts{Jurisdiction:"US",TariffCodes:[]string{"8479.89"},TariffEditions:[]string{"HS2022"},
            Amounts:[]Amount{{Kind:"value",Value:"40.00",Currency:"USD",Unit:"per good",Denominator:"FOB"}},
            Steps:[]Step{{Country:"Thailand",Process:"assembly"}},OriginStatuses:[]string{"unknown"}}}
    got,err := Compile(in,map[string]string{"Example Contact":"PARTY_7K4"})
    if err != nil { t.Fatal(err) }
    if !reflect.DeepEqual(in.Legal,got.Legal) { t.Fatal("privacy changed legal facts") }
    if strings.Contains(got.Facts,"Example Contact") || !strings.Contains(got.Facts,"PARTY_7K4") { t.Fatal("identity was not replaced") }
    for _,fact := range []string{"Thailand", "8479.89", "40.00 USD"} {
        if !strings.Contains(got.Facts,fact) { t.Fatalf("legal narrative lost %q",fact) }
    }
    if in.Facts != "Supplier Example Contact makes goods in Thailand; code 8479.89; value 40.00 USD." { t.Fatal("source mutated") }
}
```

- [ ] **Run red:** `go test ./internal/confidential -run 'Compile|Vault|Grant|Acquire|Digest'`.
- [ ] **Implement exact algorithms:** parse/replace declared supported string spans with longest-match non-overlap handling; never run identifier regexes over typed legal facts. Reject a declared entity/span that collides with an origin country, tariff code/edition, numeric legal value or other annotated legal field; do not let a sanitized narrative contradict an unchanged typed projection. Render typed facts canonically and require review of any remaining narrative. Mark unresolved free text for review. After building the final model request, scan every provider-visible string/schema for the declared/detected private values; a remaining known value prevents approval and cannot be overridden by a preview click. This is not proof of detecting unknown secrets. Deep-copy slices/maps. Canonicalize an explicit prepared-envelope DTO containing all bound fields including `RecipientPolicyVersion`, the existing `llm.Seed` constant and prompt/compiler/corpus revision; reject binary parts/tools/tool history before digest because normal request JSON omits `Parts`. MAC with a dedicated derived key. Encrypt records/maps with workspace/case/job/policy/source AAD. In the persistent adapter, atomically read/compare case source version, policy version, actor authorization and grant state with execution acquisition; do not do an unguarded read-then-CAS that permits a stale case/policy race; exact request re-use, not fresh rendering after approval. Enforce the proposed 900-second grant and 3,600-second job maximum in code with injectable clocks; cancel/expiry deletes all private job material. After a successful model call, `SaveProviderReply` durably seals the tokenized internal reply before finalization. Rehydrate into `HumanResult` while the map exists; `Complete` atomically seals/stores `Output`, records the safe receipt and deletes the mapping/prepared private material while consuming the grant. If `Complete` fails, keep the map sealed, report `completion_pending`, and retry finalization from the stored reply without another model call. If even saving the provider reply fails, mark the lease/outcome uncertain, return no human result and never automatically re-run inference; recover/expire under explicit operator procedure. An authorized repeated `Execute` on `completion_pending` resumes finalization from `LoadProviderReply` without another `Acquire` or provider call; `Status` and `Result` GETs remain read-only. Add `TestFinalizationResumeKeepsProviderCallCountOne`. Authorized completed-result retrieval until job TTL makes no model call. Job expiry deletes sealed replies/results/maps even in pending/uncertain states; the user must prepare a new job, never auto-replay the expired one. Protected filing export stays unsupported until a separate verified-source export increment.
- [ ] **Verify:** run race tests with concurrent approval/acquire/delete; restart using the persistent adapter; assert privacy store errors/logs contain no entity values. Test synthetic phone-like tariff/amount strings are preserved in final request capture. Test the compiler cannot pass merely by copying `LegalFacts` while corrupting the narrative.
- [ ] **Commit** compiler/store/approval as one tested unit. In-memory storage is a test adapter only, not a production design depending on sticky app instances.

## Task 3: raw-client egress gate and no-bypass inventory

**Create:** `internal/confidential/model.go`, `model_test.go`, `guard_test.go`; safe error mapping in `internal/llm`/HTTP as needed. **Modify:** raw constructors in `cmd/server/main.go` and `cmd/server/japan.go`, plus their tests. Use `llm.Model` exactly as it exists: `Generate(context.Context, llm.Request)` and `Provider()`.

**Proposed interface:**

```go
type LeaseVerifier interface {
    Verify(context.Context, Scope, Recipient, llm.Request) error
}
func Protect(llm.Model, Recipient, LeaseVerifier) llm.Model
```

- [ ] **Write red tests:** one secret at a time in `System`, message `Content`/`Name`, schema descriptions/enums, tool argument/result content and binary `Parts`. Missing scope, expired lease, altered request/model/region, forbidden fallback and unsupported tool requests must leave the inner fake's call list empty. Scope for standard and public-system calls is still explicit. Example deny contract:

```go
type denyLease struct{}
func (denyLease) Verify(context.Context, Scope, Recipient, llm.Request) error { return ErrDenied }
func TestDeniedRequestNeverReachesProvider(t *testing.T) {
    raw := llm.FakeText(`{"country":"Thailand"}`)
    r := Recipient{ID:"origin-primary",Provider:"fake",Model:"test",Region:"test-region"}
    gated := Protect(raw,r,denyLease{})
    ctx := WithScope(context.Background(),Scope{Class:Protected,Workspace:"ws_test",CaseID:"case_test",JobID:"job_test",ActorID:"actor_test",SourceVersion:"s1",PolicyVersion:"p1",Purpose:"origin.single_check"})
    _,err := gated.Generate(ctx,llm.Request{Model:"test",System:"PRIVATE_SENTINEL"})
    if !errors.Is(err,ErrDenied) { t.Fatal("expected policy denial") }
    if len(raw.Calls()) != 0 { t.Fatal("denial reached provider") }
}
```

- [ ] **Run red:** `go test ./internal/confidential ./internal/llm ./cmd/server -run 'Protected|Denied|Bypass|Fallback'`.
- [ ] **Implement:** gate every raw base, separate fallback and Japan gateway before pacing/retry/fallback; no response detokenization in this shared wrapper. Make policy denial non-retryable/non-fallback eligible. `LeaseVerifier` checks authenticated class, sticky case requirement, actual recipient and the canonical prepared digest; unsupported request features deny. Scan/validate generated private response before returning to authorized presentation; unknown tokens cause review. Test actual HTTP serialization at existing provider-client transports, not only the abstract request struct.
- [ ] **Before cutover, prove caller coverage:** add `TestEveryGenerateCallerClassified` using a checked-in AST-discovered inventory of every model `Generate` call and SDK constructor. Exercise HTTP users, workspace keys, visitors, health/startup probes, background callbacks and ADK forwarding; a new/unclassified caller fails CI. Observe classification before enabling any protected workspace. Then enable deny-by-default with the unchanged standard-mode suite green; this is a required checkbox, not optional rollout prose.
- [ ] **Add an AST/import guard:** new SDK/HTTP client constructors require an explicit allowlist review. Include embeddings as a separate interface and offline commands as public/synthetic-only entry points; do not claim `Model.Generate` intercepts `Embed`. Inventory remote CROSS, webhook and MCP egress separately.
- [ ] **Verify:** retry/fallback/ADK context-propagation and token-history tests, even though protected ADK is initially denied; safe error logging/action registration; `go test -race` on changed packages. Denied calls do not add billed usage.
- [ ] **Commit** with no provider/deploy setting changes.

## Task 4: one usable protected check and fail-closed unsupported paths

**Modify:** `internal/engine/{engine,prompt}.go`, HTTP principal/route/dependency wiring and cache selection; **create** `httpapi/confidential.go` and tests. Update OpenAPI for only the new implemented API, not the whole later wishlist.

**Proposed routes:**

| Route | Exact operation / authority |
|---|---|
| `GET/PUT /v1/workspaces/{id}/confidential-policy` | Load/change policy; administrator only; increment policy version and audit safe IDs/outcome. No raw vault/key setting in the public DTO. |
| `POST /v1/cases/{id}/protected-context` | Prepare single-turn context and full preview; authorized editor; server loads source and workspace, not caller-selected tenant. |
| `POST /v1/cases/{id}/protected-context/{job}/approve` | Approve returned digest as authorized actor; stale payload returns conflict, not silent regeneration. |
| `POST /v1/cases/{id}/protected-context/{job}/execute` | Acquire grant once, revalidate and execute immutable prepared request. Return `200` with `HumanResult`/receipt on completion, `202 completion_pending` while safe finalization can resume, or `409 execution_uncertain` when inference outcome cannot safely be replayed; no SSE in MVP. |
| `GET /v1/cases/{id}/protected-context/{job}/result` | Retrieve the sealed completed human result as the still-authorized case user until job expiry; never re-run inference or expose the map. |
| `GET/DELETE /v1/cases/{id}/protected-context/{job}` | Tenant-bound status or cancel/revoke/delete. Never return raw mapping. |

Factor proposed `engine.Prepare(ctx, domain.Case)` and `engine.Interpret(prepared, llm.Response)` methods from existing `Predict`. Name their concrete prepared type `engine.PreparedPrediction`; it holds the immutable request, selected local public precedents and interpretation metadata. Preserve standard `Predict` and its request snapshots. The confidential service stores the sealed prepared form and calls the protected raw model once, then the existing interpretation logic. Do not call a legacy cached `Predictor` during protected execution.

- [ ] **Write red HTTP tests:** complete prepare→approve→execute; viewer cannot approve; another workspace cannot read or execute; stale source/policy/role/recipient requires a new preview; duplicate execution and cancel race; missing metering scope does not remove privacy scope. Verify every error path calls the recording provider zero times. Add `TestProtectedPredictVotesOne`, `TestProtectedFallbackDisabled`, `TestProtectedLegacyCacheCannotServe` and `TestPreparedRequestPreservesLegalNarrative`; inspect the final fake-provider request, not only compiler structs.
- [ ] **Write unsupported-path tests:** protected raw media, remote CROSS search, protected filing export, ordinary predict/plan/chat/stream routes not integrated in MVP, ADK/CROSS queries and sensitive MCP output fail before external send. Public ruling-only reads remain available. A workspace/case requiring protected mode cannot evade it with an API key, a legacy endpoint or absent header. Customer-controlled external agents do not receive raw originals through a protected MCP path.
- [ ] **Implement:** new policy and sealed-record stores through existing persistent store dependencies; server-side class resolution for all standard/protected/system model entry points; protected case requirement independent of usage settings. Run the sticky handler privacy check before every cache lookup/legacy dispatch; a raw-client gate cannot prevent a cached raw answer. Select uncached, local BM25 single-turn engine preparation with `Votes=1` and no protected cross-model fallback; reject unsupported constructor settings; reject unsupported regime/path rather than route a protected case through a general fallback. Never mutate legal source facts. Bypass all existing private result/advice caches in MVP and test legacy entries are not read. Keep agent/tool histories tokenized; re-identification happens once inside execution after terminal typed-result validation for the authorized human result, before map deletion. The shared model wrapper never detokenizes responses or tool arguments. Source-based protected broker/filing export is a later increment.
- [ ] **Verify:** `go test -race ./internal/httpapi ./internal/engine ./internal/confidential ./internal/mcpserver ./cmd/server`; ordinary origin/media/source-snapshot tests remain green. Include raw request count and block reason in fixture evidence, without recording secrets in logs.
- [ ] **Commit** the working backend path and updated capability inventory; do not turn on deployment variables.

## Task 5: review UI and proposed settings surface

**Create/modify:** a protected-context case component, API client/types and workspace policy UI under `frontend/src`; keep existing design tokens and read installed Next docs first. This implementation is explicitly future work, not a permission to change the UI in this planning PR.

- [ ] **Write UI tests first:** original-vs-outbound preview, recipient/policy notice, unresolved-span warning, approval disabled while editing/stale, unsupported media explanation, blocked mode, expired grant, permission loss and successful receipt. No raw map in local/session storage, query strings or analytics.
- [ ] **Implement:** show exactly the approved private-context request, not a separate cosmetic redaction preview. Bind approval to returned digest; any edit requires preparation again. Distinguish `preview`, `approved`, `running`, `blocked`, `complete` and `expired`. Use plain capability text: “Protected single check”; do not claim full-agent, image, private-cloud or zero-retention support. Expose only admin-approved workspace policy; deployment keys/recipient verification are not editable secrets on this screen.
- [ ] **Verify:** `cd frontend && npm test && npm run lint && npm run build`; launch a local recording/fake provider for UI wiring only, inspect screenshots at 1280px and 390px, keyboard/focus and errors. A simulated provider must be visibly labelled and never become an accuracy claim.
- [ ] **Exercise an integration failure:** change case facts or policy after preview; the UI must display re-approval, not replay old consent.
- [ ] **Commit** without unrelated frontend cleanup or deployment changes.

## Task 6: privacy, usefulness and efficiency evidence

**Create:** `eval/privacy-boundary/PROTOCOL.md`, source-grounded development/reserved fixtures and manifest, request-capture tests, benchmark code and offline summarizer. Separate deterministic privacy transport tests from live-model utility results.

- [ ] **Freeze the protocol before evaluation:** at least 400 seeded-secret/adversarial transport cases with independent expected spans, including undeclared supported variants and ambiguous names and at least 200 reserved utility cases, disjoint from development by normalized input and with family dependence reported. Record source/label rationale, dataset hash, candidate, tokenizer/compiler/prompt versions, recipient/model settings, random seed, supported/blocked paths and all denominators. Synthetic entities must not be real private customer data.
- [ ] **Run deterministic proof:** `go test -race ./internal/confidential ./internal/httpapi ./internal/mcpserver`; assert zero escapes of declared/supported identifiers across all outbound fields, zero cross-tenant map access, exact legal-field preservation in the final rendered request/narrative (not just an untouched `LegalFacts` struct) and zero raw calls on blocked routes. `go test ./internal/confidential -bench . -benchmem` records local preparation overhead with hardware and input sizes. Proposed target: warm-server p95 ≤ 250ms for ≤ 12,000 runes; it is not an achieved result or SLA.
- [ ] **Select only on development:** inspect false positives, unresolved/blocked cases and legal-field invariants. Any changed countries/codes/values, unapproved destination or privacy escape blocks promotion. Keep all attempts; do not improve a secret detector using the reserved set and still call it blind.
- [ ] **Score utility only after a committed freeze:** check remaining programme budget; use approved gateway only. Interleave baseline/protected over the same reserved cases for three registered repeats; retain every call/error/cost and actual outbound sanitized payload proof without raw private values. Local fake responses do not count. Gate: zero new unsafe confident decisions; preregister a two-percentage-point observed non-inferiority margin for agreement and answer coverage over case-weighted paired repeat means. Publish individual repeats, detector precision/recall against independent span labels and missed/blocked cases. Pipeline zero-escape results do not imply detection recall; ambiguous-name fixtures must record manual declaration/review or a block, not be silently omitted. If repeat variation exceeds that margin or conclusions conflict, mark inconclusive and expand evidence rather than claim population certainty. A privacy block is counted as lost coverage, not a correct legal answer by default.
- [ ] **Publish evidence:** privacy escapes/denominator, detector scope, blocked coverage, correct decisions, unsafe accepts, response errors, p50/p95 preparation/model/end-to-end latency, interactive approval time, tokens and actual cost, plus proof of what was sent. Separate test-transport success from provider-specific deployment validation. Add actual measured figures to `docs/NUMBERS.md` only after they exist; leave targets labelled proposed.
- [ ] **Commit** all original results and rejected variants; no post-test relabel or clean-run replacement.

## Task 7: final-round rehearsal, owner activation and safe rollback

- [ ] **Complete regression checks:** `cd backend && gofmt -l . && go vet ./... && go test ./... && go test -race ./internal/confidential ./internal/httpapi ./internal/llm ./internal/engine ./internal/mcpserver && go run ./cmd/claimscheck`; run frontend tests/lint/build and the offline evidence checks. Read the failing output and fix it before declaring any path supported.
- [ ] **Owner checklist:** provision a dedicated key and verify recipient/provider/region/retention/caching eligibility outside feature PRs. Record capability attestations and expiry without secrets. Do not change workflow/infra in the feature branch; explicitly keep #994's automatic evaluation/provider conflict separate. If required settings are absent, demo only the labelled test harness, not a falsely enabled production mode.
- [ ] **Rehearse four synthetic-data demonstrations:** prepare/preview a seeded identifier; approve and show the actual gated request; attempt stale approval or wrong-workspace access and show zero network calls; attempt a raw scan/tool route and show a clear unsupported block. Demonstrate the legal facts preserved and measured utility/performance only if those evaluations have passed.
- [ ] **Enable only an approved test workspace:** no whole-site default change. Verify the exact deployed revision, protected and ordinary paths, audit receipt and owner recipient settings. State that supported reviewed context still contains confidential legal/manufacturing facts; it is not anonymous data.
- [ ] **Test rollback:** disable protected execution, expire a key/policy and kill a worker. Protected jobs stay blocked/cancelled, mappings expire/delete and no standard-mode fallback runs. Existing standard-mode requests retain their explicitly labelled behavior.
- [ ] **Publish rollout status:** update roadmap from planned only for the paths/settings proven operational. Retain the context preview, capability matrix, frozen evidence and honest demo script. No final-round date is promised by this planning document.

## Plan self-check and handoff

The design maps to tasks 1–7: scope/settings (1), compiler/map/approval (2), final provider boundary/bypasses (3), usable path/denials (4), user approval (5), evidence/efficiency (6), rollout/owner/rollback (7). Media/OCR and multi-turn/tool coverage remain explicitly separate later increments; their denial is part of the first release, not an assertion that they work.

All settings, endpoints, core interfaces, proposed files and evidence gates in this plan are unimplemented. This change authorizes documentation, not execution. Implementation is tracked in [#999](https://github.com/kendramoss/wassup-hackathon-2026/issues/999). Start that delivery item using this spec/plan and retain the current standard-mode behavior until the protected-path tests and activation gates pass.

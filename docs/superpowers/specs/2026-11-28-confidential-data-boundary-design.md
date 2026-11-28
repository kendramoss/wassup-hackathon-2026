# Confidential-data boundary: design and final-round scope

**Status: proposed, not implemented or enabled.** Written 28 November 2026 for #998. This specifies a buildable release, not evidence that the product already hides information. Implementation tasks and test commands are in the [delivery plan](../plans/2026-11-28-confidential-data-boundary.md). Current processing remains as described in [data handling](../../design/data-handling.md) and the [privacy research](../../design/2026-11-28-cloud-agent-document-privacy-research.md).

## 1. Decision and alternatives

**Recommend a reviewed, tokenized context with an enforceable model-egress gate.** Do not attempt to make privacy true through system-prompt instructions or the existing logging sanitizer.

| Option | Benefit | Why it is not the whole solution |
|---|---|---|
| Instructions asking an agent not to reveal PII | Cheap and useful as defense in depth | The model already received the data; instructions can be ignored and do not cover tool/network outputs. Reject as the boundary. |
| Local context preparation, stable tokens, explicit approval and provider-side send gate | Keeps unnecessary declared/detected identities out of the request while preserving legal facts; no extra redaction-model call | Detection is imperfect and business facts remain sensitive. This is the recommended bounded product release, not universal anonymization. |
| Customer-controlled OCR and inference | Addresses customers who cannot share even minimized manufacturing facts with the selected provider | Requires model/OCR quality, infrastructure and operating arrangements. Separate later release, not something a private endpoint alone proves. |

**Trust boundary:** the authorized application backend and its protected store can see originals. The external model receives the approved context. This MVP does not promise that raw data never leaves the customer's device or never enters cloud-hosted application infrastructure.

## 2. Final-round target: one complete, provable path

Build a **non-streaming, single-turn U.S. general-origin check on reviewed text/structured facts**, using local public-precedent retrieval. Keep the existing standard experience unchanged for standard workspaces. A protected case must not silently switch to standard mode.

The final-round increment is complete only when a user can prepare a context, inspect it, approve it, run the check and see an authorized result with a privacy receipt. The actual provider request must match the approved request. Existing `engine.Engine.Predict` can make additional concurrent voting calls when configured for votes. The protected constructor must force `Votes=1` and disable protected cross-model fallback; configuration violating those limits is rejected. Factor request preparation/response interpretation from the existing engine rather than introduce a second origin prompt. Same-recipient retries may resend only the identical approved request within the running lease.

**Explicitly blocked for protected workspaces in the first release:** original image/PDF submission, arbitrary free-form chat/history, plan search, ADK tools, remote CROSS search, embedding queries, SSE/progress streaming, sensitive MCP exports and protected filing export. Return a supported-path explanation before any external send. Here sensitive MCP exports means any private case facts, product/party details, documents, chat/history or re-identified result returned to an external model client; public ruling-only responses are separate. Existing ordinary human access remains governed by authorization; this is not DLP against an authorized user deliberately exporting data outside the product.

This is intentionally narrower than the complete app. It prevents claiming “all our agents are PII-blind” after protecting only one endpoint. Add a path only after its request/response and tool-egress tests pass. Public ruling browsing can remain available because it contains no private case context.

### Data flow

1. Authenticate the user, resolve workspace/case access and load the server-side policy independently of metering. Do not accept tenant/mode claims from an untrusted request header.
2. Read a fixed source version. Extract typed legal facts and user-declared sensitive entities; run bounded local detectors for supported direct identifiers. Mark unresolved free text for human review, not an assertion that it is safe.
3. Use declared entities plus a concrete bounded local detector for supported email/phone/identifier formats; names and ambiguous commercial identifiers require declaration/review, not a promised universal NER. Create random, job-bound opaque tokens for unnecessary identities. Preserve exact countries, tariff codes, process order and numeric/legal facts. Record sensitivity of confidential facts that cannot be removed without changing the answer.
4. Build a sanitized `domain.Case`, retrieve only from the local public corpus, and prepare the exact single-turn model request. Check every provider-visible string/schema against the job's declared/detected private values after final rendering; any remaining known value blocks approval, not just a warning the user can click through. Show the full outbound context, recipient and limitations. No model call is needed to produce this preview.
5. Obtain an authenticated approval for that prepared request, source version, policy version, recipient and purpose. Store the prepared record encrypted; return an opaque grant ID, not a mapping or secret.
6. Execute the immutable prepared request through the raw-client egress gate. Revalidate source/policy/actor/grant and recipient at each provider attempt. If anything changed, require a new preview; do not re-render a different request after approval.
7. Keep model-visible/internal content tokenized. Validate the response and restore approved identifiers once inside execution for the authorized final human view. Seal that terminal human result for authorized result retrieval until job expiry; delete the reversible mapping and revoke the execution grant only after this succeeds. Protected filing export is a later increment, not part of this MVP. Never detokenize a tool query before external search. Unknown/invented tokens cause review, not a vault lookup across tenants.
8. Return a receipt containing operation/result IDs, policy version, recipient logical ID and allow/block outcome. Do not put original values, reversible maps or low-entropy value hashes in logs/audit.

## 3. Proposed controls and settings

**Every name below is a proposal, not an existing environment variable or API.** Configuration references contain no real secrets, hosts or credential-file paths.

| Layer | Proposed setting | Default / required behavior |
|---|---|---|
| Deployment | `CONFIDENTIAL_BOUNDARY=disabled|preview|enforce` | `disabled`. Protected workspace/case requests fail closed when unavailable. `preview` cannot issue a protected provider call; `enforce` permits only approved supported paths. No automatic downgrade to standard processing. |
| Deployment | `CONFIDENTIAL_KEY_REF` | Dedicated owner-provisioned encryption/MAC key reference; missing/invalid key blocks protected preparation/execution. Never reuse an audit, session or webhook key. Separate purposes derive independent keys. |
| Deployment | `CONFIDENTIAL_RECIPIENT_POLICY_REF` | Server-owned allowlist of logical recipient IDs, provider, model and region plus verified handling requirements. Unknown/global recipients are denied unless explicitly approved for the workspace; cross-model fallback is disabled entirely in the MVP, even if that recipient appears in a future policy. A reference is not proof of provider compliance. |
| Deployment | `CONFIDENTIAL_GRANT_TTL_SECONDS` | Proposed 900-second approval lifetime. Expiry checked synchronously; TTL cleanup alone is insufficient. |
| Deployment | `CONFIDENTIAL_JOB_TTL_SECONDS` | Proposed 3,600-second maximum job/map lifetime. Delete the map on successful terminal rehydration; delete all private job material on cancel/expiry, plus expiry enforcement; interrupted-job cleanup is an owner-scheduled backstop. |
| Workspace | `mode=standard|protected` and monotonic `policy_version` | Standard preserves current authorized processing. Only a workspace administrator changes policy. Protected remains sticky on existing protected cases/jobs; changing a setting cannot silently release them. |
| Workspace | `allowed_recipients`, `allowed_purposes` | Initially one approved `origin.single_check` recipient/purpose. Region/model changes invalidate approval. No fallback provider/model is supported in the MVP. Expansion requires a separately bound prepared request and new approval semantics, not reuse of the primary digest. |
| Workspace | `require_preview_approval=true` | Mandatory for protected mode. The caller cannot turn it off per request. Approval actor must still hold the required case role at execution. |
| Workspace | `raw_media=deny`, `remote_tools=deny`, `embeddings=deny`, `sensitive_mcp_exports=deny` | First-release enforced restrictions, not working feature toggles that imply coverage. The product must reject an attempted relaxation until its implementation phase is released. |
| Workspace | `identifier_policy` | Supported entity classes and explicitly declared sensitive spans/fields; unknown free text must be reviewed. Typed legal fields cannot be changed by a broad phone/address regex. |
| Job | `source_version`, `prepared_digest`, `policy_version`, `recipient_id`, `purpose`, `grant_id` | Server-minted and bound together. An IDOR, stale version, changed country/value or altered destination fails before network egress. |

Provide an admin screen explaining these states, not a single misleading “PII safe” switch. Deployment/provider activation belongs to the owner; the feature PR must not edit workflows or Terraform. Owner key/recipient setup can block activation even if feature code is merged.

## 4. Identities, legal facts and storage

### What can be replaced

Declared/detected contact names, emails, phone numbers, account/tax/document identifiers and unnecessary supplier/customer/plant labels. Use random tokens such as `PARTY_7K4...`, stable within the job and uncorrelated across jobs/workspaces. Generate entropy with `crypto/rand`; do not derive public tokens by hashing low-entropy names.

A local detector is assistance, not proof it found every identity. Free-form commercial secrets, unusual identifiers, product-specific details and contextual re-identification remain limitations. Show unresolved spans and obtain reviewed context. Do not call pseudonymization anonymization.

### What must survive

Jurisdiction, country of each relevant production step/material, tariff code and edition, process sequence, material status, value/unit/currency/denominator and uncertainty. Preserve their typed values exactly; do not infer missing facts. Keep a provenance map from sanitized facts to source fields without sending that reverse map to the model.

Named-entity sanctions/scope questions can require real names. Do not fabricate tokens as valid legal evidence: block that protected path or route it through an explicitly authorized processor. Re-identify verified parties inside the authorized final broker/filing export. Never submit placeholders as filing identities.

### Vault and approval

Use a separate encrypted prepared-job store scoped by `(workspace, case, job)`, following the existing separate budget-store pattern rather than putting mappings into public workspace/case JSON. Reuse `envelope.Sealer` for encrypted records with independent key purposes and authenticated context containing workspace/case/job/policy/source version. The application must not need a shared in-memory map or sticky Cloud Run sessions.

Store only the necessary mapping, reviewed legal projection, immutable prepared request, approval state and tokenized intermediate result. Persist the sealed tokenized provider reply before finalization. On completion, atomically store a distinct sealed `HumanResult` and safe status receipt, then destroy the map/prepared private material. If completion persistence fails, retain the sealed map and reply; an authenticated repeat Execute resumes finalization without re-acquisition or inference, while status/result GETs stay read-only; a failure to save even the provider reply leaves an uncertain execution, never an automatic repeat model call or a falsely completed result. Completed-result retrieval rechecks tenant/case role and TTL but never causes another model call. Cancellation/expiry deletes private records including pending/uncertain sealed replies/results; an expired job needs fresh preparation, not automatic replay; safe tombstone/audit IDs can remain. Access is through a tenant-aware service, never arbitrary raw keys from an agent. Atomic execution acquisition compares current case/policy/authorization with grant state in the persistent transaction; a separate unchecked read-then-CAS is insufficient. It prevents duplicate job creation; retries occur within the acquired job and are individually gated. Deletion/expiry invalidates grants and map access even if asynchronous storage TTL has not removed bytes yet. Describe remaining backup/provider retention separately.

First migrate and observe explicit classification across the existing caller inventory while protected activation is prohibited. This observation phase provides no privacy claim. Install/enforce the send gate only after standard, visitor, API-key, health, startup and background caller tests all carry classification; then keep it installed even when protected execution is disabled: every server-side model request must carry a trusted classification (`standard`, `protected` or explicitly authorized public-system purpose). Missing classification denies rather than assuming standard mode. The deployment switch controls whether protected execution is available; it must not remove the guard or erase sticky case requirements. Public health/startup probes need an explicit constant-data system scope and tests.

Use a keyed MAC of the canonical relevant source snapshot as `source_version` unless an existing revision covers every relevant case change. Do not rely on a timestamp or BOM-only revision that misses edits to free-form facts. Re-read and compare the source before execution; include prompt/compiler, recipient-policy and public-corpus revision in the prepared envelope too. The workspace policy stores its required recipient-policy version; compare it transactionally with the prepared version and with trusted immutable in-process recipient config before acquire and at send, so outdated pods fail closed.

The approval digest is a keyed MAC over a canonical prepared envelope, not a public hash of raw PII. Include workspace/case/job IDs, actor authorization scope, source/policy versions, exact request content/schema/token limit/the existing `llm.Seed` constant and recipient identity/model/region/purpose. Reject binary `Parts`, tools and tool-call history before canonicalization in the MVP: `Message.Parts` has `json:"-"`, so naïve request JSON hashing would otherwise omit raw media bytes.

## 5. Where the code changes

| Current seam | Proposed implementation | Verification / first-release limit |
|---|---|---|
| `backend/internal/llm/llm.go` `Model.Generate(context.Context, Request)` | New `internal/confidential` service and raw-model decorator; add a non-retryable `policy_denied` class with safe HTTP mapping | Capture `llm.Fake.Calls()` and serialized transport bodies; the shared wrapper only blocks/flags unsafe responses and never re-identifies content; reject without constructing a paid call. Test every text-bearing field, schemas and binary parts. |
| `backend/cmd/server/main.go` `newBase`, `newEngineModel` | Protect primary raw client and separately constructed fallback, below retry/fallback composition; recipient identity comes from server construction | Wrapping only the `model` variable misses the engine composition. A deny must not become a retry or cross-region fallback. |
| `backend/cmd/server/japan.go` | Gate the optional gateway constructor too; protected Japan remains unavailable until tested | Do not claim the general provider setting covers this override. |
| `backend/internal/engine/engine.go` and `prompt.go` | Factor `Prepare` and `Interpret` from the existing single-turn prediction, preserving the standard `Predict` API | Preview and execute share the same prepared bytes. Standard-mode prompt snapshots stay unchanged. |
| `backend/internal/httpapi/principal.go`, `metering.go`, new privacy handlers | Independently construct mandatory privacy scope after authentication; add admin policy and prepare/approve/execute/status/cancel routes | Workspace identity must not disappear when metering is off. Re-authenticate when MCP re-enters HTTP. |
| `backend/internal/verifier/adk.go`, `caseagent` and `cross` | Inventory and block protected agent/CROSS paths initially; later pass only gated tokenized tool results and public/local search | ADK currently forwards through the shared model adapter, but context propagation needs a regression test. Do not detokenize before outbound tools. |
| `backend/internal/retrieval/vector.go` and `llm/embed.go` | Separate embedding gate before any future protected vector path; first release uses local BM25 | Server currently uses BM25; vector query egress exists in offline/future wiring, not every production request. |
| `backend/internal/mcpserver/server.go` | Deny sensitive protected-workspace outputs before serializing API responses to an external agent | A model-send decorator does not protect returned MCP data from the customer's chosen model. |
| Engine/verifier/advice caches and provider caches | Bypass private application result caches in MVP; register policy-aware isolated caches only in the next phase | A handler-level sticky privacy check combines the workspace default with the `(workspace, case)` requirement (a case cannot weaken its workspace) and runs before ANY engine/verifier/advice cache lookup or legacy route dispatch; the raw-client gate alone cannot see a cache hit. The dedicated MVP path uses uncached preparation/execution. Populate a legacy raw cache fixture and prove protected reads cannot return it. Provider cache/retention is separate owner verification, not achieved by bypassing an app cache. |
| `backend/internal/audit`, HTTP log policy, frontend API/types | Register only safe policy/job/recipient logical IDs and outcomes; preview UI and accurate capability labels | No raw identifiers, hashes of names, context dumps or mappings in logs, audit, metrics labels or browser storage. |

All new provider constructors and HTTP egress must be allowlisted in an AST/import guard, following the existing `logsafe/astguard_test.go` approach. The guard is additional proof, not the runtime policy. Offline public/synthetic evaluators remain explicitly separate; they cannot import a protected-case vault to bypass policy.

## 6. Reliability and efficiency: proposed release evidence

These are **test requirements and proposed performance targets, not measured results**. Record the actual hardware, fixtures, model/provider, token counts and timings when implementing.

| Claim to demonstrate | Experiment and acceptance rule |
|---|---|
| Identified private values do not reach the supported model path | Freeze at least 400 synthetic seeded-secret/adversarial cases across entity classes and context placements. Require zero escapes of declared/supported identifiers in captured provider requests, schemas, logs or private caches, and zero cross-tenant map reads. Labels come from a separate fixture manifest, not compiler output. Include undeclared supported variants and separately report detector precision/recall, missed entities and manual-review outcomes for ambiguous names. This proves only the tested scope, not universal PII detection. |
| Legal facts survive preparation | Exact equality/property tests over countries, tariff editions/codes, amounts/denominators, sequence and missingness before/after tokenization AND in the final rendered request/narrative. Include numeric strings resembling phone numbers and legally necessary names. Any changed legal field blocks release. |
| Privacy does not win by refusing everything | Separately report supported inputs, preparation blocks, model errors, decided correctness, unsafe confident decisions and answer coverage. Freeze at least 200 source-grounded in-scope utility cases distinct from development. Baseline/protected requests use the same source cases, model and settings, interleaved across three registered repeats. Require zero new unsafe confident decisions. Use a preregistered observed non-inferiority margin of two percentage points for case agreement and answer coverage across case-weighted paired repeat means; report each repeat and family dependence. If repeat variation is larger than the margin or conclusions conflict, classify the result as inconclusive and expand evidence rather than enable protected auto-accept. This is an engineering-corpus gate, not a population-accuracy guarantee. Synthetic dependency and stochastic uncertainty remain explicit; expand evidence if inconclusive. |
| No unnecessary remote latency | No extra LLM “redactor” call in MVP. Benchmark local preparation/MAC/map operations separately with `go test -bench`; proposed warm-server p95 preparation target is 250ms for text up to 12,000 runes. Report cold start, interactive approval time and total provider latency separately; do not hide blocked inputs or call this an achieved SLA. |
| Destination and approval cannot be bypassed | Change each bound field, recipient, role, policy, source version and clock; all must deny with zero raw client calls. Test retries, fallback, concurrent workers, restart, expiry, cancellation and a disabled master switch. |
| Existing behavior does not silently change | Standard-mode golden requests/snapshots and full tests remain unchanged. Protected jobs fail closed on rollback; never replay raw legacy cache entries or downgrade to standard. |

Use local fake/recording transports for deterministic privacy tests and label them as such. Utility studies require the approved gateway and preregistered real scoring; never publish fake responses as model accuracy. No paid run occurs while writing this plan. Check the programme ledger and obtain any needed allocation before future experiments. Provider-specific production claims require verification for that deployed adapter too, not extrapolation from a gateway run.

## 7. Expansion after the bounded release

1. Add local/private OCR with page/region preview, image-layer and metadata removal, and a checked rendered output; test hidden text, QR codes, EXIF, signatures and reconstruction risks before allowing protected media.
2. Add ADK/tool turns only with per-turn gated requests, token-preserving history and explicit tool-output provenance; start with local public retrieval. No blanket detokenization in `Generate` responses.
3. Add authorized protected filing export from a verified source snapshot, private MCP exports, embeddings and policy-aware caches with separate recipient and tenant tests. Avoid persisting raw embeddings or unhashed private query keys.
4. Add regional/customer-controlled deployment and provider retention controls after owner prerequisites and adapter tests. Do not describe an encrypted store or private network as private inference.

## 8. Final-round delivery, rollout and rollback

**Build order:** policy/scope and test transports → compiler/vault/prepared-request contract → raw-client gate and supported endpoint → review UI → frozen evaluation/evidence → owner activation in a test workspace. Each stage must finish its tests before expanding the supported path inventory. The delivery plan provides file-level tasks and commands.

A planning estimate is **6–12 focused engineering days for the bounded text path**, plus owner key/recipient setup and evaluation runtime; the estimate is not a committed date or measured delivery result. Backend policy/store/gate/endpoint work is estimated at 4–7 days; preview UI and evidence work at 2–5 more. These are planning assumptions; owner wait time, real-data rights and unexpected integration work add time. Media and full agent/tool coverage are outside that estimate. The final-round date is not recorded here, so the plan cannot honestly promise completion before a specific judging session.

For judging, show four explicit panels/actions: original synthetic identifiers, actual outbound-context preview, a captured/gated successful request and a blocked raw-media/tool attempt. Show privacy/utility/latency evidence only after it exists. Until then label the flow a design, not implemented protection. Do not upload confidential company documents to manufacture a demo.

Roll out to an explicit test workspace only after the evidence gates and owner configuration are met. If a gate, policy store, key, recipient assertion or approval fails, disable protected execution and keep its cases/jobs blocked. Standard mode remains separately labeled and cannot be an automatic fallback. Preserve audit IDs and failed test evidence without retaining raw secrets in logs.

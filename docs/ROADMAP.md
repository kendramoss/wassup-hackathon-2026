# Product roadmap

Updated 28 November 2026. Unde is a working hackathon prototype, not a certified enterprise service. This roadmap separates what is already built from unfinished product work and owner-operated infrastructure. It does not promise delivery dates, customers or pilots.

**Status:** **shipped** means implemented; **partial** means a named part works but the full capability does not; **planned** means not implemented. Activation is separate: a feature can be shipped but off. The [current committed deployment switches](../.github/workflows/deploy.yml) and linked release evidence take precedence over older default-off design/status tables. [Scope](scope-and-jurisdiction.md), [measured results](NUMBERS.md) and the [security implementation reference](api/SECURITY.md) provide the detail; a switch alone is not evidence of complete market coverage.

## Built today

These are foundations, not promises to build them again. Backend-only workflows are identified explicitly.

| Capability | What is implemented | Boundary or remaining work |
|---|---|---|
| Origin prediction and production-move plans | U.S. origin assessment, precedent retrieval, suggested manufacturing moves, duty/operating-cost comparisons and broker drafts | Suggestions, not rulings. Family, market and input-quality limits remain; a broker confirms before filing. [Details](DETAILS.md) |
| Market-specific checks | Market selection plus scoped non-U.S. list-rule checks | Rules checks are not precedent-based models or general coverage of every product. [Jurisdiction scope](scope-and-jurisdiction.md) |
| Trade preferences | ATIGA, extended USMCA preferential rules, RCEP calculator and the Japan agreement model | ATIGA and extended USMCA are enabled. AKFTA/ACFTA calculators are built but not enabled. Unsupported provisions and further agreements remain future work; see the coverage section below. |
| Deterministic rule layer | Part 102 tariff-shift and textile-origin checks beside model reasoning; incomplete-fact, packing-stage and rule-range fixes | Textile rules are enabled as a display-only cross-check, not an automatic decision of the most important process. [Evaluation](../eval/RESULTS.md) |
| Document and parts intake | Text, CSV/Excel, images and scanned PDFs become editable drafts; image extraction uses the development-selected v2 prompt | The [larger synthetic-image study](../eval/challenges/media/summary.json) is complete. Real photos, handwriting and engineering drawings are not thereby validated. Raw model input is not presently anonymized. [Multimodal input](MULTIMODAL.md) |
| Agents, REST API and MCP | Case assistance, structured API tools, MCP access and ADK integration | Agents and their permitted model providers can receive authorized case context. Access control is not a guarantee that an agent cannot see company information. [MCP](MCP.md) |
| Broker review and filing preparation | Revocable frozen-draft review links; backend queue, assignments, discussion, sign-offs, filing packages and status tracking | The queue/collaboration backend is enabled. Complete staff/broker screens, tracked letter editing and notifications remain partial. A filing package is not submission to CBP. |
| Supplier declarations | Enabled backend for expiring supplier links, statements, checked documents, validity and declaration facts in prediction ([implementation #916](https://github.com/kendramoss/wassup-hackathon-2026/issues/916)) | Supplier/requester experience, expiry communication and operator validation remain unfinished. [Owner check #952](https://github.com/kendramoss/wassup-hackathon-2026/issues/952) |
| ERP file import | Enabled CSV/JSON import, assembly grouping, validation and repeatable re-import by ERP ID | Not live SAP/Oracle/NetSuite synchronization or plan push-back. |
| Tariff and AD/CVD exposure checks | Enabled tariff-watch backend, documented tariff-action handling and scope-first AD/CVD screening | Snapshot/source and contested-case limits remain. Scheduling, richer screens, additional datasets and wider coverage are separate work; possible exposure is not a legal scope ruling. |
| Accounts and workspace isolation | Google sign-in, workspace roles, case isolation, API keys and authenticated private operations | Multiple workspaces per person and transfers between them are not implemented. [Roles](design/roles-and-strict-auth.md), [isolation](design/case-isolation.md) |
| Workspace administration | `/settings/workspace`, member/role management, workspace keys, retention, budget/use and audit views; SSO/SCIM setup controls | SSO/SCIM configuration screens are built; a complete activated enterprise-login/provisioning flow and group mapping are not the same thing. [SSO](design/sso.md), [SCIM](design/scim.md) |
| Webhook delivery backend | Signed callbacks, endpoint validation, retries, rotation and delivery records ([implementation #915](https://github.com/kendramoss/wassup-hackathon-2026/issues/915)) | Built but not activated in the current deployment; owner secret/retry scheduling and delivery hardening remain. [#951](https://github.com/kendramoss/wassup-hackathon-2026/issues/951) |
| Usage and model-cost controls | Enabled workspace metering, budgets, hard caps and pre-search estimates; role-scoped workspace keys share the budget checks | Payments, invoices and complete billing coverage are not implemented. |
| Audit, retention and deletion | Enabled audit trail/export, case-retention controls, linked cache/document deletion and checkpointed audit retention | `CACHE_PURGE` is enabled. Purge scheduling and legacy unlinked/provider/backup copies remain operator or contractual work. [Retention #875](https://github.com/kendramoss/wassup-hackathon-2026/issues/875) |
| Service health | Enabled status API and documented service objectives | Not a hosted public status service, monitored contractual SLA or completed on-call operation. [Status design](design/sla-and-status.md) |
| Security foundations | Authentication, tenant-aware access, rate limits, a restrictive log policy and available cache-blob encryption | Dedicated deployments, regional infrastructure and KMS activation are owner work. These controls do not implement pre-model redaction or private inference. [Data handling](design/data-handling.md) |
| Evaluation and refresh tooling | Source-linked larger capability studies, frozen inputs/candidates, retained failures and offline checks; corpus refresh, monthly re-scoring and drift tooling | Synthetic checks are not independent legal decisions. Scheduled refresh/evaluation, automatic promotion into retrieval and planner drift coverage remain partial. [Results](../eval/RESULTS.md), [refresh](REFRESH.md) |

## Next product work

These are future capabilities or the unfinished portions of shipped foundations. Privacy work is a priority before offering a confidential-document mode to customers; it is not an external-specialist gate for the hackathon demo.

| Priority | Product promise | Status and completion boundary |
|---|---|---|
| First | **Protected document intake** | **Planned.** Select pages/regions, identify unnecessary PII and commercial identifiers, and redact or tokenize them before any external model/OCR receives them. Provide an outbound-context preview. A failed local/private intake must not silently send the original to cloud OCR. This is not built today. |
| First | **Controlled context for agents and tools** | **Planned.** Enforce workspace policy on prompts, files, embeddings, tool inputs/results, fallback calls and MCP exports. Use stable tenant-bound tokens and authorized re-identification where appropriate. Retain the countries, tariff codes, processes and origin/value facts needed for valid reasoning. No blanket promise of anonymization. |
| Next | **Complete enterprise identity** | **Partial.** Finish and verify user-facing SSO entry/callback flows, IdP onboarding, SCIM groups and group-to-role mapping. Reuse the shipped backend and admin setup; do not count those as future features. Owner keys and activation remain separate. |
| Next | **Finish reviewer and supplier journeys** | **Partial.** Complete the queue, broker and supplier/requester screens; tracked draft edits, notifications, expiry reminders and automatic follow-up checks. Reuse shipped review/declaration APIs. |
| Next | **Stable integration contract and SDKs** | **Partial.** The API, import tools and webhook backend exist. Add a documented compatibility/deprecation policy, maintained SDKs, delivery redelivery/auto-disable controls and customer-managed audit-export delivery. |
| Next | **Multiple workspaces and billing** | **Planned beyond current controls.** Multi-workspace membership, authorized case transfers, operator visibility, payments/invoices and clearly defined metering for every supported caller. Existing workspace budgets are already shipped. |
| Next | **Validation on representative private-document workflows** | **Planned.** With explicit data rights, evaluate real-document intake and protected-context quality, capture outbound requests, and test prompt injection/tool egress. Retain failed privacy and accuracy cases. Public/synthetic demonstrations are not proof of private-document readiness. |

### Acceptance criteria for the planned privacy mode

- **Protect before the boundary.** Photos/PDFs may require local or explicitly approved private OCR; masking text after cloud extraction does not hide the original scan from that service.
- **Preserve legal meaning.** Company names are not the same as origin countries, and prices/relationships can be confidential without being PII. Minimize irrelevant identifiers; use approved processing when a named party or other sensitive fact is legally necessary. Test that tokenization does not change origin conclusions. Restore verified real party details inside the authorized broker/filing export boundary; do not submit placeholder tokens as filing identities.
- **Make disclosure enforceable.** Let an administrator define allowed recipients, fields and exceptions; show the user what leaves the selected boundary. Do not rely only on a prompt telling the agent to be discreet.
- **Verify provider handling.** Document actual model/OCR/embedding recipients, regions, training permissions, retention, caching, logging and deletion limitations. No-training, encryption at rest and zero retention are different claims. Do not advertise unverified provider settings.
- **Keep evidence without another leak.** Record policy decisions and test results without storing raw sensitive content in logs or audit exports. Keep re-identification mappings protected and scoped to the workspace.

This requires a deliberate revision of the present [model-input policy](design/data-handling.md), not importing the logging sanitizer into inference paths. See the [Altana/cloud-agent privacy research](design/2026-11-28-cloud-agent-document-privacy-research.md) for sources, current code boundaries and unknowns.

## Later product expansion

| Capability | New work still required |
|---|---|
| Private or customer-controlled execution | **Planned.** A private/local document-processing and model option for customers whose sensitive facts cannot enter an approved shared service. Customer-owned storage, keys, model endpoints and operational responsibility must be explicit; a VPC endpoint alone is not a model-blindness guarantee. |
| Self-service dedicated/regional deployment | **Partial foundations only.** Tenant/region guards and cache envelope-encryption code exist. Productize provisioning, regional routing, approved model/fallback selection, customer keys, monitoring and lifecycle management on top of owner-operated infrastructure. |
| Live ERP/PLM integration | **Planned beyond import.** Live connectors, plants/routings synchronization, conflict handling and approved plan push-back. File import does not satisfy this promise. |
| Broader document and quota support | **Planned.** Engineering drawing/datasheet understanding validated on representative documents; user-level limits across devices and distributed caps for the remaining per-instance classes. |
| Compliance-team template builder | **Planned.** Let teams safely author/test family templates and expand product coverage when source evidence supports it. Existing family templates are already built. |
| Operational product layer | **Partial.** Refresh/re-score/drift commands and status/audit endpoints are built. Add operational screens, planner drift checks, automatic indexing/promotion controls and supported scheduling/alerting. Infrastructure scheduling itself is owner work, not a missing API. |

## Coverage to explore, not current promises

- **Further USMCA provisions:** automotive, chemistry/textile exceptions and remaining product-specific provisions/correlations beyond the shipped preferential-rule subset. Keep preferential qualification separate from U.S. marking origin.
- **Further agreements:** remaining AKFTA/ACFTA provisions, ACFTA 3.0, direct-consignment and other agreement-specific requirements. The existing ATIGA/RCEP/AKFTA/ACFTA calculators are not future inventions or blanket treaty coverage.
- **Additional authorities and precedent:** EU BOI and further precedent-based models where usable primary decisions exist. Existing non-U.S. list-rule checks remain narrower than complete market coverage.
- **Direct filing:** the prepared ruling-request package and filing-status backend are shipped. Submission integration depends on an authorized supported channel; there is no implemented live CBP eRulings submission.

New coverage needs regime-specific evidence before a coverage claim. Formal specialist review remains optional post-hackathon work, not a new release gate ([#924](https://github.com/kendramoss/wassup-hackathon-2026/issues/924)). A broker or authority still decides real filings.

<a id="owner-and-infrastructure-steps-to-switch-these-on"></a>

## Owner setup and governance, not new feature implementation

The original audit-key, required-sign-in and bearer-check activation steps are already complete. The table below is the remaining setup, not a request to repeat those rollouts.

| Remaining responsibility | Existing foundation / tracking |
|---|---|
| Dedicated tenant infrastructure | Application guard exists; separate services, database, identity and host routing remain. [#872](https://github.com/kendramoss/wassup-hackathon-2026/issues/872), [design](design/tenant-isolation.md) |
| Regional processing and customer keys | Application region/cache-encryption support is not regional deployment or database CMEK. Provision and verify the database, services, model endpoints, fallback, logs and keys. [#873](https://github.com/kendramoss/wassup-hackathon-2026/issues/873), [#874](https://github.com/kendramoss/wassup-hackathon-2026/issues/874) |
| Retention and service operations | Schedule purge/orphan sweeps; handle legacy unlinked copies; configure uptime/SLO alerts and any hosted status/SLA offering. Activation of retention/cache purge/status is already done. [#875](https://github.com/kendramoss/wassup-hackathon-2026/issues/875), [#885](https://github.com/kendramoss/wassup-hackathon-2026/issues/885) |
| Integration activation and checks | Webhook secret and retry scheduling; validate supplier-link/document storage operation. Do not promise the whole supplier journey is complete. [#951](https://github.com/kendramoss/wassup-hackathon-2026/issues/951), [#952](https://github.com/kendramoss/wassup-hackathon-2026/issues/952) |
| Refresh and evaluation automation | Commands exist; schedule and safely promote refreshed data. Resolve the legacy automatic Vertex evaluation separately from gateway-only experiment policy; the canceled run is not evidence. [#943](https://github.com/kendramoss/wassup-hackathon-2026/issues/943), [#994](https://github.com/kendramoss/wassup-hackathon-2026/issues/994) |
| Certification and contractual commitments | The [SOC 2 / ISO controls map](compliance/soc2-iso27001-controls.md) exists. Policies, legal/provider terms, external audits and an observation period are not certificates or SLAs already delivered. |

## What we can say in the final-round demo

Show public or clearly synthetic documents. Explain that cloud AI processes authorized inputs; existing access/log controls do not conceal those inputs from the model. Present protected intake and private execution as planned capabilities, not a completed privacy guarantee. Competitor federation claims are not proof that its agents cannot read company data either.

In every phase: Unde suggests, a licensed broker or authority decides; every published number has a source in [NUMBERS](NUMBERS.md); no customer adoption, security certification or contractual guarantee is implied.

# Company documents and cloud agents: privacy research

Reviewed 28 November 2026 for the product roadmap. Documentation and proposals only; no privacy feature, provider setting or deployment was changed.

## Bottom line

The competitor is treated as **Altana**, the trade/supply-chain platform already referenced in [DETAILS](../DETAILS.md#competitors-credited). Altana's own platform page explicitly describes an AI agent and a federated data architecture. That does **not** establish that company identities, PII or document contents are hidden from its models before inference.

Our current application also must not be described as model-blind. Authorized facts and uploaded images/PDFs can be sent to a cloud model as supplied. Existing authentication, tenant isolation, log filtering and storage encryption solve different problems. A protected-input mode is a new product feature, not something the present log policy already provides.

## What Altana actually documents

Primary source: [Altana platform](https://altana.ai/platform), substantive HTML retrieved directly after the page-to-text fetch returned no body. Short first-party statements:

| Topic | Published statement | What it supports |
|---|---|---|
| Agents | “Altana's 24/7 agent makes every product trade ready” | Altana does market an AI agent, not merely a passive database. The page also describes automated customs-brokerage workflows. It does not disclose the complete agent/model runtime. |
| Federation | “Participants retain control of their own data, as a federated data model safely aggregates insights while keeping sensitive data and IP fully secure.” | A vendor claim about data control and federated insights, not proof of pre-model redaction. |
| Network learning | Customers integrate data into “a dedicated spoke”, with insights aggregated “back to the central hub” | A published description of its network architecture. It is not a blanket promise that customer data is never used for any learning or that an agent cannot read authorized content. |
| Security documentation | The page links to its [Trust Center](https://trust.altana.ai/) | A route for security/contract due diligence. The trust-center contents and customer-specific agreements were not audited in this bounded research. |

The platform links to a [privacy notice](https://altana.ai/privacy-notice) and a [federated-model explanation](https://altana.ai/resources/data-sovereignty-federated-model). Those links were discovered in the primary HTML but their substantive contents were not retrieved in this pass.

**Not established:** default PII masking; whether an agent sees an original uploaded document; which third-party model receives which fields; provider retention, residency and training terms; whether particular controls are mandatory or customer-configurable. Do not fill these gaps with assumptions. Earlier guessed `/security` and `/trust` routes returned 404, and `/privacy-policy` yielded only a legal-page heading. Those failed routes are not evidence that Altana lacks security controls. Secondary summaries were not used as proof.

Federated learning is also not the same statement as “no model training on any customer-derived information.” Altana explicitly describes shared insights. Its actual contractual permissions and implementation require separate verification.

## What the current application sends

These are code-backed facts, not an inspection of live provider settings. Model visibility means readable content is processed during authorized inference; it is not a claim that transport is unencrypted, another tenant can read the case or a breach occurred.

- **Origin facts:** the engine renders user facts and precedent excerpts into a model request ([prompt construction](../../backend/internal/engine/prompt.go), [generation](../../backend/internal/engine/engine.go)).
- **Pasted documents:** the text extractor sends document paragraphs to the model ([extractor](../../backend/internal/extract/extract.go)).
- **Photos and scanned PDFs:** raw media bytes become inline model parts; clearing the handler's buffer afterward does not undo cloud processing ([media extractor](../../backend/internal/extract/media.go), [upload handler](../../backend/internal/httpapi/media.go)). Names, addresses, signatures and commercial details visible in the pixels can therefore reach the model.
- **Case agent:** case context, questions/history and authorized tool results can enter the model context ([case-chat boundary](../../backend/internal/httpapi/ask.go), [agent tools](../../backend/internal/caseagent/agent.go)). A customer-controlled MCP/ADK client introduces that client's own model-provider boundary too; API authorization does not make its recipient model blind.
- **Embeddings:** when vector retrieval is used, case-derived query text can reach an embedding model ([vector retrieval](../../backend/internal/retrieval/vector.go)). This is conditional, not a claim that every request uses vector retrieval.

The [current data-handling policy](data-handling.md) explicitly preserves authorized model inputs, including supplier names, costs, countries and production facts. Its log helper is explicitly **not a PII detector**. Do not repurpose it as an input sanitizer: a new privacy mode needs its own policy, legal-fact preservation and tests.

The committed deployment configuration selects Vertex AI and global model locations for the ordinary application path. Japan also has an optional gateway construction path in [server/japan.go](../../backend/cmd/server/japan.go); whether an externally configured live service uses that override was not inspected. Do not claim every possible production path is Vertex-only. No environment secrets were read.

Existing access control, workspace isolation, retention/cache deletion and available cache encryption are useful controls, but none establishes pre-inference redaction or private/on-device inference. In particular, `CACHE_PURGE`, supplier declarations, textile rules and tariff watch are enabled in the current committed deployment configuration; older design/status tables describing them as off are stale. Code/config inspection is not an audit of provider retention settings or a new live activation test.

## Google Cloud terms: the relevant distinction

Primary reference: [Vertex AI generative-AI data governance](https://docs.cloud.google.com/vertex-ai/generative-ai/docs/data-governance).

Google documents that customer data is not used to train or fine-tune its managed models without permission or instruction. **That does not mean the model never receives the request.** Inference necessarily processes the supplied context.

Retention, abuse monitoring, caching, grounding and regional serving have feature-specific conditions. A no-training statement is not proof of zero retention or processing solely in a requested geography. Our actual project configuration, contractual eligibility and any exceptions were not audited here. Do not advertise zero retention, regional-only inference or private-model processing based on this research alone. Consult the current provider documentation and verify the precise services and features before making those promises.

The Google page was retrieved through a summarizing fetch. This note deliberately avoids applying unrelated model-provider provisions or quoting unverified numeric retention periods as this application's current settings.

## Roadmap recommendation

**Yes: add a confidential-data boundary, rather than abandon cloud agents or claim agents already cannot see company information.** PII is only part of the issue: supplier relationships, unit prices, formulas and manufacturing routes can be commercially sensitive without identifying a person.

1. **Protected intake, before the first external model/OCR call.** Classify documents, select pages/regions and remove unnecessary identities/metadata. Use local or explicitly approved private OCR where the promise requires it. Sending a full scan to cloud OCR and masking the resulting text is not pre-cloud protection.
2. **Minimum-necessary context and stable tokens.** Replace irrelevant party/contact identifiers with workspace-bound tokens; keep re-identification mappings in an authorized store. Preserve legally material countries, tariff codes, processes and values, or compute suitable aggregates privately. Where a named entity is legally necessary, require an approved processing route rather than silently deleting the fact. Broker and filing drafts must restore verified real party details inside the authorized review/export boundary; tokens are not valid substitutes in a real filing.
3. **Enforce the boundary for agents and tools.** Apply policy to prompts, files, embeddings, tool inputs/results, fallback calls and MCP exports. Provide an outbound-context preview and workspace policy. Do not let an agent or a failed redaction step silently bypass it.
4. **Verify provider policy and deletion.** Inventory recipients/subprocessors, approved regions, training/retention terms, caching and logging. Keep original/derived/provider copies distinct. Choose contractual/private-execution options where minimization is insufficient; encryption at rest is not private inference.
5. **Test both privacy and usefulness.** Capture outbound requests to verify the protected fields are absent; exercise scans and hidden metadata, adversarial document instructions and tool egress; measure that origin-critical facts and result quality survive. Record policy decisions without storing another sensitive payload in the audit log.

These are **planned** capabilities, not present protections, compliance certification or a guarantee of anonymization. Customer-specific deployment, dedicated storage and KMS setup remain distinct owner/contract tasks.

## Final-round presentation

Use public or clearly synthetic documents for the demo. State the present boundary honestly: cloud AI processes authorized inputs; the app restricts access and logging; protected intake/private execution are roadmap work. A credible story distinguishes what the product does today, what the provider contract promises and what the planned privacy mode will enforce. Do not say “company documents are hidden from agents” until the architecture and tests demonstrate that specific claim.

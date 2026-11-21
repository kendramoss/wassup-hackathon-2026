# From prototype to enterprise product

Unde is a hackathon prototype. This is the list of what we would build to make it a product a manufacturer's trade-compliance team could run on. It builds on what already exists; the design notes linked in each row say exactly what is there today.

How to read it:
- **Status:** *shipped* (in the prototype today), *in progress* (an open change is adding it), *planned* (not started). Nothing here is a commitment to a date.
- **Effort:** S (days), M (a few weeks), L (a quarter or more), for a small team.
- **Removes:** the current limitation the item addresses, from the README, [DETAILS.md](DETAILS.md#limitations) or the design notes.
- No customers, pilots or adoption are implied. There are none.

## Now: what the prototype already has

These are the foundations the later phases build on. Each is built, but most are off by default or exercised only in tests, as the linked notes say.

| Item | What it is | Why it matters to an enterprise | Status |
|---|---|---|---|
| Google sign-in and workspaces | Sign in with a Google account; share cases inside one workspace | Cases belong to a team, not a browser | shipped |
| Roles (viewer, editor, admin) | Per-workspace roles, read from the store on every request; `STRICT_AUTH` turns off anonymous use ([roles-and-strict-auth.md](design/roles-and-strict-auth.md)) | Least privilege; reviewers who cannot change a case | shipped (backend only; no role UI) |
| Case isolation | One request principal, existence-hiding 404s, no trust in client headers ([case-isolation.md](design/case-isolation.md)) | One team cannot see another's products | shipped |
| Audit trail | Append-only, tenant-scoped events with a hash chain, `verify`, and retention ([audit-trail.md](design/audit-trail.md)) | Evidence of who did what; input to a SOC 2 program | shipped (off unless a key is set) |
| Log policy | A closed allowlist for log fields; case data never logged ([data-handling.md](design/data-handling.md)) | Product data does not leak into logs | shipped |
| API and API keys | OpenAPI-described REST API, per-key access, batch predict, BOM import from CSV or Excel ([openapi.yaml](api/openapi.yaml)) | ERP and sourcing tools can call it | shipped |
| Ruling watch | Re-check saved cases against rulings CBP issued later | Origin answers go stale as CBP rules | shipped (needs a corpus rebuild to see new rulings) |
| Operating-cost mode | Labour, logistics, capex and retooling over a horizon, with lead-time and order checks ([modelled-operational-costs.md](modelled-operational-costs.md)) | Plans priced on more than duty | shipped (optional) |
| Claims check | Every headline number recomputed from committed files (`go run ./cmd/claimscheck`) | Numbers a buyer can audit | shipped |

## Next: 0–3 months

The work that stands between the prototype and a first paid pilot: identity, tenancy, an integration surface, and a person in the loop.

| Item | What it is | Why an enterprise needs it | Removes | Effort | Status |
|---|---|---|---|---|---|
| SSO with SAML and OIDC | Sign in through the customer's identity provider (Okta, Entra ID, Google Workspace) | Security teams require their own IdP, MFA and offboarding | Google sign-in is the only identity; "company sign-in would need work" (README) | M | planned |
| SCIM provisioning | Users and groups created, changed and removed from the IdP, mapped to roles | Access ends when someone leaves, without a manual step | Membership is invite-by-email only (roles-and-strict-auth.md, section 2) | M | planned |
| Role UI | Role display and selector, remove and leave, viewer-safe screens, a sign-in-required state | Roles exist in the backend but nobody can see or manage them | "Nothing in the frontend uses roles yet" (roles-and-strict-auth.md, section 5) | S | planned |
| Multiple workspaces per user | A user in more than one workspace, with case moves between them | Business units and brokers work across teams | A user is in at most one workspace; case reassignment is not supported | M | planned |
| Public API, versioned | A stable `/v1` contract with deprecation rules, SDK examples and webhooks for long jobs | ERP and PLM integrations need a contract that will not move | The API exists but carries no stability promise | M | planned |
| API rate limiting | Per-key limits shared across instances | One integration cannot starve the others | Limits are per instance today (D57) | S | in progress |
| MCP server | The origin, plan and draft tools exposed to the customer's own agents | Teams already run assistants and want Unde as a tool | No agent-facing interface | S | in progress |
| Multimodal input | Read a product from drawings, photos and PDF datasheets, not only text and BOM files | Engineering facts live in drawings and spec sheets | Input is typed text or a BOM file | M | in progress |
| Human-in-the-loop review queue | Below-bar and out-of-scope cases land in a queue with an assignee, a decision and a note, all audited | Someone accountable signs off before a plan is acted on | "Needs expert review" is a label, not a workflow | M | planned |
| Broker collaboration | Invite an outside broker to one case with comment and approve rights; tracked edits to the draft letter | The broker is the one who files; today the draft leaves the product as a file | The draft is exported and the conversation moves to email | M | planned |
| Audit log export | Scheduled export of the audit trail to the customer's SIEM or bucket, with the chain head anchored outside our datastore | Customers keep their own evidence; anchoring guards against a datastore admin | "Does not give immutability"; head export is listed as hardening not done (audit-trail.md, section 8) | S | planned |
| Admin console | Workspace settings, members, keys, usage, audit search and retention in one screen | Admins should not need the API or an operator | Admin actions are API or operator-token only | M | planned |

## Later: 3–9 months

Data control, compliance evidence, deeper integrations, and the coverage a global manufacturer will ask about.

| Item | What it is | Why an enterprise needs it | Removes | Effort | Status |
|---|---|---|---|---|---|
| Tenant isolation by deployment tier | Logical isolation for most tenants; a dedicated project and database for those who require it | Large buyers and regulated firms ask for physical separation | All tenants share one Firestore database | L | planned |
| Data residency | Run the app, the store and model calls in a chosen region, for example Singapore or Tokyo for ASEAN and Japan teams | Product data may have to stay in a region | Everything runs in `us-central1`; the Gemini 3.8 and 3.5 Flash models are served from `global` only for this project (D88) | L | planned |
| Customer-managed keys (CMEK) | Case data, caches and audit events encrypted with the customer's Cloud KMS key, revocable by them | Customers control and can cut off access to their data | Google-managed encryption only | M | planned |
| Retention and deletion controls | Per-workspace retention for cases, chats and audit events; deletion that also clears the derived prediction and check caches | Data kept no longer than policy allows, and gone when deleted | Deleting a case leaves its content-keyed caches (data-handling.md, item 7); only the audit trail has a retention setting | M | planned |
| SOC 2 Type II, then ISO 27001 | Policies, controls and evidence collection, a readiness assessment, then the audit period | Procurement will not start without a report | The audit trail is one technical input; no control is attested (audit-trail.md, section 11) | L | planned |
| ERP connectors | Pull BOMs, plants and routings from SAP S/4HANA, Oracle and NetSuite; push the chosen plan back as a change request | The BOM already lives in the ERP; re-keying it is error-prone | BOMs come in by file upload or a custom API client | L | planned |
| More product families | Plan templates beyond the three, starting with the families a pilot brings; a template builder for compliance teams | A manufacturer's catalogue is wider than three families | "Three product families have plan templates" (README) | M | planned |
| Continuous evaluation | A post-cutoff planner test set built from CBP rulings after 25 Sep 2026; calibration metrics; monthly re-scoring on new rulings with drift alerts | Buyers need evidence that accuracy holds over time | "The planner is the least tested part"; confidence is not calibrated and no calibration statistic is computed (DETAILS.md) | M | planned |
| Automatic corpus refresh | Fetch and index new CBP rulings on a schedule so the ruling watch and retrieval stay current | Precedent changes; a stale corpus gives stale answers | The watch sees new rulings only after a manual rebuild | S | planned |
| SLAs and status | Uptime and response-time targets, a status page, a support channel, and a regional model fallback | A tool in the approval path must be reachable | Best-effort hackathon hosting; the fallback model shares the primary's outage (D88) | M | planned |
| Usage-based pricing and cost controls | Metered predictions and plan searches, per-workspace budgets and caps, and a spend estimate before each search | Finance needs predictable spend; admins need a ceiling | A per-search estimate exists (D87), but there is no per-tenant budget or bill | M | planned |

## Exploring

Larger scopes we would only start with a qualified customs professional and a separate evaluation, as [scope-and-jurisdiction.md](scope-and-jurisdiction.md) (section 7) requires.

| Item | What it is | Why an enterprise needs it | Removes | Effort | Status |
|---|---|---|---|---|---|
| EU Binding Origin Information | EU non-preferential origin with BOI decisions as precedent | Manufacturers ship to more than one market | U.S. origin only | L | planned |
| USMCA regional value content | Encoded product-specific rules and RVC calculations, separate from marking origin | North American flows depend on qualification, not only origin | RVC is not computed; Part 102 here is marking origin only | L | planned |
| Preferential origin under FTAs | Qualification checks for trade agreements such as RCEP and ATIGA, with supplier declarations | ASEAN manufacturers claim preference on most shipments | No preferential-origin support of any kind | L | planned |
| Other customs authorities | Origin questions for authorities such as Japan, Korea and the UK | One tool across a manufacturer's markets | No non-U.S. jurisdiction is encoded | L | planned |
| E-filing of ruling requests | Submit the broker-approved request through CBP's eRulings channel and track the answer | Removes a manual step and keeps the answer on the case | The draft is a letter the broker files by hand | M | planned |
| Deterministic rule layer | Tariff-shift and textile rules in tested code beside the model | Auditable answers for rule-based regimes | The rules live only in prompts (scope-and-jurisdiction.md, section 2) | L | planned |

## What stays true in every phase

- Unde suggests; a licensed customs broker or a CBP ruling decides.
- A new number goes into [NUMBERS.md](NUMBERS.md) with its source before it appears anywhere.
- A new jurisdiction or regime gets its own held-out evaluation before any coverage claim.

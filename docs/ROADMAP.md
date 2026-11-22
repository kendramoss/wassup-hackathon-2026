# From prototype to enterprise product

Unde is a hackathon prototype. This is the list of what we would build to make it a product a manufacturer's trade-compliance team could run on. It builds on what already exists; the design notes linked in each row say exactly what is there today.

How to read it:
- **Status:** *shipped* (built and in the prototype today), *in progress* (an open change is adding it), *planned* (not started). Nothing here is a commitment to a date.
- **Built is not the same as switched on.** A shipped item can be built and tested but off on the live site, undelabs.dev. The "Now" table says so where that is the case; [api/SECURITY.md](api/SECURITY.md#as-built-vs-as-deployed-21-nov-2026) has every switch with its deployed value.
- **Effort:** S (days), M (a few weeks), L (a quarter or more), for a small team.
- **Removes:** the current limitation the item addresses, from the README, [DETAILS.md](DETAILS.md#limitations) or the design notes.
- No customers, pilots or adoption are implied. There are none.

## Now: what the prototype already has

These are the foundations the later phases build on. Each is built, but most are off by default or exercised only in tests, as the linked notes say.

| Item | What it is | Why it matters to an enterprise | Status |
|---|---|---|---|
| Google sign-in and workspaces | Sign in with a Google account; share cases inside one workspace | Cases belong to a team, not a browser | shipped |
| Roles (viewer, editor, admin) | Per-workspace roles, read from the store on every request; role display, role management and viewer-safe screens in the UI; `STRICT_AUTH` turns off anonymous use ([roles-and-strict-auth.md](design/roles-and-strict-auth.md)) | Least privilege; reviewers who cannot change a case | shipped; on undelabs.dev roles apply to signed-in users, but required sign-in (`STRICT_AUTH`) is not enabled, so anonymous use is on |
| Case isolation | One request principal, existence-hiding 404s, no trust in client headers ([case-isolation.md](design/case-isolation.md)) | One team cannot see another's products | shipped |
| Audit trail | Append-only, tenant-scoped events with a hash chain, `verify`, and retention ([audit-trail.md](design/audit-trail.md)) | Evidence of who did what; input to a SOC 2 program | built and tested; **not enabled on undelabs.dev** (no audit key is set in production, so it is off) |
| Log policy | A closed allowlist for log fields; case data never logged ([data-handling.md](design/data-handling.md)) | Product data does not leak into logs | shipped |
| API and API keys | OpenAPI-described REST API, per-key access, batch predict, BOM import from CSV or Excel ([openapi.yaml](api/openapi.yaml)) | ERP and sourcing tools can call it | shipped |
| Ruling watch | Re-check saved cases against rulings CBP issued later | Origin answers go stale as CBP rules | shipped (needs a corpus rebuild to see new rulings) |
| Operating-cost mode | Labour, logistics, capex and retooling over a horizon, with lead-time and order checks ([modelled-operational-costs.md](modelled-operational-costs.md)) | Plans priced on more than duty | shipped (optional) |
| Claims check | Headline evaluation counts recomputed from committed result rows; figure ledger and provenance checked (`go run ./cmd/claimscheck`) | Numbers a buyer can audit | shipped |
| API rate limiting | Every route has a rate class; model-route limits shared across instances in Firestore on Cloud Run; per-client caps and `RateLimit-*` headers (D137, [SECURITY.md](api/SECURITY.md)) | One integration cannot starve the others | shipped (the basic read class and the in-flight and failed-key caps are still per instance) |
| MCP server | The origin, plan, draft and broker review tools over MCP, 14 tools, the same limits as the API ([MCP.md](MCP.md), #379). A Google ADK agent in `examples/adk-agent` runs a whole case over it (D391) | Teams already run assistants and want Unde as a tool | shipped; the ADK agent run once against the live server through a gateway, not with a Google API key |
| Multimodal input | A photo, screenshot or scanned PDF of a BOM read by Gemini into a draft the user checks ([MULTIMODAL.md](MULTIMODAL.md), #378) | Engineering facts live in drawings and spec sheets | shipped (measured on 20 synthetic images only; drawings and real scans not measured) |
| Broker review link | The case owner sends one frozen draft (current setup or a plan) to an outside customs broker as an expiring, revocable link; the broker approves, requests changes with a comment, or marks it to file as a binding ruling request; the owner sees the status and comments on the case (D200, [SECURITY.md](api/SECURITY.md#broker-review-links-d200)) | The broker is the one who files; the human check becomes a step in the product instead of an email | shipped (minimal: no broker account, no edits to the letter, no notifications; e-filing stays planned) |
| Calibrated confidence | The model's stated confidence mapped to a probability by a map fitted on the development set; auto-accept at calibrated 0.90 (D160, D161) | A number a reviewer can read as a chance | shipped (general engine on Gemini 4 Argon only) |

### Owner and infrastructure steps to switch these on

These are deployment changes for the project owner, not code changes, and none is made by a feature PR:

1. **Enable the audit trail:** create an audit key in Secret Manager and pass it to the backend as `AUDIT_ACTOR_KEY`. Then check that `GET /v1/audit` and `GET /v1/audit/verify` answer (they are registered only when the trail is on, [audit-trail.md](design/audit-trail.md)).
2. **Require sign-in:** set `STRICT_AUTH=on`. Anonymous visitors then lose access to private case operations; decide first how the public demo should open (the pinned demo cases, or a demo account).
3. **Bearer check behind IAM:** set `REQUIRE_BEARER=on` after confirming Cloud Run passes the header to the container (defence in depth, [api/SECURITY.md](api/SECURITY.md)).

## Next: 0–3 months

The work that stands between the prototype and a first paid pilot: identity, tenancy, an integration surface, and a person in the loop.

| Item | What it is | Why an enterprise needs it | Removes | Effort | Status |
|---|---|---|---|---|---|
| SSO with SAML and OIDC | Sign in through the customer's identity provider (Okta, Entra ID, Google Workspace) | Security teams require their own IdP, MFA and offboarding | Google sign-in is the only identity; "company sign-in would need work" (README) | M | planned |
| SCIM provisioning | Users and groups created, changed and removed from the IdP, mapped to roles | Access ends when someone leaves, without a manual step | Membership is invite-by-email only (roles-and-strict-auth.md, section 2) | M | planned |
| Multiple workspaces per user | A user in more than one workspace, with case moves between them | Business units and brokers work across teams | A user is in at most one workspace; case reassignment is not supported | M | planned |
| Public API, versioned | A stable `/v1` contract with deprecation rules, SDK examples and webhooks for long jobs | ERP and PLM integrations need a contract that will not move | The API exists but carries no stability promise | M | planned |
| Per-user limits and drawing input | Per-user (signed-in) rate limits, and shared in-flight and failed-key caps; reading engineering drawings and PDF datasheets, measured on real documents | Limits that follow a person across devices; facts that live in drawings | The basic class and some caps are per instance (D137); image import is measured on synthetic BOM images only (D167, D168) | M | planned |
| Human-in-the-loop review queue | Below-bar and out-of-scope cases land in a queue with an assignee, a decision and a note, all audited | Someone accountable signs off before a plan is acted on | "Needs expert review" is a label, not a workflow | M | planned |
| Broker collaboration, beyond the review link | Tracked edits to the draft letter by the broker, several brokers per case with named accounts, notifications | The broker is the one who files and usually edits the letter | The broker review link (shipped, below) is read and answer only: no edits, no email, no broker account | M | planned |
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
| More product families | Eleven families added from CBP precedent (D138, D212), six written from Part 102 marking rulings (D580) and six picked from rulings with no template in either regime (D660), 26 in all; next: car air and cabin filters and bicycles (counted in D660, too few pool rulings to write from), the families a pilot brings and a template builder for compliance teams | About one in five general held-out rulings matches no family (D661: 298 of 377 match); 10 of the 18 frozen held-out Part 102 rulings still match none (D661) | "Plans need one of 26 product families" (README) | M | partly built (26 families); builder planned |
| Continuous evaluation | Monthly re-scoring on new rulings with drift alerts, building on the post-cutoff planner test set (D170/D192), the second held-out split (D150) and calibration metrics (D156, D160) | Buyers need evidence that accuracy holds over time | "The planner is the least tested part": the post-cutoff planner test is 18 frozen pairs (12 real changes, 6 controls; one reader, one gateway run; precision 9/9, recall 9/12). Recompute with `python3 -I -B eval/planner/summarize.py . --check` and `python3 -I -B eval/planner/summarize_addendum.py . --check`. Re-scoring is by hand. | M | partly built (post-cutoff test sets, calibration); monthly re-scoring planned |
| Automatic corpus refresh | Fetch and index new CBP rulings on a schedule so the ruling watch and retrieval stay current | Precedent changes; a stale corpus gives stale answers | The watch sees new rulings only after a manual rebuild | S | planned |
| SLAs and status | Uptime and response-time targets, a status page, a support channel, and a regional model fallback | A tool in the approval path must be reachable | Best-effort hackathon hosting; the fallback model shares the primary's outage (D88) | M | planned |
| Usage-based pricing and cost controls | Metered predictions and plan searches, per-workspace budgets and caps, and a spend estimate before each search | Finance needs predictable spend; admins need a ceiling | A per-search estimate exists (D87), but there is no per-tenant budget or bill | M | planned |

## Exploring

Larger scopes we would only start with a qualified customs professional and a separate evaluation, as [scope-and-jurisdiction.md](scope-and-jurisdiction.md) (section 7) requires.

| Item | What it is | Why an enterprise needs it | Removes | Effort | Status |
|---|---|---|---|---|---|
| Market on every case | Each case names the market it is for; a question for a market with no engine gets no answer and goes to review | A non-U.S. question must never get a U.S. answer | Nothing checked that a question was about U.S. entry | S | built (D680-D682): `jurisdiction` on cases, API and MCP; a market picker on the new-case form; a registry of supported markets (the United States; Canada as a rules check only, D725); a deterministic detector for text that names no market, which reads about three in four model-written non-U.S. questions (22 of 30) and flags none of 1,026 U.S. rulings |
| EU Binding Origin Information | EU non-preferential origin with BOI decisions as precedent | Manufacturers ship to more than one market | U.S. origin only | L | in progress (Phase 4, J3): a market registers its engine with the D680 registry |
| USMCA regional value content | Encoded product-specific rules and RVC calculations, separate from marking origin | North American flows depend on qualification, not only origin | RVC is not computed; Part 102 here is marking origin only | L | planned |
| Preferential origin under FTAs | Qualification checks for trade agreements such as RCEP and ATIGA, with supplier declarations | ASEAN manufacturers claim preference on most shipments | No preferential-origin support of any kind | L | first version built: an RCEP rules check from the official Annex 3A rules (HS 2022), with supplier declarations, RVC, de minimis and cumulation, separate from origin answers (D690). Not reviewed by a qualified professional; the positive-only Japan Customs evaluation is recorded (D691, D716, D717). ATIGA and other agreements: planned |
| Other customs authorities | Origin questions for authorities such as Japan, Korea and the UK | One tool across a manufacturer's markets | No non-U.S. origin rules are encoded; such cases are recognised and sent to review (D680) | L | planned |
| E-filing of ruling requests | Submit the broker-approved request through CBP's eRulings channel and track the answer | Removes a manual step and keeps the answer on the case | The draft is a letter the broker files by hand | M | planned |
| Deterministic rule layer | Tariff-shift and textile rules in tested code beside the model | Auditable answers for rule-based regimes | The rules live only in prompts (scope-and-jurisdiction.md, section 2) | L | first version shipped: the 19 CFR 102.20 tariff-shift rules for chapters 73, 82, 84, 85, 87, 90 and 94, and since D460 also 15, 21, 39, 40, 44, 48, 71, 76, 83 and 95, shown beside the model as a display-only check when the parts list gives HTS codes (D360-D363). It decides few real cases, because they rarely give codes (EVALUATION.md §9). Shipped since: HTS codes suggested for each part, which the user accepts per part (D380); with them it decides 16 of 69 development and 14 of 29 held-out Part 102 rulings with the 17 chapters (12 of 29 held out without the five chapters picked with held-out data, D560), against 8 and 4 without (EVALUATION.md §13; 9 and 7 with the first 7 chapters, §10). Part 102-aware plans, first version shipped: each plan move carries the rule check of the product after the move, flagged when precedent supports the move and the rule does not; display only (D441; EVALUATION.md §11). Part 102 ranking shipped: on a Part 102 case, shown plans rank by tier, then the rule check (D500); the broker draft and the country comparison carry the check, and three Part 102 plan searches on development rulings were scored (D502; EVALUATION.md §11.1). Textile rules (102.21), the remaining chapters (each had fewer than 4 goods in the Part 102 data, D460) and letting it affect answers: planned |

## What stays true in every phase

- Unde suggests; a licensed customs broker or a CBP ruling decides.
- A new number goes into [NUMBERS.md](NUMBERS.md) with its source before it appears anywhere.
- A new jurisdiction or regime gets its own held-out evaluation before any coverage claim.

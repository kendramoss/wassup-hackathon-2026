# SOC 2 Type II, then ISO 27001: controls map (#876)

This is a working map for a readiness assessment, not a compliance statement. Unde Labs has no SOC 2 report and no ISO 27001 certificate, and nothing here says any control is designed or operating effectively. A SOC 2 Type II report is an independent auditor's opinion over an observation period (usually 3 to 12 months); ISO 27001 certification is an accredited body's audit of an information security management system (ISMS). Both need people, policies and a period of evidence that code alone cannot supply.

Each row lists what exists in this repository today, the gap, and who acts. "Owner" means the company and its cloud owner (kendramoss), not this codebase.

## 1. Scope to decide first (owner)

- System boundary: the backend (Cloud Run), the frontend (Cloud Run), Firestore, the model gateway, GitHub, Google Cloud project IAM. Third parties in scope as subservice organisations: Google Cloud, the model provider, GitHub.
- Trust Services Criteria: Security (required). Availability and Confidentiality are the likely additions for an enterprise trade-compliance product; Processing Integrity and Privacy only if customers ask.
- Auditor and observation period; a readiness assessment before the period starts.

## 2. Common criteria (Security)

| Criterion | What exists in the repo | Gap | Who |
|---|---|---|---|
| CC1 Control environment (ethics, structure, oversight) | none | Code of conduct, org chart, security role owner, board or founder oversight record | Owner |
| CC2 Communication | `docs/api/SECURITY.md` (vulnerability reporting, limits), `docs/design/*` | Customer-facing security page, internal policy acknowledgement, a commitments page (SLA, support) | Owner |
| CC3 Risk assessment | `docs/scope-and-jurisdiction.md` (product risk), known-risk sections in `docs/design/audit-trail.md` §12 and `docs/design/data-handling.md` | A risk register reviewed at least yearly, with owners and treatments | Owner |
| CC4 Monitoring of controls | CI on every pull request (`.github/workflows/ci.yml`: gofmt, vet, staticcheck, Go tests, frontend tests, lint, build); claims checks (`cmd/claimscheck`) | A periodic control self-review and its record | Owner |
| CC5 Control activities and policies | Merge only on green CI on the exact head commit (project rule); route classification tests (`internal/httpapi/isolation_matrix_test.go`) that fail when a new route is not classified | Written policies: information security, access, change management, vendor, incident response, data retention, acceptable use | Owner |
| CC6.1 Logical access | Google sign-in for users and workspace roles viewer, editor, admin (`docs/design/roles-and-strict-auth.md`); API keys stored as SHA-256 only (`internal/apikey`); `STRICT_AUTH`; case isolation (`docs/design/case-isolation.md`); admin token for platform actions | SSO/SAML and SCIM for customer identity (#868, #869); MFA enforcement evidence for staff accounts in Google Cloud and GitHub | Owner, plus #868 and #869 |
| CC6.2 and 6.3 Provisioning and deprovisioning | Workspace invite, role change, member removal, each audited as a sensitive write (`docs/design/audit-trail.md` §4) | Staff joiner and leaver checklist; quarterly access review of Google Cloud IAM, GitHub, Firestore | Owner |
| CC6.6 and 6.7 Boundary and data in transit | Backend private behind the frontend proxy; HTTPS; security headers; rate limits (`docs/api/SECURITY.md`) | Evidence of TLS settings and ingress configuration from the deployed project | Owner |
| CC6.8 Malicious software | Dependency lockfiles (`go.sum`, `package-lock.json`) | Automated dependency and vulnerability scanning (for example govulncheck and npm audit in CI), image scanning | Owner (CI change) |
| CC7.1 to 7.3 Detection and monitoring | Tenant audit trail with a hash chain and verification (`GET /v1/audit`, `/v1/audit/verify`); audit export for the customer's own records (`GET /v1/audit/export`, `cmd/auditverify`, #871); minimised structured logs (`internal/logsafe`) | Alerting on audit verification failures and on auth-denial spikes; Cloud Logging retention evidence; an external anchor for the audit head (`docs/design/audit-trail.md` §8) | Owner |
| CC7.4 and 7.5 Incident response and recovery | none in the repo | Incident response plan, on-call, a tabletop exercise record, post-incident reviews | Owner |
| CC8.1 Change management | Pull requests, required CI, signed commits by the automation account, the decision log (`docs/DECISIONS.md`), pre-registered evaluations before model changes | Branch protection and required review evidence from GitHub settings; deploy approvals; segregation between author and approver | Owner |
| CC9.1 Risk mitigation, business disruption | none | Business continuity plan; backup and restore test (Firestore export) | Owner |
| CC9.2 Vendor management | none | Vendor inventory (Google Cloud, model provider, GitHub) with their SOC 2 reports reviewed yearly | Owner |

## 3. Additional criteria if chosen

| Criterion | What exists | Gap | Who |
|---|---|---|---|
| A1 Availability | Health checks (`GET /health?deep=1`); rate limits and spend caps | Uptime target and status page (#885), capacity monitoring, restore test | Owner, #885 |
| C1.1 Confidential information identified and protected | Case data never logged (`docs/design/data-handling.md`); tenant isolation tests | Data classification policy; customer-managed keys (#874) and data residency (#873) if contracted | Owner, #873, #874 |
| C1.2 Disposal | Workspace retention policy and purge with tombstones (`RETENTION=on`, `cmd/retention`, #875); case delete is a hard delete | Caches derived from case facts are not purged with the case (`docs/design/data-handling.md` item 7); backups and provider copies outside the application | Owner, follow-up code |

## 4. ISO 27001:2022 Annex A, the same evidence

ISO 27001 needs an ISMS first: scope, information security policy, risk assessment and treatment, a Statement of Applicability, internal audit and management review (clauses 4 to 10). None exists yet; all are owner actions. The repository evidence above maps to these Annex A controls: 5.15 to 5.18 access control and identity (roles, API keys, sign-in), 5.23 cloud services (owner), 5.33 records protection and 8.15 logging (audit trail and export), 5.34 privacy and 8.11 data masking (log minimisation), 8.10 information deletion (retention purge and tombstones), 8.25 to 8.29 secure development and testing (CI, route classification tests), 8.32 change management (pull requests and decision log).

## 5. Order of work

1. Owner: choose criteria, auditor and period; write the seven policies in CC5; name a security owner.
2. Owner: turn on the controls that already exist in code for the production deployment and keep the startup log lines as evidence (`AUDIT_ACTOR_KEY`, `AUDIT_EXPORT=on`, `RETENTION=on`, `STRICT_AUTH`).
3. Owner: GitHub branch protection, MFA, quarterly access reviews, vendor reports, backups with a restore test.
4. Code: dependency scanning in CI, audit-failure alerting, purge of derived caches with the case.
5. Readiness assessment, then the Type II observation period, then ISO 27001 using the same evidence.
